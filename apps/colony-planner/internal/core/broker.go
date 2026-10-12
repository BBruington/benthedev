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

// InvalidateMessage is the data of every `event: invalidate` message on a campaign stream.
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

// recipients is who a published message reached, always within one campaign: every active stream
// of the campaign, every stream of the campaign (a suspension), one membership's streams or one
// session's streams there. It is kept with the message so a resume can tell what the reconnecting
// stream would have heard.
type recipients struct {
	everyStream   bool
	activeStreams bool
	memberID      uuid.UUID
	sessionKey    string
}

func (scope recipients) include(subscription *Subscription) bool {
	switch {
	case scope.everyStream:
		return true
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

// campaignLog is one campaign's message history. Sequence numbers count only that campaign's
// messages, so a player learns nothing of other campaigns' activity, and a busy campaign never pushes
// another's messages out of its ring.
type campaignLog struct {
	sequence uint64
	// recent holds the last recentMessageLimit published messages, oldest first, for resumes.
	recent []publishedMessage
}

// Broker fans invalidations out to open streams. It lives in-process (single replica, spec §8.8) and
// touches no database: reconnects and heartbeats cost nothing. Every stream belongs to one campaign,
// and there is no unscoped broadcast.
type Broker struct {
	mutex         sync.Mutex
	bootID        string
	logs          map[uuid.UUID]*campaignLog
	subscriptions map[*Subscription]struct{}
	// memberActive is the newest status this process gave each membership, and campaignActive the
	// newest status of each campaign. A stream that subscribes with a Viewer read just before an
	// approval, removal or suspension still gets the scope it has now.
	memberActive   map[uuid.UUID]bool
	campaignActive map[uuid.UUID]bool
}

func NewBroker() *Broker {
	bootBytes := make([]byte, 6)
	_, _ = rand.Read(bootBytes)
	return &Broker{
		bootID:         hex.EncodeToString(bootBytes),
		logs:           map[uuid.UUID]*campaignLog{},
		subscriptions:  map[*Subscription]struct{}{},
		memberActive:   map[uuid.UUID]bool{},
		campaignActive: map[uuid.UUID]bool{},
	}
}

// Subscription is one open stream in one campaign. Pending and removed members' streams, and every
// stream of a suspended campaign, are not active: they receive only their own `me` topic.
type Subscription struct {
	broker     *Broker
	campaignID uuid.UUID
	memberID   uuid.UUID
	accountID  uuid.UUID
	sessionKey string
	memberOK   bool
	campaignOK bool
	active     bool
	messages   chan StreamMessage
	closed     chan struct{}
	closeOnce  sync.Once
}

func (subscription *Subscription) Messages() <-chan StreamMessage {
	return subscription.messages
}

// Closed is closed when the broker ends the stream: the member signed out, the campaign was deleted,
// or the stream fell behind.
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

func (subscription *Subscription) rescope() {
	subscription.active = subscription.memberOK && subscription.campaignOK
}

func (broker *Broker) logLocked(campaignID uuid.UUID) *campaignLog {
	log, found := broker.logs[campaignID]
	if !found {
		log = &campaignLog{}
		broker.logs[campaignID] = log
	}
	return log
}

func (broker *Broker) currentIDLocked(log *campaignLog) string {
	return broker.bootID + "-" + strconv.FormatUint(log.sequence, 10)
}

// Subscribe registers a stream in the Viewer's campaign. With the campaign's newest message ID as
// lastEventID nothing happened, so nothing is sent. With an older ID from this boot that the campaign's
// recent messages still reach back to, it returns one invalidate for the topics this stream would have
// heard since then, or nothing when none of those messages were for it. Otherwise (no ID, another boot,
// too far behind) it returns one invalidate for everything the stream may see. The zero Viewer gets
// nothing: there is no stream outside a campaign.
func (broker *Broker) Subscribe(viewer Viewer, lastEventID string) (*Subscription, *StreamMessage, error) {
	if err := requireMembership(viewer); err != nil {
		return nil, nil, err
	}
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	subscription := &Subscription{
		broker:     broker,
		campaignID: viewer.campaignID,
		memberID:   viewer.memberID,
		accountID:  viewer.accountID,
		sessionKey: string(viewer.sessionTokenHash),
		memberOK:   viewer.IsActive(),
		campaignOK: viewer.CampaignActive(),
		messages:   make(chan StreamMessage, subscriptionBufferSize),
		closed:     make(chan struct{}),
	}
	if recorded, known := broker.memberActive[viewer.memberID]; known && !viewer.IsDM() {
		subscription.memberOK = recorded
	}
	if recorded, known := broker.campaignActive[viewer.campaignID]; known {
		subscription.campaignOK = recorded
	}
	subscription.rescope()
	broker.subscriptions[subscription] = struct{}{}

	log := broker.logLocked(viewer.campaignID)
	currentID := broker.currentIDLocked(log)
	if lastEventID == currentID {
		return subscription, nil, nil
	}
	missed := subscription.visibleTopics(broker.missedTopicsLocked(log, subscription, lastEventID))
	if len(missed) == 0 {
		return subscription, nil, nil
	}
	return subscription, &StreamMessage{ID: currentID, Data: InvalidateMessage{Topics: missed}}, nil
}

// missedTopicsLocked is the union of the topics published to this subscription's recipients after
// lastEventID, or every topic when the campaign's recent messages can't answer that.
func (broker *Broker) missedTopicsLocked(log *campaignLog, subscription *Subscription, lastEventID string) []Topic {
	bootID, sequenceText, found := strings.Cut(lastEventID, "-")
	lastSequence, err := strconv.ParseUint(sequenceText, 10, 64)
	if !found || err != nil || bootID != broker.bootID || lastSequence > log.sequence {
		return AllTopics()
	}
	if len(log.recent) == 0 || log.recent[0].sequence > lastSequence+1 {
		return AllTopics()
	}
	missed := map[Topic]bool{}
	for _, message := range log.recent {
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

// Publish tells every active stream of one campaign to refetch these topics. Pending and removed
// streams never hear a broadcast, and no other campaign hears anything.
func (broker *Broker) Publish(campaignID uuid.UUID, topics ...Topic) {
	broker.publish(campaignID, topics, recipients{activeStreams: true})
}

func (broker *Broker) publishToMember(campaignID uuid.UUID, memberID uuid.UUID, topics ...Topic) {
	broker.publish(campaignID, topics, recipients{memberID: memberID})
}

func (broker *Broker) publishToEveryStream(campaignID uuid.UUID, topics ...Topic) {
	broker.publish(campaignID, topics, recipients{everyStream: true})
}

// publishToSession reaches one web session's streams in every campaign where it has one; the message
// goes into each of those campaigns' logs.
func (broker *Broker) publishToSession(tokenHash []byte, topics ...Topic) {
	sessionKey := string(tokenHash)
	broker.mutex.Lock()
	campaigns := map[uuid.UUID]bool{}
	for subscription := range broker.subscriptions {
		if subscription.sessionKey == sessionKey {
			campaigns[subscription.campaignID] = true
		}
	}
	broker.mutex.Unlock()
	for campaignID := range campaigns {
		broker.publish(campaignID, topics, recipients{sessionKey: sessionKey})
	}
}

func (broker *Broker) publish(campaignID uuid.UUID, topics []Topic, scope recipients) {
	if len(topics) == 0 || campaignID == uuid.Nil {
		return
	}
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	log := broker.logLocked(campaignID)
	log.sequence++
	messageID := broker.currentIDLocked(log)
	if len(log.recent) == recentMessageLimit {
		log.recent = slices.Delete(log.recent, 0, 1)
	}
	log.recent = append(log.recent, publishedMessage{sequence: log.sequence, topics: slices.Clone(topics), recipients: scope})
	for subscription := range broker.subscriptions {
		if subscription.campaignID != campaignID || !scope.include(subscription) {
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

// memberStatusChanged rescopes a membership's open streams after approve, deny, remove, re-admit or
// leave.
func (broker *Broker) memberStatusChanged(memberID uuid.UUID, active bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.memberActive[memberID] = active
	for subscription := range broker.subscriptions {
		if subscription.memberID == memberID {
			subscription.memberOK = active
			subscription.rescope()
		}
	}
}

// campaignStatusChanged rescopes every stream of a campaign after suspend or unsuspend.
func (broker *Broker) campaignStatusChanged(campaignID uuid.UUID, active bool) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	broker.campaignActive[campaignID] = active
	for subscription := range broker.subscriptions {
		if subscription.campaignID == campaignID {
			subscription.campaignOK = active
			subscription.rescope()
		}
	}
}

// CloseCampaign ends every stream of a deleted campaign and drops its log; open tabs reconnect, get
// 404 and show "No such campaign".
func (broker *Broker) CloseCampaign(campaignID uuid.UUID) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	for subscription := range broker.subscriptions {
		if subscription.campaignID == campaignID {
			broker.closeLocked(subscription)
		}
	}
	delete(broker.logs, campaignID)
	delete(broker.campaignActive, campaignID)
}

// CloseAccount ends every stream of an account in every campaign: sign out everywhere.
func (broker *Broker) CloseAccount(accountID uuid.UUID) {
	broker.closeWhere(func(subscription *Subscription) bool { return subscription.accountID == accountID })
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

// OpenStreams counts open streams; tests use it to prove a deletion closed them.
func (broker *Broker) OpenStreams() int {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	return len(broker.subscriptions)
}

// Subscribe opens a live-update stream for GET /api/campaigns/{campaignId}/stream; see
// Broker.Subscribe. Any membership may subscribe (pending level); its scope follows its status.
func (service *Service) Subscribe(viewer Viewer, lastEventID string) (*Subscription, *StreamMessage, error) {
	return service.broker.Subscribe(viewer, lastEventID)
}

// Broker is the in-process fan-out every operation publishes to after it commits.
func (service *Service) Broker() *Broker {
	return service.broker
}
