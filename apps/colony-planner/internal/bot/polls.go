package bot

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/game"
	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
)

const (
	// defaultPollVoteDelay gathers a burst of vote events into one read of the voters.
	defaultPollVoteDelay = 2 * time.Second
	// pollVotersPage is the most voters Discord lists per request.
	pollVotersPage = 100
	// pollVoterPagesPerAnswer caps one answer's read at 5,000 voters, far past any campaign's server.
	pollVoterPagesPerAnswer = 50
	// pollVoteSyncTimeout bounds one read of every answer's voters and its write.
	pollVoteSyncTimeout = 30 * time.Second
	// pollMaxDurationHours is Discord's longest poll, 32 days.
	pollMaxDurationHours = 768
)

// pollWork is one poll's running worker; dirty means another change arrived meanwhile.
type pollWork struct {
	dirty bool
}

// pollDeadline is an open poll's closes_at the planner closes it at; retryAt delays a failed close.
type pollDeadline struct {
	campaignID uuid.UUID
	closesAt   time.Time
	retryAt    time.Time
	// sequence orders this entry against a planner reload that read the database before it was set.
	sequence uint64
}

// pollPostRetry is when the planner tries again to post a poll whose post failed, and the wait after.
type pollPostRetry struct {
	campaignID uuid.UUID
	at         time.Time
	delay      time.Duration
}

// pollMessage is the open campaign poll a posted Discord message holds, so vote events for any other
// message cost no database read.
type pollMessage struct {
	guildID    string
	campaignID uuid.UUID
	pollID     uuid.UUID
}

// pollLock serializes one poll's vote syncs and its close; holders lets the last one drop it.
type pollLock struct {
	mutex   sync.Mutex
	holders int
}

// PollNotifier is what core tells after poll writes, or nil while Beny Bot is off, so core falls back
// to its no-op notifier.
func (benyBot *Bot) PollNotifier() core.PollNotifier {
	if !benyBot.Enabled() {
		return nil
	}
	return benyBot
}

// PollChanged implements core.PollNotifier. One worker per poll posts it, so two racing changes never
// post twice.
func (benyBot *Bot) PollChanged(campaignID uuid.UUID, pollID uuid.UUID) {
	benyBot.startPollWorker(benyBot.pollWorkers, "poll", pollID, func(service colonyService) {
		benyBot.syncPoll(service, campaignID, pollID)
	})
}

// requestPollVoteSync reads a poll's voters from Discord shortly, once for every burst of vote events.
func (benyBot *Bot) requestPollVoteSync(campaignID uuid.UUID, pollID uuid.UUID) {
	benyBot.startPollWorker(benyBot.pollVoteWorkers, "poll votes", pollID, func(service colonyService) {
		if !benyBot.pause(benyBot.pollVoteDelay) {
			return
		}
		// Votes that arrived during the pause are in the read that follows.
		benyBot.pollMutex.Lock()
		benyBot.pollVoteWorkers[pollID].dirty = false
		benyBot.pollMutex.Unlock()
		unlock := benyBot.lockPoll(pollID)
		syncContext, cancel := benyBot.untilStopping(pollVoteSyncTimeout)
		err := benyBot.syncPollVotes(syncContext, service, campaignID, pollID)
		cancel()
		unlock()
		if err != nil {
			benyBot.options.Logger.Warn("Beny Bot couldn't sync a poll's votes", slog.String("campaign_id", campaignID.String()), slog.String("poll_id", pollID.String()), slog.Any("error", err))
		}
	})
}

// startPollWorker runs work for the poll in the background, or, when that poll's worker in workers is
// already running, has it run work once more after it finishes.
func (benyBot *Bot) startPollWorker(workers map[uuid.UUID]*pollWork, what string, pollID uuid.UUID, work func(service colonyService)) {
	if !benyBot.Enabled() {
		return
	}
	benyBot.mutex.Lock()
	defer benyBot.mutex.Unlock()
	if benyBot.service == nil || benyBot.stopping {
		benyBot.options.Logger.Warn("Beny Bot isn't running; skipped a post", slog.String("post", what))
		return
	}
	service := benyBot.service
	benyBot.pollMutex.Lock()
	defer benyBot.pollMutex.Unlock()
	if running, found := workers[pollID]; found {
		running.dirty = true
		return
	}
	workers[pollID] = &pollWork{}
	benyBot.posts.Go(func() {
		for {
			work(service)
			benyBot.pollMutex.Lock()
			running := workers[pollID]
			if !running.dirty {
				delete(workers, pollID)
				benyBot.pollMutex.Unlock()
				return
			}
			running.dirty = false
			benyBot.pollMutex.Unlock()
		}
	})
}

