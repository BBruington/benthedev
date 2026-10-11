package core

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strconv"
	"sync"

	"github.com/google/uuid"
)

// Topic names a group of queries the client refetches. Messages carry only topics, never data, so a
// stream message can't leak anything (spec §8.8). Later milestones add topics; the web client maps
// each one to query-key prefixes.
type Topic string

const (
	// TopicMe reaches pending and removed sessions too; it is how /pending opens live.
	TopicMe       Topic = "me"
	TopicMembers  Topic = "members"
	TopicSettings Topic = "settings"
)

// InvalidateMessage is the data of every `event: invalidate` message on GET /api/stream.
type InvalidateMessage struct {
	Topics []Topic `json:"topics"`
}

// AllTopics lists every topic; it feeds the Topic enum in the OpenAPI document.
func AllTopics() []Topic {
	return []Topic{TopicMe, TopicMembers, TopicSettings}
}

// StreamMessage is one `event: invalidate` message, with its `<bootId>-<seq>` id.
type StreamMessage struct {
	ID   string
	Data InvalidateMessage
}

// A stream that falls this far behind is closed rather than blocking publishers; its client
// reconnects and, having missed messages, gets one invalidate for everything.
const subscriptionBufferSize = 32

// Broker fans invalidations out to open streams. It lives in-process (single replica, spec §8.8) and
// touches no database: reconnects and heartbeats cost nothing.
type Broker struct {
	mutex         sync.Mutex
	bootID        string
	sequence      uint64
	subscriptions map[*Subscription]struct{}
	// memberActive is the newest status this process gave each member. A stream that subscribes with a
	// Viewer read just before an approval or removal still gets the scope the member has now.
	memberActive map[uuid.UUID]bool
}

func NewBroker() *Broker {
	bootBytes := make([]byte, 6)
	_, _ = rand.Read(bootBytes)
	return &Broker{
		bootID:        hex.EncodeToString(bootBytes),
		subscriptions: map[*Subscription]struct{}{},
		memberActive:  map[uuid.UUID]bool{},
	}
}

// Subscription is one open stream. Pending and removed members' streams are not active: they receive
// only their own `me` topic.
type Subscription struct {
	broker     *Broker
	memberID   uuid.UUID
	sessionKey string
	active     bool
	messages   chan StreamMessage
	closed     chan struct{}
	closeOnce  sync.Once
}

func (subscription *Subscription) Messages() <-chan StreamMessage {
	return subscription.messages
}

// Closed is closed when the broker ends the stream: the member was removed, signed out, or fell behind.
func (subscription *Subscription) Closed() <-chan struct{} {
	return subscription.closed
}

// Close unregisters the stream; the handler defers it.
func (subscription *Subscription) Close() {
	subscription.broker.mutex.Lock()
	defer subscription.broker.mutex.Unlock()
	subscription.broker.closeLocked(subscription)
}

func (subscription *Subscription) visibleTopics(topics []Topic) []Topic {
	if subscription.active {
		return topics
	}
	if slices.Contains(topics, TopicMe) {
		return []Topic{TopicMe}
	}
	return nil
}

func (broker *Broker) currentIDLocked() string {
	return broker.bootID + "-" + strconv.FormatUint(broker.sequence, 10)
}

// Subscribe registers a stream. When lastEventID is not the newest message ID (the client missed
// something, comes from an earlier boot, or has no ID yet), it also returns one invalidate for
// everything the stream may see; when it is the newest, nothing happened, so nothing is sent.
func (broker *Broker) Subscribe(viewer Viewer, lastEventID string) (*Subscription, *StreamMessage) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	active := viewer.IsActive()
	if recorded, known := broker.memberActive[viewer.MemberID]; known && !viewer.IsDM {
		active = recorded
	}
	subscription := &Subscription{
		broker:     broker,
		memberID:   viewer.MemberID,
		sessionKey: string(viewer.SessionTokenHash),
		active:     active,
		messages:   make(chan StreamMessage, subscriptionBufferSize),
		closed:     make(chan struct{}),
	}
	broker.subscriptions[subscription] = struct{}{}

	currentID := broker.currentIDLocked()
	if lastEventID == currentID {
		return subscription, nil
	}
	return subscription, &StreamMessage{
		ID:   currentID,
		Data: InvalidateMessage{Topics: subscription.visibleTopics(AllTopics())},
	}
}

// Publish tells every active stream to refetch these topics. Pending and removed streams never hear
// a broadcast.
func (broker *Broker) Publish(topics ...Topic) {
	broker.publish(topics, func(subscription *Subscription) bool { return subscription.active })
}

func (broker *Broker) publishToMember(memberID uuid.UUID, topics ...Topic) {
	broker.publish(topics, func(subscription *Subscription) bool { return subscription.memberID == memberID })
}

func (broker *Broker) publishToSession(tokenHash []byte, topics ...Topic) {
	sessionKey := string(tokenHash)
	broker.publish(topics, func(subscription *Subscription) bool { return subscription.sessionKey == sessionKey })
}

func (broker *Broker) publish(topics []Topic, include func(*Subscription) bool) {
	if len(topics) == 0 {
		return
	}
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.sequence++
	messageID := broker.currentIDLocked()
	for subscription := range broker.subscriptions {
		if !include(subscription) {
			continue
		}
		visible := subscription.visibleTopics(topics)
		if len(visible) == 0 {
			continue
		}
		select {
		case subscription.messages <- StreamMessage{ID: messageID, Data: InvalidateMessage{Topics: visible}}:
		default:
			broker.closeLocked(subscription)
		}
	}
}

// memberStatusChanged rescopes a member's open streams after approve, deny, remove or re-admit.
func (broker *Broker) memberStatusChanged(memberID uuid.UUID, active bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.memberActive[memberID] = active
	for subscription := range broker.subscriptions {
		if subscription.memberID == memberID {
			subscription.active = active
		}
	}
}

// CloseMember ends every stream of a member: removal and sign out everywhere.
func (broker *Broker) CloseMember(memberID uuid.UUID) {
	broker.closeWhere(func(subscription *Subscription) bool { return subscription.memberID == memberID })
}

// CloseSession ends the streams of one web session: sign out.
func (broker *Broker) CloseSession(tokenHash []byte) {
	sessionKey := string(tokenHash)
	broker.closeWhere(func(subscription *Subscription) bool { return subscription.sessionKey == sessionKey })
}

func (broker *Broker) closeWhere(match func(*Subscription) bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for subscription := range broker.subscriptions {
		if match(subscription) {
			broker.closeLocked(subscription)
		}
	}
}

func (broker *Broker) closeLocked(subscription *Subscription) {
	delete(broker.subscriptions, subscription)
	subscription.closeOnce.Do(func() { close(subscription.closed) })
}

// OpenStreams counts open streams; tests use it to prove a removal closed them.
func (broker *Broker) OpenStreams() int {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return len(broker.subscriptions)
}

// Subscribe opens a live-update stream for GET /api/stream; see Broker.Subscribe.
func (service *Service) Subscribe(viewer Viewer, lastEventID string) (*Subscription, *StreamMessage) {
	return service.broker.Subscribe(viewer, lastEventID)
}

// Broker is the in-process fan-out every operation publishes to after it commits.
func (service *Service) Broker() *Broker {
	return service.broker
}
