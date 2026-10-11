package core

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
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
	// TopicColony is the board: day, shipments, stock, headcounts, names and the undo slot.
	TopicColony Topic = "colony"
	// TopicTimeline is every timeline read, including the recent entries the toasts come from.
	TopicTimeline Topic = "timeline"
	// TopicResources is the DM's resource settings on /settings.
	TopicResources Topic = "resources"
)

// InvalidateMessage is the data of every `event: invalidate` message on GET /api/stream.
type InvalidateMessage struct {
	Topics []Topic `json:"topics"`
}

// AllTopics lists every topic; it feeds the Topic enum in the OpenAPI document, reconnect catch-up
// and view as player. TestAllTopicsListsEveryTopicConstant fails when a Topic constant is missing here.
func AllTopics() []Topic {
	return []Topic{TopicMe, TopicMembers, TopicSettings, TopicColony, TopicTimeline, TopicResources}
}

// StreamMessage is one `event: invalidate` message, with its `<bootId>-<seq>` id.
type StreamMessage struct {
	ID   string
	Data InvalidateMessage
}

// A stream that falls this far behind is closed rather than blocking publishers; its client
// reconnects and catches up on what it missed.
const subscriptionBufferSize = 32

// recentMessageLimit is how many published messages the broker remembers for resumes. A client that
// was away for more messages than this gets one invalidate for everything.
const recentMessageLimit = 256

// recipients is who a published message reached: every active stream, one member's streams or one
// session's streams. It is kept with the message so a resume can tell what the reconnecting stream
// would have heard.
type recipients struct {
	activeStreams bool
	memberID      uuid.UUID
	sessionKey    string
}

func (scope recipients) include(subscription *Subscription) bool {
	switch {
	case scope.activeStreams:
		return subscription.active
	case scope.sessionKey != "":
		return subscription.sessionKey == scope.sessionKey
	default:
		return subscription.memberID == scope.memberID
	}
}

type publishedMessage struct {
	sequence   uint64
	topics     []Topic
	recipients recipients
}

// Broker fans invalidations out to open streams. It lives in-process (single replica, spec §8.8) and
// touches no database: reconnects and heartbeats cost nothing.
type Broker struct {
	mutex         sync.Mutex
	bootID        string
	sequence      uint64
	subscriptions map[*Subscription]struct{}
	// recent holds the last recentMessageLimit published messages, oldest first, for resumes.
	recent []publishedMessage
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

// Subscribe registers a stream. With the newest message ID as lastEventID nothing happened, so
// nothing is sent. With an older ID from this boot that the recent messages still reach back to, it
// returns one invalidate for the topics this stream would have heard since then, or nothing when none
// of those messages were for it (a view-as-player switch in the DM's browser, a message to another
// member). Otherwise (no ID, another boot, too far behind) it returns one invalidate for everything
// the stream may see.
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
	missed := subscription.visibleTopics(broker.missedTopicsLocked(subscription, lastEventID))
	if len(missed) == 0 {
		return subscription, nil
	}
	return subscription, &StreamMessage{ID: currentID, Data: InvalidateMessage{Topics: missed}}
}

// missedTopicsLocked is the union of the topics published to this subscription's recipients after
// lastEventID, or every topic when the recent messages can't answer that.
func (broker *Broker) missedTopicsLocked(subscription *Subscription, lastEventID string) []Topic {
	bootID, sequenceText, found := strings.Cut(lastEventID, "-")
	lastSequence, err := strconv.ParseUint(sequenceText, 10, 64)
	if !found || err != nil || bootID != broker.bootID || lastSequence > broker.sequence {
		return AllTopics()
	}
	if len(broker.recent) == 0 || broker.recent[0].sequence > lastSequence+1 {
		return AllTopics()
	}
	missed := map[Topic]bool{}
	for _, message := range broker.recent {
		if message.sequence > lastSequence && message.recipients.include(subscription) {
			for _, topic := range message.topics {
				missed[topic] = true
			}
		}
	}
	topics := make([]Topic, 0, len(missed))
	for _, topic := range AllTopics() {
		if missed[topic] {
			topics = append(topics, topic)
		}
	}
	return topics
}

// Publish tells every active stream to refetch these topics. Pending and removed streams never hear
// a broadcast.
func (broker *Broker) Publish(topics ...Topic) {
	broker.publish(topics, recipients{activeStreams: true})
}

func (broker *Broker) publishToMember(memberID uuid.UUID, topics ...Topic) {
	broker.publish(topics, recipients{memberID: memberID})
}

func (broker *Broker) publishToSession(tokenHash []byte, topics ...Topic) {
	broker.publish(topics, recipients{sessionKey: string(tokenHash)})
}

func (broker *Broker) publish(topics []Topic, scope recipients) {
	if len(topics) == 0 {
		return
	}
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.sequence++
	messageID := broker.currentIDLocked()
	if len(broker.recent) == recentMessageLimit {
		broker.recent = slices.Delete(broker.recent, 0, 1)
	}
	broker.recent = append(broker.recent, publishedMessage{sequence: broker.sequence, topics: slices.Clone(topics), recipients: scope})
	for subscription := range broker.subscriptions {
		if !scope.include(subscription) {
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