// pause waits for delay; false means the bot began stopping.
func (benyBot *Bot) pause(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-benyBot.stopSignal:
		return false
	}
}

// untilStopping bounds background poll work by timeout and by the bot's life, so stop never waits it out.
func (benyBot *Bot) untilStopping(timeout time.Duration) (context.Context, context.CancelFunc) {
	workContext, cancel := context.WithTimeout(context.Background(), timeout)
	go func() {
		select {
		case <-benyBot.stopSignal:
			cancel()
		case <-workContext.Done():
		}
	}()
	return workContext, cancel
}

// lockPoll takes the poll's lock, so a vote sync and the deadline close never interleave their read of
// Discord's voters and their write; the returned function releases it.
func (benyBot *Bot) lockPoll(pollID uuid.UUID) func() {
	benyBot.pollMutex.Lock()
	lock := benyBot.pollLocks[pollID]
	if lock == nil {
		lock = &pollLock{}
		benyBot.pollLocks[pollID] = lock
	}
	lock.holders++
	benyBot.pollMutex.Unlock()
	lock.mutex.Lock()
	return func() {
		lock.mutex.Unlock()
		benyBot.pollMutex.Lock()
		lock.holders--
		if lock.holders == 0 {
			delete(benyBot.pollLocks, pollID)
		}
		benyBot.pollMutex.Unlock()
	}
}

// syncPoll brings the deadline timer up to date with the poll as it is now, posts an open DISCORD poll
// that has no message yet, and ends a posted one the DM closed early.
func (benyBot *Bot) syncPoll(service colonyService, campaignID uuid.UUID, pollID uuid.UUID) {
	readContext, cancel := benyBot.untilStopping(postTimeout)
	defer cancel()
	logger := benyBot.options.Logger.With(slog.String("campaign_id", campaignID.String()), slog.String("poll_id", pollID.String()))
	target, found, err := service.DiscordTargetFor(readContext, campaignID)
	if err != nil {
		logger.Error("Beny Bot couldn't read the campaign's Discord server", slog.Any("error", err))
		return
	}
	if !found {
		benyBot.forgetPollDeadline(pollID)
		return
	}
	discordPoll, err := service.PollForDiscord(readContext, campaignID, pollID)
	if isNotFound(err) {
		// A SITE poll isn't Beny Bot's to post, but the planner still closes it at its deadline.
		benyBot.refreshPollDeadline(readContext, logger, service, campaignID, pollID)
		return
	}
	if err != nil {
		logger.Error("Beny Bot couldn't read a poll", slog.Any("error", err))
		return
	}
	poll := discordPoll.Poll
	if poll.CloseRecorded {
		// Only a poll the timer still tracked was open in Discord, so each early close ends it once.
		tracked := benyBot.forgetPollDeadline(pollID)
		if tracked && discordPoll.MessageID != "" && poll.ClosedAt != nil && poll.ClosedAt.Before(poll.ClosesAt) {
			benyBot.expirePoll(readContext, logger, discordPoll)
		}
		return
	}
	// A deadline already past but unstored is due at once, so the planner closes it right away.
	benyBot.setPollDeadline(pollID, campaignID, poll.ClosesAt)
	if discordPoll.MessageID != "" {
		benyBot.clearPollPostRetry(pollID)
		if discordPoll.GuildID == target.GuildID {
			benyBot.rememberPollMessage(discordPoll.MessageID, pollMessage{guildID: discordPoll.GuildID, campaignID: campaignID, pollID: pollID})
		}
		return
	}
	if poll.Closed {
		benyBot.clearPollPostRetry(pollID)
		return
	}
	benyBot.postPoll(readContext, logger, service, campaignID, target, discordPoll)
}

