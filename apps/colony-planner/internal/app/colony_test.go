package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
)

func (testHarness *harness) boardVersion(token string) int64 {
	testHarness.t.Helper()
	response := testHarness.do(http.MethodGet, campaignPathOf(testHarness.campaignA, "/colony"), requestOptions{token: token})
	testHarness.expectStatus(response, http.StatusOK)
	var board core.ColonyBoard
	if err := json.Unmarshal(response.Body.Bytes(), &board); err != nil {
		testHarness.t.Fatalf("decode the board: %v", err)
	}
	return board.Version
}

// Example E over HTTP: confirming the same advance twice at once changes the colony once; the second
// answers 409 with the day it changed to.
func TestConfirmingAnAdvanceTwiceGivesOneConflict(t *testing.T) {
	testHarness := newHarness(t)
	dmToken := testHarness.signIn(testHarness.dmA)
	body := fmt.Sprintf(`{"days":10,"expectedVersion":%d}`, testHarness.boardVersion(dmToken))

	statuses := make([]int, 2)
	bodies := make([]string, 2)
	var group sync.WaitGroup
	for index := range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			response := testHarness.do(http.MethodPost, campaignPathOf(testHarness.campaignA, "/colony/advance"), requestOptions{token: dmToken, body: body})
			statuses[index] = response.Code
			bodies[index] = response.Body.String()
		}()
	}
	group.Wait()
	conflict := slices.Index(statuses, http.StatusConflict)
	if !slices.Contains(statuses, http.StatusOK) || conflict < 0 {
		t.Fatalf("statuses = %v; want one 200 and one 409 (%v)", statuses, bodies)
	}
	if !strings.Contains(bodies[conflict], "The colony changed (now Day 10). Review and retry.") {
		t.Fatalf("409 body = %s", bodies[conflict])
	}
}

// An open tab that sits idle after the DM acts runs no database query (spec §8.8): the broker carries
// topics only. A reconnect after phone sleep runs exactly one read, the membership the access gate
// checks, and no write (docs/design/multi-campaign.md §3.2); sessions come from the cache.
func TestIdleTabsAndReconnectsRunNoQueryAfterColonyChanges(t *testing.T) {
	testHarness := newHarness(t)
	campaignA := testHarness.campaignA
	playerToken := testHarness.signIn(testHarness.ana)
	stream := testHarness.openStream(campaignA, playerToken, "")
	catchUp, received := stream.nextMessage(2 * time.Second)
	if !received {
		t.Fatal("no catch-up message on connect")
	}

	dmViewer := testHarness.enter(testHarness.dmA, campaignA.id)
	board, err := testHarness.core.GetColonyBoard(context.Background(), dmViewer)
	if err != nil {
		t.Fatalf("read the board: %v", err)
	}
	if _, err := testHarness.core.Advance(context.Background(), dmViewer, core.AdvanceInput{Days: 3, ExpectedVersion: board.Version}); err != nil {
		t.Fatalf("advance: %v", err)
	}
	invalidation, received := stream.nextMessage(2 * time.Second)
	if !received {
		t.Fatal("the player's tab heard nothing after the advance")
	}
	var message core.InvalidateMessage
	if err := json.Unmarshal([]byte(invalidation.data), &message); err != nil {
		t.Fatalf("decode %q: %v", invalidation.data, err)
	}
	for _, topic := range []core.Topic{core.TopicColony, core.TopicTimeline} {
		if !slices.Contains(message.Topics, topic) {
			t.Errorf("the advance's invalidation %v lacks %s", message.Topics, topic)
		}
	}

	queries := testHarness.database.Queries
	queries.Reset()
	// Several heartbeats pass (the harness beats every 50 ms) with nothing new to say.
	if extra, received := stream.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("an idle tab got %+v", extra)
	}
	if count := queries.Count(); count != 0 {
		t.Fatalf("an idle open tab ran %d queries; want none", count)
	}
	stream.cancel()

	upToDate := testHarness.openStream(campaignA, playerToken, invalidation.id)
	if extra, received := upToDate.nextMessage(300 * time.Millisecond); received {
		t.Fatalf("an up-to-date reconnect got %+v; want nothing", extra)
	}
	upToDate.cancel()
	if count := queries.Count(); count != 1 {
		t.Fatalf("an up-to-date reconnect ran %d queries; want exactly one, the membership read", count)
	}
	queries.Reset()
	behind := testHarness.openStream(campaignA, playerToken, catchUp.id)
	missed, received := behind.nextMessage(2 * time.Second)
	if !received || !strings.Contains(missed.data, string(core.TopicColony)) {
		t.Fatalf("a reconnect from before the advance got %+v; want the colony invalidation", missed)
	}
	behind.cancel()
	if count := queries.Count(); count != 1 {
		t.Fatalf("a reconnect from behind ran %d queries; want exactly one, the membership read", count)
	}
}
