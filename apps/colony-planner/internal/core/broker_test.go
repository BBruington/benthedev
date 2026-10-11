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

func activeViewer() Viewer {
	return Viewer{MemberID: uuid.New(), Status: store.MemberStatusACTIVE, SessionTokenHash: []byte(uuid.NewString())}
}

func pendingViewer() Viewer {
	return Viewer{MemberID: uuid.New(), Status: store.MemberStatusPENDING, SessionTokenHash: []byte(uuid.NewString())}
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
	_, firstCatchUp := broker.Subscribe(activeViewer(), "")
	if firstCatchUp == nil || fmt.Sprint(firstCatchUp.Data.Topics) != fmt.Sprint(AllTopics()) {
		t.Fatalf("a first connect should invalidate everything, got %+v", firstCatchUp)
	}
	if _, upToDate := broker.Subscribe(activeViewer(), firstCatchUp.ID); upToDate != nil {
		t.Fatalf("a reconnect with the newest ID should get nothing, got %+v", upToDate)
	}

	broker.Publish(TopicSettings)
	_, behind := broker.Subscribe(activeViewer(), firstCatchUp.ID)
	if behind == nil || behind.ID == firstCatchUp.ID {
		t.Fatalf("a client that missed a message should be told to refetch, got %+v", behind)
	}
	if _, otherBoot := broker.Subscribe(activeViewer(), "0ldb007-"+fmt.Sprint(broker.sequence)); otherBoot == nil {
		t.Fatal("an ID from another boot should be told to refetch")
	}

	_, pendingCatchUp := broker.Subscribe(pendingViewer(), "")
	if pendingCatchUp == nil || fmt.Sprint(pendingCatchUp.Data.Topics) != "[me]" {
		t.Fatalf("a pending stream's catch-up should be only me, got %+v", pendingCatchUp)
	}
}

func TestPublishScopes(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	pending := pendingViewer()
	playerStream, _ := broker.Subscribe(player, "")
	pendingStream, _ := broker.Subscribe(pending, "")

	broker.Publish(TopicMembers, TopicSettings)
	if messages := received(pendingStream); len(messages) != 0 {
		t.Fatalf("a pending stream heard a broadcast: %+v", messages)
	}
	if messages := received(playerStream); len(messages) != 1 || fmt.Sprint(messages[0].Data.Topics) != "[members settings]" {
		t.Fatalf("player got %+v", messages)
	}

	broker.publishToMember(pending.MemberID, TopicMe, TopicMembers)
	if messages := received(pendingStream); len(messages) != 1 || fmt.Sprint(messages[0].Data.Topics) != "[me]" {
		t.Fatalf("a pending member should hear only their own me topic, got %+v", messages)
	}
	if messages := received(playerStream); len(messages) != 0 {
		t.Fatalf("a message for one member reached another: %+v", messages)
	}

	broker.memberStatusChanged(pending.MemberID, true)
	broker.Publish(TopicSettings)
	if messages := received(pendingStream); len(messages) != 1 {
		t.Fatalf("an approved member's open stream should hear broadcasts, got %+v", messages)
	}

	// A Viewer read just before the approval still subscribes with the approved scope.
	staleStream, _ := broker.Subscribe(pending, "")
	broker.Publish(TopicSettings)
	if messages := received(staleStream); len(messages) != 1 {
		t.Fatalf("a stream opened with a stale Viewer missed a broadcast: %+v", messages)
	}
}

func TestCloseStreams(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	secondDevice := player
	secondDevice.SessionTokenHash = []byte("second device")
	other := activeViewer()
	first, _ := broker.Subscribe(player, "")
	second, _ := broker.Subscribe(secondDevice, "")
	untouched, _ := broker.Subscribe(other, "")

	broker.CloseSession(player.SessionTokenHash)
	assertClosed(t, first, true)
	assertClosed(t, second, false)

	broker.CloseMember(player.MemberID)
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
	slow, _ := broker.Subscribe(activeViewer(), "")
	for range subscriptionBufferSize + 1 {
		broker.Publish(TopicSettings)
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
	dm := activeViewer()
	dm.IsDM = true
	_, catchUp := broker.Subscribe(player, "")
	lastID := catchUp.ID

	broker.publishToSession(dm.SessionTokenHash, AllTopics()...)
	broker.publishToMember(uuid.New(), TopicMe)
	if _, resume := broker.Subscribe(player, lastID); resume != nil {
		t.Fatalf("a targeted publish to someone else invalidated my resume: %+v", resume)
	}

	broker.Publish(TopicSettings)
	broker.publishToMember(player.MemberID, TopicMe)
	_, resume := broker.Subscribe(player, lastID)
	if resume == nil || fmt.Sprint(resume.Data.Topics) != "[me settings]" || resume.ID != broker.currentIDLocked() {
		t.Fatalf("resume = %+v, want [me settings] at the newest ID", resume)
	}

	// The same session hears its own targeted publish.
	_, dmResume := broker.Subscribe(dm, lastID)
	if dmResume == nil || fmt.Sprint(dmResume.Data.Topics) != fmt.Sprint(AllTopics()) {
		t.Fatalf("the DM's session resume = %+v, want every topic", dmResume)
	}

	// A pending stream hears only its own me topic, even when broadcasts went by.
	pending := pendingViewer()
	_, pendingCatchUp := broker.Subscribe(pending, "")
	broker.Publish(TopicMembers)
	if _, pendingResume := broker.Subscribe(pending, pendingCatchUp.ID); pendingResume != nil {
		t.Fatalf("a pending stream's resume after a broadcast = %+v, want nothing", pendingResume)
	}
}

func TestResumeFallsBackToEverythingWhenTheRecentMessagesDoNotReachBack(t *testing.T) {
	broker := NewBroker()
	player := activeViewer()
	_, catchUp := broker.Subscribe(player, "")
	for range recentMessageLimit + 1 {
		broker.publishToMember(uuid.New(), TopicMe)
	}
	_, resume := broker.Subscribe(player, catchUp.ID)
	if resume == nil || fmt.Sprint(resume.Data.Topics) != fmt.Sprint(AllTopics()) {
		t.Fatalf("resume past the recent messages = %+v, want every topic", resume)
	}
	for _, lastEventID := range []string{"garbage", broker.bootID + "-x", broker.bootID + "-999999"} {
		if _, fallback := broker.Subscribe(player, lastEventID); fallback == nil || len(fallback.Data.Topics) != len(AllTopics()) {
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