// refreshPollDeadline tracks the poll's deadline from the campaign's open polls, or forgets it.
func (benyBot *Bot) refreshPollDeadline(readContext context.Context, logger *slog.Logger, service colonyService, campaignID uuid.UUID, pollID uuid.UUID) {
	deadlines, err := service.PollDeadlines(readContext, campaignID)
	if err != nil {
		logger.Error("Beny Bot couldn't read a campaign's poll deadlines", slog.Any("error", err))
		return
	}
	for _, deadline := range deadlines {
		if deadline.PollID == pollID {
			benyBot.setPollDeadline(pollID, campaignID, deadline.ClosesAt)
			return
		}
	}
	benyBot.forgetPollDeadline(pollID)
}

// postPoll posts the poll as a native Discord poll and records its message and answer IDs.
func (benyBot *Bot) postPoll(postContext context.Context, logger *slog.Logger, service colonyService, campaignID uuid.UUID, target core.DiscordTarget, discordPoll core.DiscordPoll) {
	poll := discordPoll.Poll
	duration, open := pollDurationHours(poll.ClosesAt, benyBot.now())
	if !open {
		return
	}
	answers := make([]discordgo.PollAnswer, 0, len(poll.Options))
	for _, option := range poll.Options {
		answers = append(answers, discordgo.PollAnswer{Media: &discordgo.PollMedia{Text: option.Label}})
	}
	posted, failed := benyBot.postToCampaign(postContext, service, campaignID, announceChannel, &discordgo.MessageSend{
		Content: pollAnnouncement(benyBot.pollsURL(target.CampaignSlug)),
		Poll: &discordgo.Poll{
			Question:         discordgo.PollMedia{Text: poll.Question},
			Answers:          answers,
			AllowMultiselect: poll.AllowMultiple,
			Duration:         duration,
		},
	}, nil)
	if posted == nil {
		if failed {
			retryIn := benyBot.schedulePollPostRetry(campaignID, poll.ID)
			logger.Warn("Beny Bot couldn't post a poll; it tries again later", slog.Duration("retry_in", retryIn))
			return
		}
		benyBot.clearPollPostRetry(poll.ID)
		logger.Info("Beny Bot skipped a poll's post: the campaign has no announcement channel it may use")
		return
	}
	benyBot.clearPollPostRetry(poll.ID)
	// The message is in Discord now, so stopping mustn't cut off its record or its deletion.
	recordContext, cancel := context.WithTimeout(context.WithoutCancel(postContext), postTimeout)
	defer cancel()
	input := core.RecordPollMessageInput{
		GuildID:   target.GuildID,
		ChannelID: posted.ChannelID,
		MessageID: posted.ID,
		Expiry:    benyBot.now().Add(time.Duration(duration) * time.Hour),
	}
	if posted.Poll != nil {
		if posted.Poll.Expiry != nil {
			input.Expiry = posted.Poll.Expiry.UTC()
		}
		for _, answer := range posted.Poll.Answers {
			input.AnswerIDs = append(input.AnswerIDs, answer.AnswerID)
		}
	}
	err := errPollAnswersMissing
	if len(input.AnswerIDs) == len(poll.Options) {
		err = service.RecordPollMessage(recordContext, campaignID, poll.ID, input)
	}
	if err != nil {
		// An unrecorded poll would never count its votes or be deleted, so it goes rather than linger.
		lostToAnother := errors.Is(err, core.ErrPollAlreadyPosted) || errors.Is(err, core.ErrPollClosed)
		switch {
		case errors.Is(err, core.ErrPollAlreadyPosted):
			logger.Info("another process posted this poll first; Beny Bot deletes its duplicate message")
		case errors.Is(err, core.ErrPollClosed):
			logger.Info("the poll closed before Beny Bot recorded its message; Beny Bot deletes the message")
		default:
			logger.Warn("Beny Bot couldn't record a poll's message; it deletes the message", slog.Any("error", err))
		}
		benyBot.deletePollMessage(recordContext, logger, posted.ChannelID, posted.ID)
		if lostToAnother {
			// This worker runs syncPoll again, which caches the winning message and its recorded deadline.
			benyBot.PollChanged(campaignID, poll.ID)
		}
		return
	}
	benyBot.setPollDeadline(poll.ID, campaignID, input.Expiry)
	benyBot.rememberPollMessage(posted.ID, pollMessage{guildID: target.GuildID, campaignID: campaignID, pollID: poll.ID})
}

var (
	errPollAnswersMissing = errors.New("discord returned a poll without one answer per option")
	errPollVotersStuck    = errors.New("discord returned a full page of poll voters that doesn't advance")
)

