package core

import (
	"fmt"
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
