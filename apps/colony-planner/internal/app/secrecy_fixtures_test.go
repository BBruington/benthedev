package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/store"
)

// secrecyFixture is one feature's share of the secrecy sweep (secrecy_test.go). The sweep can only
// catch a leak of a value that holds a canary, and it can only reach a handler's data path when every
// path parameter names a real row, so each feature that adds a Dm field, a hidden row or a path
// parameter adds or extends an entry here:
//   - plant writes canary (or numericCanary) into every DM-only value the feature stores: each field
//     of its Dm sub-struct and each row players must not see (a DM-visibility timeline entry, an undone
//     PUBLIC entry, a hidden NPC). It returns a fixture ID for each path parameter name the feature's
//     routes use, pointing at a row a player is allowed to read where one exists.
//   - dmProof lists GET paths on which the DM must see a canary, so TestDmSeesTheCanaries proves the
//     planting really happened and the sweep can fail.
type secrecyFixture struct {
	feature string
	plant   func(t *testing.T, testHarness *harness) map[string]string
	dmProof []string
}

var secrecyFixtures = []secrecyFixture{
	{
		feature: "members",
		// newHarness already gives every member a canary Discord ID and username (MemberDm).
		plant: func(_ *testing.T, testHarness *harness) map[string]string {
			// Another member's ID, so member-targeted operations aim at someone else's row.
			return map[string]string{"memberId": testHarness.otherPlayer.ID.String()}
		},
		dmProof: []string{"/api/members"},
	},
	{
		feature: "settings",
		plant: func(t *testing.T, testHarness *harness) map[string]string {
			t.Helper()
			channelID := numericCanary
			if _, err := store.New(testHarness.database.Pool).UpdateSettings(context.Background(), store.UpdateSettingsParams{
				CampaignName:             "Test Campaign",
				Timezone:                 "UTC",
				DiscordAnnounceChannelID: &channelID,
				DiscordReminderChannelID: &channelID,
				DiscordDmChannelID:       &channelID,
			}); err != nil {
				t.Fatalf("plant settings canaries: %v", err)
			}
			return nil
		},
		dmProof: []string{"/api/settings"},
	},
	{
		feature: "colony",
		// The undo slot (ColonyBoardDm.LastChange) is the board's only DM-only value; players read
		// exact stock (open question 5). A real DM-only adjustment also goes through core first, its
		// reason a canary, so the sweep covers the text the write path renders.
		plant: func(t *testing.T, testHarness *harness) map[string]string {
			t.Helper()
			testContext := context.Background()
			dmViewer := testHarness.viewerOf(testHarness.dm)
			board, err := testHarness.core.GetColonyBoard(testContext, dmViewer)
			if err != nil {
				t.Fatalf("read the board: %v", err)
			}
			adjusted, err := testHarness.core.AdjustStock(testContext, dmViewer, core.AdjustInput{
				ExpectedVersion: board.Version,
				Source:          core.AdjustSourceAdjustment,
				Deltas:          map[string]int64{"gold": 7},
				SetTo:           map[string]int64{},
				Reason:          canary + " paid the smuggler",
			})
			if err != nil {
				t.Fatalf("plant a DM-only adjustment: %v", err)
			}
			if adjusted.Entries[0].Dm.Visibility != core.TimelineVisibilityDm {
				t.Fatalf("a stock adjustment defaulted to %s; want DM", adjusted.Entries[0].Dm.Visibility)
			}
			lastChange, err := json.Marshal(map[string]any{
				"groupId":      uuid.New(),
				"label":        canary + " 3 days pass",
				"day":          3,
				"versionAfter": adjusted.Board.Version + 1,
				"changedAt":    time.Now().UTC(),
				"before":       map[string]any{"day": 0},
			})
			if err != nil {
				t.Fatalf("encode the undo slot: %v", err)
			}
			if _, err := store.New(testHarness.database.Pool).SaveColonyChange(testContext, store.SaveColonyChangeParams{
				Day:        3,
				LastChange: lastChange,
			}); err != nil {
				t.Fatalf("plant colony canaries: %v", err)
			}
			return map[string]string{"resourceKey": "gold"}
		},
		dmProof: []string{"/api/colony", "/api/timeline/recent"},
	},
	{
		feature: "timeline",
		// Hidden rows: a DM-visibility entry and an undone PUBLIC entry. They sit on day 1, inside the
		// range the sweep asks for (required integer query parameters are filled with 1).
		plant: func(t *testing.T, testHarness *harness) map[string]string {
			t.Helper()
			queries := store.New(testHarness.database.Pool)
			testContext := context.Background()
			createEntry := func(groupID *uuid.UUID, title string, visibility store.TimelineVisibility) store.TimelineEvent {
				entry, err := queries.CreateTimelineEvent(testContext, store.CreateTimelineEventParams{
					GroupID:    groupID,
					Day:        1,
					Kind:       string(core.TimelineKindStockAdjusted),
					Title:      title,
					Body:       title,
					Visibility: visibility,
					Deltas:     []byte(`{"resources":{"gold":5}}`),
					Data:       []byte(`{}`),
				})
				if err != nil {
					t.Fatalf("plant timeline entry %q: %v", title, err)
				}
				return entry
			}
			publicGroup := uuid.New()
			publicEntry := createEntry(&publicGroup, "Gold found in the river", core.TimelineVisibilityPublic)
			createEntry(nil, canary+" DM-only entry", core.TimelineVisibilityDm)
			undoneGroup := uuid.New()
			createEntry(&undoneGroup, canary+" undone entry", core.TimelineVisibilityPublic)
			if _, err := queries.MarkTimelineGroupUndone(testContext, undoneGroup); err != nil {
				t.Fatalf("mark the planted entry undone: %v", err)
			}
			return map[string]string{"timelineEntryId": publicEntry.ID.String()}
		},
		dmProof: []string{"/api/timeline?fromDay=0&toDay=6", "/api/timeline/recent"},
	},
}

// plantCanaries runs every feature's fixture and returns the path parameter IDs they name.
func plantCanaries(t *testing.T, testHarness *harness) map[string]string {
	t.Helper()
	fixtureIDs := map[string]string{}
	for _, fixture := range secrecyFixtures {
		for name, id := range fixture.plant(t, testHarness) {
			if existing, taken := fixtureIDs[name]; taken && existing != id {
				t.Fatalf("secrecy fixture %s names path parameter %q again", fixture.feature, name)
			}
			fixtureIDs[name] = id
		}
	}
	return fixtureIDs
}