// pollDurationHours is the whole hours, within Discord's limits, that end the poll at or just after
// closesAt; open is false once closesAt has passed.
func pollDurationHours(closesAt time.Time, now time.Time) (hours int, open bool) {
	if !now.Before(closesAt) {
		return 0, false
	}
	return min(max(int(math.Ceil(closesAt.Sub(now).Hours())), 1), pollMaxDurationHours), true
}

func pollAnnouncement(pollsURL string) string {
	if pollsURL == "" {
		return "New poll: vote below."
	}
	return "New poll: vote below. Every poll and its results: " + pollsURL
}

// expirePoll ends a posted poll the DM closed before its deadline, best effort.
func (benyBot *Bot) expirePoll(postContext context.Context, logger *slog.Logger, discordPoll core.DiscordPoll) {
	endpoint := discordgo.EndpointPollExpire(discordPoll.ChannelID, discordPoll.MessageID)
	if _, err := benyBot.client.RequestWithBucketID("POST", endpoint, nil, endpoint, discordgo.WithContext(postContext)); err != nil {
		logger.Warn("Beny Bot couldn't end a closed poll in Discord", slog.String("channel_id", discordPoll.ChannelID), slog.String("message_id", discordPoll.MessageID), slog.Any("error", err))
	}
}

// handlePollVote syncs the votes of the open campaign poll a vote event names. Only a message Beny Bot
// posted in that very server counts, so an event can't reach another campaign's poll.
func (benyBot *Bot) handlePollVote(guildID string, messageID string) {
	if guildID == "" || messageID == "" {
		return
	}
	benyBot.pollMutex.Lock()
	message, found := benyBot.pollMessages[messageID]
	benyBot.pollMutex.Unlock()
	if found && message.guildID == guildID {
		benyBot.requestPollVoteSync(message.campaignID, message.pollID)
	}
}

// syncPollVotes hands core every answer's complete voter list from Discord. It does nothing for a SITE
// poll, an unposted one, one whose close is stored, or one posted in a server the campaign has left.
// The caller holds the poll's lock.
func (benyBot *Bot) syncPollVotes(syncContext context.Context, service colonyService, campaignID uuid.UUID, pollID uuid.UUID) error {
	discordPoll, err := service.PollForDiscord(syncContext, campaignID, pollID)
	if isNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if discordPoll.MessageID == "" || discordPoll.Poll.CloseRecorded {
		return nil
	}
	target, found, err := service.DiscordTargetFor(syncContext, campaignID)
	if err != nil {
		return err
	}
	if !found || target.GuildID != discordPoll.GuildID {
		benyBot.forgetPollMessage(pollID)
		return nil
	}
	logger := benyBot.options.Logger.With(slog.String("campaign_id", campaignID.String()), slog.String("poll_id", pollID.String()))
	votes := make(map[int][]string, len(discordPoll.AnswerIDs))
	for _, answerID := range discordPoll.AnswerIDs {
		if answerID == 0 {
			continue
		}
		voterIDs, err := benyBot.pollAnswerVoters(syncContext, logger, discordPoll.ChannelID, discordPoll.MessageID, answerID)
		if err != nil {
			return err
		}
		votes[answerID] = voterIDs
	}
	return service.SyncDiscordPollVotes(syncContext, campaignID, pollID, votes)
}

