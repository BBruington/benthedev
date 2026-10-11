package core

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/store"
)

var testCampaignID = uuid.MustParse("00000000-0000-4000-8000-00000000000a")

func viewerIn(campaignID uuid.UUID, status MemberStatus, role CampaignRole) Viewer {
	return Viewer{
		accountID:        uuid.New(),
		memberID:         uuid.New(),
		campaignID:       campaignID,
		sessionTokenHash: []byte(uuid.NewString()),
		role:             role,
		status:           status,
		campaignStatus:   store.CampaignStatusACTIVE,
	}
}

func activeViewer() Viewer {
	return viewerIn(testCampaignID, store.MemberStatusACTIVE, store.CampaignRolePLAYER)
}

func pendingViewer() Viewer {
	return viewerIn(testCampaignID, store.MemberStatusPENDING, store.CampaignRolePLAYER)
}

func subscribe(t *testing.T, broker *Broker, viewer Viewer, lastEventID string) (*Subscription, *StreamMessage) {
	t.Helper()
	subscription, catchUp, err := broker.Subscribe(viewer, lastEventID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	return subscription, catchUp
}

func received(subscription *Subscription) []StreamMessage {
	var messages []StreamMessage
	for {
		select {
		case message := <-subscription.Messages():
			messages = append(messages, message)
		default:
			return messages
		}
	}
}

func TestSubscribeCatchUp(t *testing.T) {
	broker := NewBroker()
	_, firstCatchUp := subscribe(t, broker, activeViewer(), "")
	if firstCatchUp == nil || fmt.Sprint(firstCatchUp.Data.Topics) != fmt.Sprint(AllTopics()) {
		t.Fatalf("a first connect should invalidate everything, got %+v", firstCatchUp)
	}
	if _, upToDate := subscribe(t, broker, activeViewer(), firstCatchUp.ID); upToDate != nil {
		t.Fatalf("a reconnect with the newest ID should get nothing, got %+v", upToDate)
	}

	broker.Publish(testCampaignID, TopicSettings)
	_, behind := subscribe(t, broker, activeViewer(), firstCatchUp.ID)
	if behind == nil || behind.ID == firstCatchUp.ID {
		t.Fatalf("a client that missed a message should be told to refetch, got %+v", behind)
	}
	if _, otherBoot := subscribe(t, broker, activeViewer(), "0ldb007-"+fmt.Sprint(broker.logs[testCampaignID].sequence)); otherBoot == nil {
		t.Fatal("an ID from another boot should be told to refetch")
	}

	_, pendingCatchUp := subscribe(t, broker, pendingViewer(), "")
	if pendingCatchUp == nil || fmt.Sprint(pendingCatchUp.Data.Topics) != "[me]" {
		t.Fatalf("a pending stream's catch-up should be only me, got %+v", pendingCatchUp)
	}
	if _, _, err := broker.Subscribe(Viewer{}, ""); err == nil {
		t.Fatal("the zero Viewer opened a stream outside any campaign")
	}
}

func TestPublishScopes(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	pending := pendingViewer()
	playerStream, _ := subscribe(t, broker, player, "")
	pendingStream, _ := subscribe(t, broker, pending, "")

	broker.Publish(testCampaignID, TopicMembers, TopicSettings)
	if messages := received(pendingStream); len(messages) != 0 {
		t.Fatalf("a pending stream heard a broadcast: %+v", messages)
	}
	if messages := received(playerStream); len(messages) != 1 || fmt.Sprint(messages[0].Data.Topics) != "[members settings]" {
		t.Fatalf("player got %+v", messages)
	}

	broker.publishToMember(testCampaignID, pending.memberID, TopicMe, TopicMembers)
	if messages := received(pendingStream); len(messages) != 1 || fmt.Sprint(messages[0].Data.Topics) != "[me]" {
		t.Fatalf("a pending member should hear only their own me topic, got %+v", messages)
	}
	if messages := received(playerStream); len(messages) != 0 {
		t.Fatalf("a message for one member reached another: %+v", messages)
	}

	broker.memberStatusChanged(pending.memberID, true)
	broker.Publish(testCampaignID, TopicSettings)
	if messages := received(pendingStream); len(messages) != 1 {
		t.Fatalf("an approved member's open stream should hear broadcasts, got %+v", messages)
	}

	// A Viewer read just before the approval still subscribes with the approved scope.
	staleStream, _ := subscribe(t, broker, pending, "")
	broker.Publish(testCampaignID, TopicSettings)
	if messages := received(staleStream); len(messages) != 1 {
		t.Fatalf("a stream opened with a stale Viewer missed a broadcast: %+v", messages)
	}
}

// Campaigns don't hear each other, and each counts its own messages, so a player learns nothing of
// another campaign's activity from stream IDs.
func TestCampaignsHaveSeparateStreamsAndSequences(t *testing.T) {
	broker := NewBroker()
	otherCampaignID := uuid.New()
	inA := activeViewer()
	inB := viewerIn(otherCampaignID, store.MemberStatusACTIVE, store.CampaignRoleDM)
	// One person in both campaigns, on one session.
	sameSessionInB := viewerIn(otherCampaignID, store.MemberStatusACTIVE, store.CampaignRolePLAYER)
	sameSessionInB.sessionTokenHash = inA.sessionTokenHash
	streamA, catchUpA := subscribe(t, broker, inA, "")
	streamB, _ := subscribe(t, broker, inB, "")
	sameSessionStreamB, _ := subscribe(t, broker, sameSessionInB, "")

	for range 5 {
		broker.Publish(otherCampaignID, TopicColony)
	}
	if messages := received(streamA); len(messages) != 0 {
		t.Fatalf("campaign A heard campaign B: %+v", messages)
	}
	if messages := received(streamB); len(messages) != 5 {
		t.Fatalf("campaign B heard %d of its 5 messages", len(messages))
	}
	received(sameSessionStreamB)
	if _, resume := subscribe(t, broker, inA, catchUpA.ID); resume != nil {
		t.Fatalf("B's traffic invalidated A's resume: %+v", resume)
	}
	broker.Publish(testCampaignID, TopicColony)
	if messages := received(streamA); len(messages) != 1 || messages[0].ID != broker.bootID+"-1" {
		t.Fatalf("A's first message = %+v, want ID %s-1", messages, broker.bootID)
	}

	// A session message (view as player) reaches that session's streams in every campaign.
	broker.publishToSession(inA.sessionTokenHash, TopicMe)
	if len(received(streamA)) != 1 || len(received(sameSessionStreamB)) != 1 || len(received(streamB)) != 0 {
		t.Fatal("a session message should reach exactly that session's streams, in each campaign")
	}

	broker.Publish(uuid.Nil, TopicColony)
	if len(received(streamA)) != 0 || len(received(streamB)) != 0 {
		t.Fatal("a publish without a campaign reached a stream")
	}
}

func TestSuspensionRescopesAndDeletionClosesACampaign(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	stream, _ := subscribe(t, broker, player, "")
	other := viewerIn(uuid.New(), store.MemberStatusACTIVE, store.CampaignRolePLAYER)
	otherStream, _ := subscribe(t, broker, other, "")

	broker.campaignStatusChanged(testCampaignID, false)
	broker.publishToEveryStream(testCampaignID, TopicMe)
	broker.Publish(testCampaignID, TopicColony)
	if messages := received(stream); len(messages) != 1 || fmt.Sprint(messages[0].Data.Topics) != "[me]" {
		t.Fatalf("a suspended campaign's stream got %+v, want only me", messages)
	}
	// A Viewer read just before the suspension still gets the suspended scope.
	stale, _ := subscribe(t, broker, player, "")
	broker.Publish(testCampaignID, TopicColony)
	if len(received(stale)) != 0 {
		t.Fatal("a stream opened with a stale Viewer heard a suspended campaign's broadcast")
	}
	broker.campaignStatusChanged(testCampaignID, true)
	broker.Publish(testCampaignID, TopicColony)
	if len(received(stream)) != 1 {
		t.Fatal("an unsuspended campaign's stream missed a broadcast")
	}

	broker.CloseCampaign(testCampaignID)
	assertClosed(t, stream, true)
	assertClosed(t, stale, true)
	assertClosed(t, otherStream, false)
	if _, found := broker.logs[testCampaignID]; found {
		t.Fatal("a deleted campaign's log was kept")
	}
}

func TestCloseStreams(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	secondDevice := player
	secondDevice.sessionTokenHash = []byte("second device")
	other := activeViewer()
	first, _ := subscribe(t, broker, player, "")
	second, _ := subscribe(t, broker, secondDevice, "")
	untouched, _ := subscribe(t, broker, other, "")

	broker.CloseSession(player.sessionTokenHash)
	assertClosed(t, first, true)
	assertClosed(t, second, false)

	broker.CloseAccount(player.accountID)
	assertClosed(t, second, true)
	assertClosed(t, untouched, false)
	if broker.OpenStreams() != 1 {
		t.Fatalf("open streams = %d, want 1", broker.OpenStreams())
	}
	untouched.Close()
	untouched.Close()
	if broker.OpenStreams() != 0 {
		t.Fatalf("open streams = %d after closing, want 0", broker.OpenStreams())
	}
}

func TestSlowStreamsAreClosedRatherThanBlocking(t *testing.T) {
	broker := NewBroker()
	slow, _ := subscribe(t, broker, activeViewer(), "")
	for range subscriptionBufferSize + 1 {
		broker.Publish(testCampaignID, TopicSettings)
	}
	assertClosed(t, slow, true)
}

func assertClosed(t *testing.T, subscription *Subscription, want bool) {
	t.Helper()
	select {
	case <-subscription.Closed():
		if !want {
			t.Fatal("a stream was closed that should be open")
		}
	default:
		if want {
			t.Fatal("a stream is open that should be closed")
		}
	}
}

// A resume sends only what this stream missed: a targeted publish to someone else (the DM's view as
// player switch, a message to another member) does not make every other tab refetch everything.
func TestResumeSendsOnlyWhatTheStreamMissed(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	dm := viewerIn(testCampaignID, store.MemberStatusACTIVE, store.CampaignRoleDM)
	_, catchUp := subscribe(t, broker, player, "")
	lastID := catchUp.ID
	dmStream, _ := subscribe(t, broker, dm, "")
	dmStream.Close()

	broker.publish(testCampaignID, AllTopics(), recipients{sessionKey: string(dm.sessionTokenHash)})
	broker.publishToMember(testCampaignID, uuid.New(), TopicMe)
	if _, resume := subscribe(t, broker, player, lastID); resume != nil {
		t.Fatalf("a targeted publish to someone else invalidated my resume: %+v", resume)
	}

	broker.Publish(testCampaignID, TopicSettings)
	broker.publishToMember(testCampaignID, player.memberID, TopicMe)
	_, resume := subscribe(t, broker, player, lastID)
	if resume == nil || fmt.Sprint(resume.Data.Topics) != "[me settings]" || resume.ID != broker.currentIDLocked(broker.logs[testCampaignID]) {
		t.Fatalf("resume = %+v, want [me settings] at the newest ID", resume)
	}

	// The same session hears its own targeted publish.
	_, dmResume := subscribe(t, broker, dm, lastID)
	if dmResume == nil || fmt.Sprint(dmResume.Data.Topics) != fmt.Sprint(AllTopics()) {
		t.Fatalf("the DM's session resume = %+v, want every topic", dmResume)
	}

	// A pending stream hears only its own me topic, even when broadcasts went by.
	pending := pendingViewer()
	_, pendingCatchUp := subscribe(t, broker, pending, "")
	broker.Publish(testCampaignID, TopicMembers)
	if _, pendingResume := subscribe(t, broker, pending, pendingCatchUp.ID); pendingResume != nil {
		t.Fatalf("a pending stream's resume after a broadcast = %+v, want nothing", pendingResume)
	}
}

func TestResumeFallsBackToEverythingWhenTheRecentMessagesDoNotReachBack(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	_, catchUp := subscribe(t, broker, player, "")
	for range recentMessageLimit + 1 {
		broker.publishToMember(testCampaignID, uuid.New(), TopicMe)
	}
	_, resume := subscribe(t, broker, player, catchUp.ID)
	if resume == nil || fmt.Sprint(resume.Data.Topics) != fmt.Sprint(AllTopics()) {
		t.Fatalf("resume past the recent messages = %+v, want every topic", resume)
	}
	for _, lastEventID := range []string{"garbage", broker.bootID + "-x", broker.bootID + "-999999"} {
		if _, fallback := subscribe(t, broker, player, lastEventID); fallback == nil || len(fallback.Data.Topics) != len(AllTopics()) {
			t.Errorf("last ID %q: %+v, want every topic", lastEventID, fallback)
		}
	}
}

// AllTopics feeds reconnect catch-up, view as player and the TypeScript Topic union; a constant left
// out of it would silently drop out of all three.
func TestAllTopicsListsEveryTopicConstant(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list the package files: %v", err)
	}
	fileSet := token.NewFileSet()
	var declared []Topic
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, declaration := range parsed.Decls {
			general, isGeneral := declaration.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				valueSpec, _ := spec.(*ast.ValueSpec)
				if typeName, isIdent := valueSpec.Type.(*ast.Ident); !isIdent || typeName.Name != "Topic" {
					continue
				}
				for _, value := range valueSpec.Values {
					literal, isLiteral := value.(*ast.BasicLit)
					if !isLiteral {
						t.Fatalf("%s: a Topic constant that is not a string literal", file)
					}
					unquoted, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatalf("%s: %v", file, err)
					}
					declared = append(declared, Topic(unquoted))
				}
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("found no Topic constants")
	}
	for _, topic := range declared {
		if !slices.Contains(AllTopics(), topic) {
			t.Errorf("Topic %q is missing from AllTopics()", topic)
		}
	}
	if len(declared) != len(AllTopics()) {
		t.Errorf("AllTopics() = %v, but the declared Topic constants are %v", AllTopics(), declared)
	}
}