// pollAnswerVoters reads every page of one answer's voters, up to pollVoterPagesPerAnswer pages.
func (benyBot *Bot) pollAnswerVoters(syncContext context.Context, logger *slog.Logger, channelID string, messageID string, answerID int) ([]string, error) {
	endpoint := discordgo.EndpointPollAnswerVoters(channelID, messageID, answerID)
	voterIDs := []string{}
	after := ""
	for range pollVoterPagesPerAnswer {
		query := url.Values{"limit": {strconv.Itoa(pollVotersPage)}}
		if after != "" {
			query.Set("after", after)
		}
		body, err := benyBot.client.RequestWithBucketID("GET", endpoint+"?"+query.Encode(), nil, endpoint, discordgo.WithContext(syncContext))
		if err != nil {
			return nil, err
		}
		var page struct {
			Users []*discordgo.User `json:"users"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		for _, user := range page.Users {
			if user != nil && user.ID != "" {
				voterIDs = append(voterIDs, user.ID)
			}
		}
		if len(page.Users) < pollVotersPage {
			return voterIDs, nil
		}
		last := page.Users[len(page.Users)-1]
		if last == nil || last.ID == "" || last.ID == after {
			return nil, errPollVotersStuck
		}
		after = last.ID
	}
	logger.Warn("Beny Bot read only the first voters of a poll's answer", slog.Int("answer_id", answerID), slog.Int("voters_read", len(voterIDs)))
	return voterIDs, nil
}

// PollDeleted implements core.PollNotifier: the deleted poll's message goes too, best effort.
func (benyBot *Bot) PollDeleted(campaignID uuid.UUID, channelID string, messageID string) {
	if channelID == "" || messageID == "" {
		return
	}
	benyBot.pollMutex.Lock()
	delete(benyBot.pollMessages, messageID)
	benyBot.pollMutex.Unlock()
	benyBot.inBackground(context.Background(), "poll deletion", func(postContext context.Context, _ colonyService) {
		logger := benyBot.options.Logger.With(slog.String("campaign_id", campaignID.String()))
		benyBot.deletePollMessage(postContext, logger, channelID, messageID)
	})
}

func (benyBot *Bot) deletePollMessage(postContext context.Context, logger *slog.Logger, channelID string, messageID string) {
	if err := benyBot.client.ChannelMessageDelete(channelID, messageID, discordgo.WithContext(postContext)); err != nil {
		logger.Info("Beny Bot couldn't delete a poll's message", slog.String("channel_id", channelID), slog.String("message_id", messageID), slog.Any("error", err))
	}
}

func (benyBot *Bot) pollsURL(campaignSlug string) string {
	if campaignSlug == "" || benyBot.options.PublicURL == "" {
		return ""
	}
	return benyBot.options.PublicURL + "/c/" + url.PathEscape(campaignSlug) + "/polls"
}

// setPollDeadline keeps the planner's timer for an open poll until its close is stored. A failed
// close's retry stays only while the deadline is unchanged.
func (benyBot *Bot) setPollDeadline(pollID uuid.UUID, campaignID uuid.UUID, closesAt time.Time) {
	benyBot.pollMutex.Lock()
	benyBot.pollDeadlineSequence++
	deadline := pollDeadline{campaignID: campaignID, closesAt: closesAt, sequence: benyBot.pollDeadlineSequence}
	if existing := benyBot.pollDeadlines[pollID]; existing.closesAt.Equal(closesAt) {
		deadline.retryAt = existing.retryAt
	}
	benyBot.pollDeadlines[pollID] = deadline
	benyBot.pollMutex.Unlock()
	benyBot.signalPollsChanged()
}

// forgetPollDeadline drops everything Beny Bot keeps for an open poll; tracked tells whether its
// deadline was still tracked.
func (benyBot *Bot) forgetPollDeadline(pollID uuid.UUID) (tracked bool) {
	benyBot.pollMutex.Lock()
	_, tracked = benyBot.pollDeadlines[pollID]
	delete(benyBot.pollDeadlines, pollID)
	delete(benyBot.pollPostRetries, pollID)
	benyBot.forgetPollMessageLocked(pollID)
	benyBot.pollMutex.Unlock()
	if tracked {
		benyBot.signalPollsChanged()
	}
	return tracked
}

// delayPollClose has the planner try a failed close again in a minute.
func (benyBot *Bot) delayPollClose(pollID uuid.UUID) {
	benyBot.pollMutex.Lock()
	defer benyBot.pollMutex.Unlock()
	if tracked, found := benyBot.pollDeadlines[pollID]; found {
		tracked.retryAt = benyBot.now().Add(firstRetryDelay)
		benyBot.pollDeadlines[pollID] = tracked
	}
}

func (benyBot *Bot) rememberPollMessage(messageID string, message pollMessage) {
	benyBot.pollMutex.Lock()
	benyBot.pollMessages[messageID] = message
	benyBot.pollMutex.Unlock()
}

func (benyBot *Bot) forgetPollMessage(pollID uuid.UUID) {
	benyBot.pollMutex.Lock()
	benyBot.forgetPollMessageLocked(pollID)
	benyBot.pollMutex.Unlock()
}

func (benyBot *Bot) forgetPollMessageLocked(pollID uuid.UUID) {
	for messageID, message := range benyBot.pollMessages {
		if message.pollID == pollID {
			delete(benyBot.pollMessages, messageID)
		}
	}
}

// schedulePollPostRetry has the planner try a failed post again, each wait twice the last up to
// longestRetryDelay, and returns this wait.
func (benyBot *Bot) schedulePollPostRetry(campaignID uuid.UUID, pollID uuid.UUID) time.Duration {
	benyBot.pollMutex.Lock()
	retry, found := benyBot.pollPostRetries[pollID]
	if !found {
		retry.delay = firstRetryDelay
	}
	wait := retry.delay
	retry.campaignID = campaignID
	retry.at = benyBot.now().Add(wait)
	retry.delay = min(wait*2, longestRetryDelay)
	benyBot.pollPostRetries[pollID] = retry
	benyBot.pollMutex.Unlock()
	benyBot.signalPollsChanged()
	return wait
}

func (benyBot *Bot) clearPollPostRetry(pollID uuid.UUID) {
	benyBot.pollMutex.Lock()
	delete(benyBot.pollPostRetries, pollID)
	benyBot.pollMutex.Unlock()
}

// retryDuePollPosts tries again every failed poll post whose wait has passed. The next wait is set
// now, so a retry still running never fires twice; a post that fails again replaces it.
func (benyBot *Bot) retryDuePollPosts() {
	now := benyBot.now()
	due := map[uuid.UUID]uuid.UUID{}
	benyBot.pollMutex.Lock()
	for pollID, retry := range benyBot.pollPostRetries {
		if !now.Before(retry.at) {
			due[pollID] = retry.campaignID
			retry.at = now.Add(retry.delay)
			benyBot.pollPostRetries[pollID] = retry
		}
	}
	benyBot.pollMutex.Unlock()
	for pollID, campaignID := range due {
		benyBot.PollChanged(campaignID, pollID)
	}
}

func (benyBot *Bot) signalPollsChanged() {
	select {
	case benyBot.pollsChanged <- struct{}{}:
	default:
	}
}

// reloadPollDeadlines replaces the deadlines with the open polls of the campaigns the planner lists, so
// an unlinked or paused campaign's polls drop out. A campaign whose read fails keeps what it had. Every
// open poll then gets another try at posting, and after Ready its votes are read again, since Discord
// doesn't replay vote events missed while offline.
func (benyBot *Bot) reloadPollDeadlines(workContext context.Context, service colonyService, plans []core.CampaignSessionPlan) {
	benyBot.pollMutex.Lock()
	startSequence := benyBot.pollDeadlineSequence
	benyBot.pollMutex.Unlock()
	read := map[uuid.UUID][]core.PollDeadline{}
	failed := map[uuid.UUID]bool{}
	for _, plan := range plans {
		deadlines, err := service.PollDeadlines(workContext, plan.CampaignID)
		if err != nil {
			benyBot.options.Logger.Error("Beny Bot couldn't read a campaign's poll deadlines", slog.String("campaign_id", plan.CampaignID.String()), slog.Any("error", err))
			failed[plan.CampaignID] = true
			continue
		}
		read[plan.CampaignID] = deadlines
	}
	benyBot.pollMutex.Lock()
	reloaded := map[uuid.UUID]pollDeadline{}
	for pollID, deadline := range benyBot.pollDeadlines {
		// A poll change after the reads began is newer than what they saw.
		if failed[deadline.campaignID] || deadline.sequence > startSequence {
			reloaded[pollID] = deadline
		}
	}
	for campaignID, deadlines := range read {
		for _, deadline := range deadlines {
			if _, newer := reloaded[deadline.PollID]; newer {
				continue
			}
			reloaded[deadline.PollID] = pollDeadline{campaignID: campaignID, closesAt: deadline.ClosesAt, retryAt: benyBot.pollDeadlines[deadline.PollID].retryAt}
		}
	}
	benyBot.pollDeadlines = reloaded
	for messageID, message := range benyBot.pollMessages {
		if _, open := reloaded[message.pollID]; !open && !failed[message.campaignID] {
			delete(benyBot.pollMessages, messageID)
		}
	}
	for pollID, retry := range benyBot.pollPostRetries {
		if _, open := reloaded[pollID]; !open && !failed[retry.campaignID] {
			delete(benyBot.pollPostRetries, pollID)
		}
	}
	syncVotes := benyBot.pollVotesStale
	if len(failed) == 0 {
		benyBot.pollVotesStale = false
	}
	benyBot.pollMutex.Unlock()
	for campaignID, deadlines := range read {
		for _, deadline := range deadlines {
			benyBot.PollChanged(campaignID, deadline.PollID)
			if syncVotes {
				benyBot.requestPollVoteSync(campaignID, deadline.PollID)
			}
		}
	}
}

// closeDuePolls closes every poll whose deadline has passed, each in the background so the planner never
// waits on Discord's voter reads.
func (benyBot *Bot) closeDuePolls(lifetimeContext context.Context) {
	service := benyBot.currentService()
	if service == nil {
		return
	}
	now := benyBot.now()
	due := map[uuid.UUID]pollDeadline{}
	benyBot.pollMutex.Lock()
	for pollID, deadline := range benyBot.pollDeadlines {
		if !benyBot.pollsClosing[pollID] && !now.Before(deadline.closesAt) && !now.Before(deadline.retryAt) {
			due[pollID] = deadline
			benyBot.pollsClosing[pollID] = true
		}
	}
	benyBot.pollMutex.Unlock()
	for pollID, deadline := range due {
		benyBot.posts.Go(func() {
			benyBot.closeDuePoll(lifetimeContext, service, pollID, deadline)
			benyBot.pollMutex.Lock()
			delete(benyBot.pollsClosing, pollID)
			benyBot.pollMutex.Unlock()
			benyBot.signalPollsChanged()
		})
	}
}

// closeDuePoll closes a poll at its deadline, a posted DISCORD poll after its final voters are read. A
// close that fails waits a minute before it tries again.
func (benyBot *Bot) closeDuePoll(lifetimeContext context.Context, service colonyService, pollID uuid.UUID, deadline pollDeadline) {
	workContext, cancel := context.WithTimeout(lifetimeContext, plannerWorkTimeout)
	defer cancel()
	logger := benyBot.options.Logger.With(slog.String("campaign_id", deadline.campaignID.String()), slog.String("poll_id", pollID.String()))
	unlock := benyBot.lockPoll(pollID)
	defer unlock()
	syncContext, cancelSync := context.WithTimeout(workContext, pollVoteSyncTimeout)
	err := benyBot.syncPollVotes(syncContext, service, deadline.campaignID, pollID)
	cancelSync()
	if err != nil {
		// Past the grace core closes the poll on its own, so the last votes are lost either way.
		if benyBot.now().Before(deadline.closesAt.Add(game.DiscordPollCloseGrace)) {
			logger.Warn("Beny Bot couldn't read a poll's final votes; it tries again in a minute", slog.Any("error", err))
			benyBot.delayPollClose(pollID)
			return
		}
		logger.Warn("Beny Bot couldn't read a poll's final votes; it closes the poll with the votes it has", slog.Any("error", err))
	}
	result, err := service.ClosePollAtDeadline(workContext, deadline.campaignID, pollID)
	if err != nil {
		logger.Error("Beny Bot couldn't close a poll at its deadline; it tries again in a minute", slog.Any("error", err))
		benyBot.delayPollClose(pollID)
		return
	}
	if !result.Closed && result.ClosesAt.After(benyBot.now()) {
		// A recorded Discord expiry moved the close later, so the poll is still open.
		benyBot.setPollDeadline(pollID, deadline.campaignID, result.ClosesAt)
		return
	}
	benyBot.forgetPollDeadline(pollID)
}

// nextPollWake is the earliest instant a tracked poll is due to close or a failed post is due again,
// nil when there is none. A close still running is skipped; it wakes the planner when it ends.
func (benyBot *Bot) nextPollWake() *time.Time {
	benyBot.pollMutex.Lock()
	defer benyBot.pollMutex.Unlock()
	var wake *time.Time
	consider := func(due time.Time) {
		if wake == nil || due.Before(*wake) {
			wake = new(due)
		}
	}
	for pollID, deadline := range benyBot.pollDeadlines {
		if benyBot.pollsClosing[pollID] {
			continue
		}
		due := deadline.closesAt
		if deadline.retryAt.After(due) {
			due = deadline.retryAt
		}
		consider(due)
	}
	for _, retry := range benyBot.pollPostRetries {
		consider(retry.at)
	}
	return wake
}
