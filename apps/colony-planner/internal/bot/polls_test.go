package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/game"
	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
)

// newPollWorld links server X to campaign A with its announce channel, and server Y to campaign B.
func newPollWorld(t *testing.T) *world {
	t.Helper()
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.link(guildY, test.dmB)
	return test
}

// startPoll creates a poll in the DM's campaign and waits until Beny Bot has posted it and read its
// first votes; the voter read count then starts from zero.
func (test *world) startPoll(dm core.Viewer, allowMultiple bool, durationHours int) core.Poll {
	test.t.Helper()
	poll, err := test.service.CreatePoll(test.context, dm, core.CreatePollInput{
		Question:      "Where do we sail next?",
		Options:       []string{"North", "South", "West"},
		AllowMultiple: allowMultiple,
		DurationHours: durationHours,
	})
	if err != nil {
		test.t.Fatalf("create poll: %v", err)
	}
	test.bot.posts.Wait()
	test.client.mutex.Lock()
	test.client.voterReads = 0
	test.client.mutex.Unlock()
	return poll
}

// pollAsDm is the poll as the DM reads it on the site.
func (test *world) pollAsDm(dm core.Viewer, pollID uuid.UUID) core.Poll {
	test.t.Helper()
	list, err := test.service.ListPolls(test.context, dm)
	if err != nil {
		test.t.Fatalf("list polls: %v", err)
	}
	for _, poll := range slices.Concat(list.Open, list.Closed) {
		if poll.ID == pollID {
			return poll
		}
	}
	test.t.Fatalf("poll %s is missing", pollID)
	return core.Poll{}
}

// discordPoll is the poll as Beny Bot reads it.
func (test *world) discordPoll(dm core.Viewer, pollID uuid.UUID) core.DiscordPoll {
	test.t.Helper()
	discordPoll, err := test.service.PollForDiscord(test.context, dm.CampaignID(), pollID)
	if err != nil {
		test.t.Fatalf("poll for Discord: %v", err)
	}
	return discordPoll
}

// voteEvent hands Beny Bot one poll vote event and waits for the sync it starts.
func (test *world) voteEvent(guildID string, messageID string) {
	test.bot.handlePollVote(guildID, messageID)
	test.bot.posts.Wait()
}

func (test *world) voterReads() int {
	test.client.mutex.Lock()
	defer test.client.mutex.Unlock()
	return test.client.voterReads
}

func (test *world) pollExpires() []string {
	test.client.mutex.Lock()
	defer test.client.mutex.Unlock()
	return slices.Clone(test.client.pollExpires)
}

func optionCounts(poll core.Poll) []int64 {
	counts := make([]int64, 0, len(poll.Options))
	for _, option := range poll.Options {
		counts = append(counts, option.Count)
	}
	return counts
}

func TestCreatingAPollPostsANativeDiscordPoll(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, true, 48)

	messages := test.client.messagesTo(channelXGeneral)
	if len(messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(messages))
	}
	sent := messages[0].message
	if sent.Content != "New poll: vote below. Every poll and its results: "+testPublicURL+"/c/sample-colony/polls" {
		t.Errorf("content = %q", sent.Content)
	}
	if sent.AllowedMentions == nil || len(sent.AllowedMentions.Parse) != 0 || sent.Flags&discordgo.MessageFlagsSuppressEmbeds == 0 {
		t.Errorf("mentions %+v and flags %d, want no mentions and no embeds", sent.AllowedMentions, sent.Flags)
	}
	if len(sent.Components) != 0 {
		t.Errorf("components = %v, want none", sent.Components)
	}
	if sent.Poll == nil {
		t.Fatal("the message has no poll")
	}
	answers := []string{}
	for _, answer := range sent.Poll.Answers {
		answers = append(answers, answer.Media.Text)
	}
	if sent.Poll.Question.Text != "Where do we sail next?" || !slices.Equal(answers, []string{"North", "South", "West"}) || !sent.Poll.AllowMultiselect || sent.Poll.Duration != 48 {
		t.Errorf("poll = %q %v multiselect %v for %d hours", sent.Poll.Question.Text, answers, sent.Poll.AllowMultiselect, sent.Poll.Duration)
	}

	message := test.prompt(channelXGeneral)
	discordPoll := test.discordPoll(test.dmA, poll.ID)
	if discordPoll.GuildID != guildX || discordPoll.ChannelID != channelXGeneral || discordPoll.MessageID != message.ID {
		t.Errorf("recorded %s/%s/%s, want %s/%s/%s", discordPoll.GuildID, discordPoll.ChannelID, discordPoll.MessageID, guildX, channelXGeneral, message.ID)
	}
	if !slices.Equal(discordPoll.AnswerIDs, []int{1, 2, 3}) {
		t.Errorf("answer IDs = %v, want [1 2 3]", discordPoll.AnswerIDs)
	}
	if !discordPoll.Poll.ClosesAt.Equal(*message.Poll.Expiry) {
		t.Errorf("closes at %v, want Discord's expiry %v", discordPoll.Poll.ClosesAt, *message.Poll.Expiry)
	}
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(*message.Poll.Expiry) {
		t.Errorf("next poll wake = %v, want the expiry %v", wake, *message.Poll.Expiry)
	}
}

func TestPollDurationHoursFollowsDiscordsLimits(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name          string
		closesAt      time.Time
		expectedHours int
		expectedOpen  bool
	}{
		{"whole hours", now.Add(48 * time.Hour), 48, true},
		{"a part hour rounds up", now.Add(47*time.Hour + time.Minute), 48, true},
		{"under an hour is one hour", now.Add(time.Minute), 1, true},
		{"past Discord's longest", now.Add(1000 * time.Hour), 768, true},
		{"at the deadline", now, 0, false},
		{"past the deadline", now.Add(-time.Hour), 0, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			hours, open := pollDurationHours(testCase.closesAt, now)
			if hours != testCase.expectedHours || open != testCase.expectedOpen {
				t.Errorf("pollDurationHours = %d, %v, want %d, %v", hours, open, testCase.expectedHours, testCase.expectedOpen)
			}
		})
	}
}

func TestAPollWithoutAnAnnounceChannelStaysOnTheSite(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	test.client.mutex.Lock()
	sentBefore := len(test.client.messages)
	test.client.mutex.Unlock()
	poll := test.startPoll(test.dmA, false, 24)
	if _, err := test.service.ClosePoll(test.context, test.dmA, poll.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	test.bot.posts.Wait()
	test.client.mutex.Lock()
	defer test.client.mutex.Unlock()
	if sent := len(test.client.messages) - sentBefore; sent != 0 || len(test.client.pollExpires) != 0 {
		t.Errorf("sent %d messages and ended %d polls, want none", sent, len(test.client.pollExpires))
	}
}

func TestAVoteEventSyncsEveryVoterFromDiscord(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, true, 24)
	message := test.prompt(channelXGeneral)
	test.client.setVoters(message.ID, 1, activeUserID, dmUserID, strangerUserID)
	test.client.setVoters(message.ID, 2, activeUserID)

	test.voteEvent(guildX, message.ID)
	synced := test.pollAsDm(test.dmA, poll.ID)
	if counts := optionCounts(synced); !slices.Equal(counts, []int64{3, 1, 0}) {
		t.Errorf("counts = %v, want [3 1 0]", counts)
	}
	if voters := synced.Options[0].Voters; len(voters) != 1 || voters[0].Name != "ana" || synced.Options[0].OtherVotes != 2 {
		t.Errorf("first option's voters = %v with %d others, want ana and 2 others", voters, synced.Options[0].OtherVotes)
	}

	// A withdrawn vote leaves Discord's list, and the next sync drops it.
	test.client.setVoters(message.ID, 1)
	test.voteEvent(guildX, message.ID)
	if counts := optionCounts(test.pollAsDm(test.dmA, poll.ID)); !slices.Equal(counts, []int64{0, 1, 0}) {
		t.Errorf("counts after withdrawing = %v, want [0 1 0]", counts)
	}
}

// pollReadCountingService counts Beny Bot's reads of a poll.
type pollReadCountingService struct {
	colonyService
	reads *atomic.Int64
}

func (service pollReadCountingService) PollForDiscord(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID) (core.DiscordPoll, error) {
	service.reads.Add(1)
	return service.colonyService.PollForDiscord(requestContext, campaignID, pollID)
}

func TestVoteEventsForAnotherServerOrMessageReadNothing(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	test.client.setVoters(message.ID, 1, activeUserID)
	pollReads := &atomic.Int64{}
	test.bot.mutex.Lock()
	test.bot.service = pollReadCountingService{colonyService: test.bot.service, reads: pollReads}
	test.bot.mutex.Unlock()
	for _, testCase := range []struct {
		name      string
		guildID   string
		messageID string
	}{
		{"another campaign's server", guildY, message.ID},
		{"an unlinked server", guildZ, message.ID},
		{"a message that isn't a poll", guildX, "499999999999999999"},
		{"no server", "", message.ID},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			test.voteEvent(testCase.guildID, testCase.messageID)
			if reads := test.voterReads(); reads != 0 {
				t.Errorf("read voters %d times, want none", reads)
			}
			if reads := pollReads.Load(); reads != 0 {
				t.Errorf("read the poll %d times, want none", reads)
			}
		})
	}
	if voters := test.pollAsDm(test.dmA, poll.ID).VoterCount; voters != 0 {
		t.Errorf("voters = %d, want 0", voters)
	}
}

func TestAVoteSyncAfterAnUnlinkReadsNoVoters(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	test.client.setVoters(message.ID, 1, activeUserID)
	if reply := test.run(serverCommand(guildX, dmUserID, "unlink", manageServer)); reply == "" {
		t.Fatal("/unlink gave no answer")
	}
	test.bot.posts.Wait()

	test.bot.requestPollVoteSync(test.dmA.CampaignID(), poll.ID)
	test.bot.posts.Wait()
	if reads := test.voterReads(); reads != 0 {
		t.Errorf("read voters %d times after the unlink, want none", reads)
	}
	test.voteEvent(guildX, message.ID)
	if reads := test.voterReads(); reads != 0 {
		t.Errorf("a vote event after the unlink read voters %d times, want none", reads)
	}
}

func TestAVoterPageThatDoesNotAdvanceFailsTheSync(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	sameVoters := make([]*discordgo.User, pollVotersPage)
	for index := range sameVoters {
		sameVoters[index] = &discordgo.User{ID: "900000000000000001"}
	}
	stuckPage, err := json.Marshal(map[string][]*discordgo.User{"users": sameVoters})
	if err != nil {
		t.Fatal(err)
	}
	test.client.mutex.Lock()
	test.client.voterRead = func(string, int, string, []byte) ([]byte, error) { return stuckPage, nil }
	test.client.mutex.Unlock()

	test.voteEvent(guildX, message.ID)
	if reads := test.voterReads(); reads != 2 {
		t.Errorf("read voters %d times, want 2: the page that repeats its last voter stops the read", reads)
	}
	if voters := test.pollAsDm(test.dmA, poll.ID).VoterCount; voters != 0 {
		t.Errorf("voters = %d, want 0: a failed read writes nothing", voters)
	}
}

func TestAnAnswerPastItsVoterPageCapWritesNothing(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	test.client.mutex.Lock()
	test.client.voterRead = func(_ string, answerID int, rawQuery string, page []byte) ([]byte, error) {
		if answerID != 1 {
			return page, nil
		}
		query, err := url.ParseQuery(rawQuery)
		if err != nil {
			return nil, err
		}
		first := 0
		if after := query.Get("after"); after != "" {
			last, err := strconv.Atoi(after[1:])
			if err != nil {
				return nil, err
			}
			first = last + 1
		}
		users := make([]*discordgo.User, pollVotersPage)
		for index := range users {
			users[index] = &discordgo.User{ID: fmt.Sprintf("9%017d", first+index)}
		}
		return json.Marshal(map[string][]*discordgo.User{"users": users})
	}
	test.client.mutex.Unlock()

	test.voteEvent(guildX, message.ID)
	if reads := test.voterReads(); reads != pollVoterPagesPerAnswer {
		t.Errorf("read voters %d times, want %d", reads, pollVoterPagesPerAnswer)
	}
	if voters := test.pollAsDm(test.dmA, poll.ID).VoterCount; voters != 0 {
		t.Errorf("voters = %d, want 0: a part of an answer's voters is never stored as the whole", voters)
	}
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(test.clock.now().Add(firstRetryDelay)) {
		t.Errorf("next poll wake = %v, want the vote read's retry", wake)
	}
}

func TestVoterReadsPageThroughEveryVoter(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	voterIDs := make([]string, 250)
	for index := range voterIDs {
		voterIDs[index] = fmt.Sprintf("9%017d", index)
	}
	test.client.setVoters(message.ID, 1, voterIDs...)

	test.voteEvent(guildX, message.ID)
	if counts := optionCounts(test.pollAsDm(test.dmA, poll.ID)); !slices.Equal(counts, []int64{250, 0, 0}) {
		t.Errorf("counts = %v, want [250 0 0]", counts)
	}
	// Three pages for the first answer, one each for the others.
	if reads := test.voterReads(); reads != 5 {
		t.Errorf("read voters %d times, want 5", reads)
	}
}

func TestReadyReadsEveryOpenPollsVotesAgain(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	// Votes cast while Beny Bot was offline send no event it hears later.
	test.client.setVoters(message.ID, 2, activeUserID)
	test.bot.pollMutex.Lock()
	test.bot.pollVotesStale = true
	test.bot.pollMutex.Unlock()

	test.plan()
	test.bot.posts.Wait()
	if counts := optionCounts(test.pollAsDm(test.dmA, poll.ID)); !slices.Equal(counts, []int64{0, 1, 0}) {
		t.Errorf("counts = %v, want [0 1 0]", counts)
	}
	readsAfterReady := test.voterReads()
	test.plan()
	test.bot.posts.Wait()
	if reads := test.voterReads(); reads != readsAfterReady {
		t.Errorf("a later plan read voters %d more times, want none", reads-readsAfterReady)
	}
}

func TestClosingAPollEarlyEndsItInDiscordOnce(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	if _, err := test.service.ClosePoll(test.context, test.dmA, poll.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	test.bot.posts.Wait()
	test.bot.PollChanged(test.dmA.CampaignID(), poll.ID)
	test.bot.posts.Wait()
	if expires := test.pollExpires(); !slices.Equal(expires, []string{message.ID}) {
		t.Errorf("ended polls = %v, want [%s] once", expires, message.ID)
	}
	if wake := test.bot.nextPollWake(); wake != nil {
		t.Errorf("next poll wake = %v, want none", wake)
	}
}

func TestDeletingAPollDeletesItsMessage(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	if err := test.service.DeletePoll(test.context, test.dmA, poll.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	test.bot.posts.Wait()
	test.client.mutex.Lock()
	defer test.client.mutex.Unlock()
	if !slices.Equal(test.client.deletes, []string{message.ID}) {
		t.Errorf("deletes = %v, want [%s]", test.client.deletes, message.ID)
	}
	if _, stillThere := test.client.posted[message.ID]; stillThere {
		t.Error("the poll's message is still in the channel")
	}
}

func TestThePlannerClosesAPollAtItsDeadlineWithItsFinalVotes(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 2)
	message := test.prompt(channelXGeneral)
	closesAt := *message.Poll.Expiry
	test.plan()
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(closesAt) {
		t.Fatalf("next poll wake = %v, want %v", wake, closesAt)
	}

	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	if test.pollAsDm(test.dmA, poll.ID).ClosedAt != nil {
		t.Fatal("the poll closed before its deadline")
	}

	// The last vote, cast just before Discord ended the poll, is in the final read.
	test.client.setVoters(message.ID, 3, activeUserID)
	test.advanceTo(closesAt)
	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	closed := test.pollAsDm(test.dmA, poll.ID)
	if closed.ClosedAt == nil || !closed.ClosedAt.Equal(closesAt) {
		t.Fatalf("closed at %v, want the deadline %v", closed.ClosedAt, closesAt)
	}
	if counts := optionCounts(closed); !slices.Equal(counts, []int64{0, 0, 1}) {
		t.Errorf("final counts = %v, want [0 0 1]", counts)
	}
	if expires := test.pollExpires(); len(expires) != 0 {
		t.Errorf("ended polls = %v, want none: Discord ended it itself", expires)
	}
	if wake := test.bot.nextPollWake(); wake != nil {
		t.Errorf("next poll wake = %v, want none", wake)
	}
}

func TestAFailedFinalVoteReadRetriesWithinTheGrace(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 2)
	closesAt := *test.prompt(channelXGeneral).Poll.Expiry
	test.client.mutex.Lock()
	test.client.voterError = errors.New("discord is down")
	test.client.mutex.Unlock()

	test.advanceTo(closesAt)
	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	if test.pollAsDm(test.dmA, poll.ID).CloseRecorded {
		t.Fatal("the poll closed without its final votes inside the grace")
	}
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(closesAt.Add(firstRetryDelay)) {
		t.Errorf("next poll wake = %v, want a retry at %v", wake, closesAt.Add(firstRetryDelay))
	}

	test.advanceTo(closesAt.Add(game.DiscordPollCloseGrace))
	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	if closed := test.pollAsDm(test.dmA, poll.ID); closed.ClosedAt == nil || !closed.ClosedAt.Equal(closesAt) {
		t.Errorf("closed at %v after the grace, want the deadline %v", closed.ClosedAt, closesAt)
	}
}

func TestThePlannerClosesASitePollAtItsDeadline(t *testing.T) {
	test := newWorld(t)
	test.link(guildX, test.dmA)
	poll := test.startPoll(test.dmA, false, 2)
	closesAt := poll.ClosesAt
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(closesAt) {
		t.Fatalf("next poll wake = %v, want %v", wake, closesAt)
	}
	test.advanceTo(closesAt)
	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	if test.pollAsDm(test.dmA, poll.ID).ClosedAt == nil {
		t.Error("the site poll is still open after its deadline")
	}
	if wake := test.bot.nextPollWake(); wake != nil {
		t.Errorf("next poll wake = %v, want none", wake)
	}
}

func TestAnOverduePollWhoseCloseIsUnstoredClosesAtOnce(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 2)
	closesAt := *test.prompt(channelXGeneral).Poll.Expiry
	test.advanceTo(closesAt.Add(time.Minute))
	// The poll reads as closed from its deadline on, but until the close is stored the timer must stay.
	test.bot.forgetPollDeadline(poll.ID)
	test.bot.PollChanged(test.dmA.CampaignID(), poll.ID)
	test.bot.posts.Wait()
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(closesAt) {
		t.Fatalf("next poll wake = %v, want the passed deadline %v", wake, closesAt)
	}
	if delay := test.bot.timerDelay(test.bot.nextPollWake()); delay != 0 {
		t.Errorf("timer delay = %v, want 0", delay)
	}

	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	if wake := test.bot.nextPollWake(); wake != nil {
		t.Errorf("next poll wake after the stored close = %v, want none", wake)
	}
	test.bot.PollChanged(test.dmA.CampaignID(), poll.ID)
	test.bot.posts.Wait()
	if wake := test.bot.nextPollWake(); wake != nil {
		t.Errorf("a stored close was tracked again: next poll wake = %v", wake)
	}
	if expires := test.pollExpires(); len(expires) != 0 {
		t.Errorf("ended polls = %v, want none for a close at the deadline", expires)
	}
}

func TestStoppingDoesNotWaitOutAQueuedVoteSync(t *testing.T) {
	test := newPollWorld(t)
	test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	test.bot.pollVoteDelay = time.Hour
	test.bot.handlePollVote(guildX, message.ID)

	stopped := make(chan struct{})
	go func() {
		test.bot.stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop waited out a vote sync an hour away")
	}
	if reads := test.voterReads(); reads != 0 {
		t.Errorf("read voters %d times after stopping, want none", reads)
	}
}

// raceLostService records another process's message for the poll just before Beny Bot records its own.
type raceLostService struct {
	colonyService
}

func (service raceLostService) RecordPollMessage(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID, input core.RecordPollMessageInput) error {
	first := input
	first.MessageID = "488888888888888888"
	if err := service.colonyService.RecordPollMessage(requestContext, campaignID, pollID, first); err != nil {
		return err
	}
	return service.colonyService.RecordPollMessage(requestContext, campaignID, pollID, input)
}

func TestAPollPostThatLostTheRaceIsDeletedAndTheWinnerSyncsVotes(t *testing.T) {
	test := newPollWorld(t)
	test.bot.mutex.Lock()
	test.bot.service = raceLostService{colonyService: test.bot.service}
	test.bot.mutex.Unlock()
	poll := test.startPoll(test.dmA, false, 24)
	if sent := len(test.client.messagesTo(channelXGeneral)); sent != 1 {
		t.Fatalf("sent %d poll messages, want 1", sent)
	}

	test.client.mutex.Lock()
	deletes := slices.Clone(test.client.deletes)
	remaining := 0
	for _, posted := range test.client.posted {
		if posted.ChannelID == channelXGeneral {
			remaining++
		}
	}
	test.client.mutex.Unlock()
	if len(deletes) != 1 || remaining != 0 {
		t.Errorf("deletes = %v with %d messages left, want the duplicate deleted", deletes, remaining)
	}
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID != "488888888888888888" {
		t.Errorf("recorded message = %q, want the first post's", messageID)
	}

	// Beny Bot caches the winning message, so its vote events count.
	test.client.mutex.Lock()
	test.client.posted["488888888888888888"] = &discordgo.Message{ID: "488888888888888888", ChannelID: channelXGeneral, Poll: &discordgo.Poll{
		Answers: []discordgo.PollAnswer{{AnswerID: 1}, {AnswerID: 2}, {AnswerID: 3}},
	}}
	test.client.mutex.Unlock()
	test.client.setVoters("488888888888888888", 1, activeUserID)
	test.voteEvent(guildX, "488888888888888888")
	if counts := optionCounts(test.pollAsDm(test.dmA, poll.ID)); !slices.Equal(counts, []int64{1, 0, 0}) {
		t.Errorf("counts after a vote on the winning message = %v, want [1 0 0]", counts)
	}
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(test.pollAsDm(test.dmA, poll.ID).ClosesAt) {
		t.Errorf("next poll wake = %v, want the recorded close", wake)
	}
}

func TestARefusedPollPostTellsTheDmOnce(t *testing.T) {
	test := newPollWorld(t)
	settings, err := test.service.GetSettings(test.context, test.dmA)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	announceChannelID, dmChannelID := channelXGeneral, channelXDesk
	if _, err := test.service.UpdateSettings(test.context, test.dmA, core.SettingsInput{
		CampaignName:             settings.CampaignName,
		ColonyName:               settings.ColonyName,
		Timezone:                 settings.Timezone,
		DiscordAnnounceChannelID: &announceChannelID,
		DiscordDmChannelID:       &dmChannelID,
		DiscordEventLocation:     settings.DiscordEventLocation,
	}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	test.client.mutex.Lock()
	test.client.pollSendError = &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeMissingPermissions}}
	test.client.mutex.Unlock()
	notices := func() int {
		test.client.mutex.Lock()
		defer test.client.mutex.Unlock()
		count := 0
		for _, sent := range test.client.messages {
			if sent.channelID == channelXDesk && strings.Contains(sent.message.Content, "missing a permission") {
				count++
			}
		}
		return count
	}

	poll := test.startPoll(test.dmA, false, 24)
	if count := notices(); count != 1 {
		t.Fatalf("%d notices in the DM channel after the refused post, want 1", count)
	}
	test.advanceTo(test.clock.now().Add(firstRetryDelay))
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
	if count := notices(); count != 1 {
		t.Errorf("%d notices after a second refusal, want still 1", count)
	}

	test.client.mutex.Lock()
	test.client.pollSendError = nil
	test.client.mutex.Unlock()
	test.advanceTo(test.clock.now().Add(2 * firstRetryDelay))
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID == "" {
		t.Fatal("the poll wasn't posted once the permission was fixed")
	}
}

func TestAFailedPollPostIsRetriedLater(t *testing.T) {
	test := newPollWorld(t)
	test.client.mutex.Lock()
	test.client.sendError = errors.New("HTTP 503 Service Unavailable")
	test.client.mutex.Unlock()
	poll := test.startPoll(test.dmA, false, 24)
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID != "" {
		t.Fatalf("recorded message %q after a failed post", messageID)
	}
	retryAt := test.clock.now().Add(firstRetryDelay)
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(retryAt) {
		t.Fatalf("next poll wake = %v, want the retry at %v", wake, retryAt)
	}

	test.client.mutex.Lock()
	test.client.sendError = nil
	test.client.mutex.Unlock()
	test.advanceTo(retryAt)
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID == "" {
		t.Fatal("the retry didn't post the poll")
	}
	test.bot.pollMutex.Lock()
	retries := len(test.bot.pollPostRetries)
	test.bot.pollMutex.Unlock()
	if retries != 0 {
		t.Errorf("%d post retries left after the post, want none", retries)
	}
}

// movedCloseService answers the deadline close as if a recorded Discord expiry had moved it to closesAt.
type movedCloseService struct {
	colonyService
	closesAt time.Time
}

func (service movedCloseService) ClosePollAtDeadline(context.Context, uuid.UUID, uuid.UUID) (core.PollDeadlineClose, error) {
	return core.PollDeadlineClose{Closed: false, ClosesAt: service.closesAt}, nil
}

func TestADeadlineCloseThatMovedKeepsTrackingThePoll(t *testing.T) {
	test := newPollWorld(t)
	test.startPoll(test.dmA, false, 2)
	closesAt := *test.prompt(channelXGeneral).Poll.Expiry
	movedTo := closesAt.Add(time.Hour)
	test.bot.mutex.Lock()
	test.bot.service = movedCloseService{colonyService: test.bot.service, closesAt: movedTo}
	test.bot.mutex.Unlock()

	test.advanceTo(closesAt)
	test.bot.closeDuePolls(test.context)
	test.bot.posts.Wait()
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(movedTo) {
		t.Errorf("next poll wake = %v, want the moved close %v", wake, movedTo)
	}
}

// closeRaceService holds the deadline close until a stale vote sync has written, so a close that doesn't
// wait for the running sync would keep its stale votes.
type closeRaceService struct {
	colonyService
	closing      chan struct{}
	closingOnce  *sync.Once
	staleWritten chan struct{}
	staleOnce    *sync.Once
}

func (service closeRaceService) SyncDiscordPollVotes(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID, votes map[int][]string) error {
	err := service.colonyService.SyncDiscordPollVotes(requestContext, campaignID, pollID, votes)
	if len(votes[1]) > 0 {
		service.staleOnce.Do(func() { close(service.staleWritten) })
	}
	return err
}

func (service closeRaceService) ClosePollAtDeadline(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID) (core.PollDeadlineClose, error) {
	service.closingOnce.Do(func() { close(service.closing) })
	select {
	case <-service.staleWritten:
	case <-time.After(2 * time.Second):
	}
	return service.colonyService.ClosePollAtDeadline(requestContext, campaignID, pollID)
}

func TestTheDeadlineCloseWaitsForARunningVoteSync(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 2)
	message := test.prompt(channelXGeneral)
	closesAt := *message.Poll.Expiry
	test.client.setVoters(message.ID, 1, activeUserID)
	race := closeRaceService{colonyService: test.bot.service, closing: make(chan struct{}), closingOnce: &sync.Once{}, staleWritten: make(chan struct{}), staleOnce: &sync.Once{}}
	test.bot.mutex.Lock()
	test.bot.service = race
	test.bot.mutex.Unlock()
	reading := make(chan struct{})
	release := make(chan struct{})
	var readsSeen atomic.Int64
	test.client.mutex.Lock()
	test.client.voterRead = func(_ string, _ int, _ string, page []byte) ([]byte, error) {
		if readsSeen.Add(1) == 1 {
			close(reading)
			<-release
		}
		return page, nil
	}
	test.client.mutex.Unlock()

	// The worker holds ana's vote on the first answer, read before she moved it to the second.
	test.bot.requestPollVoteSync(test.dmA.CampaignID(), poll.ID)
	<-reading
	test.client.setVoters(message.ID, 1)
	test.client.setVoters(message.ID, 2, activeUserID)
	test.advanceTo(closesAt)
	closed := make(chan struct{})
	go func() {
		test.bot.closeDuePolls(test.context)
		close(closed)
	}()
	select {
	case <-race.closing:
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-closed
	test.bot.posts.Wait()
	if counts := optionCounts(test.pollAsDm(test.dmA, poll.ID)); !slices.Equal(counts, []int64{0, 1, 0}) {
		t.Errorf("final counts = %v, want [0 1 0]: the close must read after the stale sync", counts)
	}
}

// hasPollPostRetry reports whether Beny Bot means to post the poll again.
func (test *world) hasPollPostRetry(pollID uuid.UUID) bool {
	test.bot.pollMutex.Lock()
	defer test.bot.pollMutex.Unlock()
	_, found := test.bot.pollPostRetries[pollID]
	return found
}

func TestARetriedPollPostReusesItsNonceSoDiscordPostsItOnce(t *testing.T) {
	test := newPollWorld(t)
	test.client.mutex.Lock()
	test.client.answerLost = errors.New("context deadline exceeded")
	test.client.mutex.Unlock()
	poll := test.startPoll(test.dmA, false, 24)
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID != "" {
		t.Fatalf("recorded message %q without Discord's answer", messageID)
	}

	test.client.mutex.Lock()
	test.client.answerLost = nil
	test.client.mutex.Unlock()
	test.advanceTo(test.clock.now().Add(firstRetryDelay))
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
	messages := test.client.messagesTo(channelXGeneral)
	if len(messages) != 1 {
		t.Fatalf("Discord posted %d poll messages, want 1", len(messages))
	}
	if nonce := messages[0].nonce; nonce != pollNonce(poll.ID, 0) || len(nonce) > 25 {
		t.Errorf("nonce = %q, want the poll's own of at most 25 characters", nonce)
	}
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID != test.prompt(channelXGeneral).ID {
		t.Errorf("recorded message = %q, want the one Discord posted", messageID)
	}
}

// recordFailingService refuses poll message records with err: the first failures of them, or every one
// while failures is nil.
type recordFailingService struct {
	colonyService
	err      error
	failures *atomic.Int32
}

func (service recordFailingService) RecordPollMessage(recordContext context.Context, campaignID uuid.UUID, pollID uuid.UUID, input core.RecordPollMessageInput) error {
	if service.failures == nil || service.failures.Add(-1) >= 0 {
		return service.err
	}
	return service.colonyService.RecordPollMessage(recordContext, campaignID, pollID, input)
}

// failFirstRecord has Beny Bot's first poll message record fail.
func (test *world) failFirstRecord() {
	failures := &atomic.Int32{}
	failures.Store(1)
	test.bot.mutex.Lock()
	test.bot.service = recordFailingService{colonyService: test.bot.service, err: errDatabaseDown, failures: failures}
	test.bot.mutex.Unlock()
}

func (test *world) setDeleteError(err error) {
	test.client.mutex.Lock()
	test.client.deleteError = err
	test.client.mutex.Unlock()
}

// retryPollWorkAfter advances the clock by wait and runs the poll work then due.
func (test *world) retryPollWorkAfter(wait time.Duration) {
	test.advanceTo(test.clock.now().Add(wait))
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
}

func TestAPollPostedInAServerNoLongerLinkedIsDeletedAndNotRetried(t *testing.T) {
	test := newPollWorld(t)
	test.bot.mutex.Lock()
	test.bot.service = recordFailingService{colonyService: test.bot.service, err: core.ErrPollServerChanged}
	test.bot.mutex.Unlock()
	poll := test.startPoll(test.dmA, false, 24)
	if standing := test.client.standingIn(channelXGeneral); len(standing) != 0 {
		t.Errorf("%d poll messages still stand, want the post deleted", len(standing))
	}
	if test.hasPollPostRetry(poll.ID) {
		t.Error("a post retry was scheduled; the relink re-plans instead")
	}
}

func TestARepostAfterADeletedPostUsesAFreshNonce(t *testing.T) {
	test := newPollWorld(t)
	test.failFirstRecord()
	poll := test.startPoll(test.dmA, false, 24)
	if !test.hasPollPostRetry(poll.ID) {
		t.Fatal("no post retry after the deleted post")
	}

	test.retryPollWorkAfter(firstRetryDelay)
	sent := test.client.messagesTo(channelXGeneral)
	if len(sent) != 2 || sent[0].nonce == sent[1].nonce {
		t.Fatalf("sent %d posts with nonces %v, want 2 with different nonces", len(sent), sent)
	}
	standing := test.client.standingIn(channelXGeneral)
	if len(standing) != 1 || test.discordPoll(test.dmA, poll.ID).MessageID != standing[0].ID {
		t.Errorf("recorded %q among standing messages %v, want the repost", test.discordPoll(test.dmA, poll.ID).MessageID, standing)
	}
}

func TestAPostThatCouldntBeDeletedGoesBeforeTheNextPost(t *testing.T) {
	test := newPollWorld(t)
	test.failFirstRecord()
	test.setDeleteError(&discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeMissingPermissions}})
	poll := test.startPoll(test.dmA, false, 24)
	first := test.prompt(channelXGeneral)

	test.retryPollWorkAfter(firstRetryDelay)
	if sent := test.client.messagesTo(channelXGeneral); len(sent) != 1 {
		t.Fatalf("sent %d poll messages while the first can't be deleted, want 1", len(sent))
	}
	if !test.hasPollPostRetry(poll.ID) {
		t.Fatal("no post retry while the first post stands")
	}

	test.setDeleteError(nil)
	test.retryPollWorkAfter(2 * firstRetryDelay)
	standing := test.client.standingIn(channelXGeneral)
	if len(standing) != 1 || standing[0].ID == first.ID {
		t.Fatalf("standing messages = %v, want only the repost", standing)
	}
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID != standing[0].ID {
		t.Errorf("recorded message = %q, want the repost %q", messageID, standing[0].ID)
	}
}

func TestTwoProcessesPostingAPollKeepItsOneMessage(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	// A deploy overlap: a second process on the same token read the poll before the first recorded it.
	second := newBot(Options{Token: "test-token", Logger: quietLogger()}, test.client, test.state, test.clock.now)
	if err := second.start(test.service); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.stop)
	stale := test.discordPoll(test.dmA, poll.ID)
	stale.GuildID, stale.ChannelID, stale.MessageID = "", "", ""
	target, _, err := test.service.DiscordTargetFor(test.context, test.dmA.CampaignID())
	if err != nil {
		t.Fatal(err)
	}

	second.postPoll(test.context, quietLogger(), test.service, test.dmA.CampaignID(), target, stale)
	second.posts.Wait()
	test.client.mutex.Lock()
	deletes := len(test.client.deletes)
	test.client.mutex.Unlock()
	if deletes != 0 {
		t.Errorf("tried %d deletes, want none", deletes)
	}
	if standing := test.client.standingIn(channelXGeneral); len(standing) != 1 || standing[0].ID != message.ID {
		t.Errorf("standing messages = %v, want only the first post", standing)
	}
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID != message.ID {
		t.Errorf("recorded message = %q, want %q", messageID, message.ID)
	}
}

// targetFailingService can't read any campaign's Discord server.
type targetFailingService struct {
	colonyService
}

func (targetFailingService) DiscordTargetFor(context.Context, uuid.UUID) (core.DiscordTarget, bool, error) {
	return core.DiscordTarget{}, false, errDatabaseDown
}

func TestAPollWhoseServerCantBeReadIsPostedLater(t *testing.T) {
	test := newPollWorld(t)
	original := test.bot.service
	test.bot.mutex.Lock()
	test.bot.service = targetFailingService{colonyService: original}
	test.bot.mutex.Unlock()
	poll := test.startPoll(test.dmA, false, 24)
	if !test.hasPollPostRetry(poll.ID) {
		t.Fatal("no post retry after the server read failed")
	}

	test.bot.mutex.Lock()
	test.bot.service = original
	test.bot.mutex.Unlock()
	test.advanceTo(test.clock.now().Add(firstRetryDelay))
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
	if messageID := test.discordPoll(test.dmA, poll.ID).MessageID; messageID == "" {
		t.Error("the retry didn't post the poll")
	}
}

func TestAFailedVoteReadIsRetried(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	test.client.setVoters(message.ID, 1, activeUserID)
	test.client.mutex.Lock()
	test.client.voterError = errors.New("HTTP 503 Service Unavailable")
	test.client.mutex.Unlock()
	test.voteEvent(guildX, message.ID)
	retryAt := test.clock.now().Add(firstRetryDelay)
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(retryAt) {
		t.Fatalf("next poll wake = %v, want the vote read's retry at %v", wake, retryAt)
	}

	test.client.mutex.Lock()
	test.client.voterError = nil
	test.client.mutex.Unlock()
	test.advanceTo(retryAt)
	test.bot.retryDuePollWork()
	test.bot.posts.Wait()
	if counts := optionCounts(test.pollAsDm(test.dmA, poll.ID)); !slices.Equal(counts, []int64{1, 0, 0}) {
		t.Errorf("counts after the retry = %v, want [1 0 0]", counts)
	}
	if wake := test.bot.nextPollWake(); wake == nil || !wake.Equal(test.pollAsDm(test.dmA, poll.ID).ClosesAt) {
		t.Errorf("next poll wake = %v, want the poll's close", wake)
	}
}

func TestAFailedPollEndIsRetriedUntilDiscordEndsIt(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		expireError error
		wantTries   int
	}{
		{name: "Discord fails once", expireError: errors.New("HTTP 503 Service Unavailable"), wantTries: 2},
		{name: "the poll had already ended", expireError: &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordPollExpiredCode}}, wantTries: 1},
		{name: "Beny Bot lost its permissions", expireError: &discordgo.RESTError{Message: &discordgo.APIErrorMessage{Code: discordgo.ErrCodeMissingPermissions}}, wantTries: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			test := newPollWorld(t)
			poll := test.startPoll(test.dmA, false, 24)
			test.client.mutex.Lock()
			test.client.expireError = testCase.expireError
			test.client.mutex.Unlock()
			if _, err := test.service.ClosePoll(test.context, test.dmA, poll.ID); err != nil {
				t.Fatalf("close: %v", err)
			}
			test.bot.posts.Wait()

			test.client.mutex.Lock()
			test.client.expireError = nil
			test.client.mutex.Unlock()
			test.advanceTo(test.clock.now().Add(firstRetryDelay))
			test.bot.retryDuePollWork()
			test.bot.posts.Wait()
			if tries := len(test.pollExpires()); tries != testCase.wantTries {
				t.Errorf("tried to end the poll %d times, want %d", tries, testCase.wantTries)
			}
			if wake := test.bot.nextPollWake(); wake != nil {
				t.Errorf("next poll wake = %v, want none once the poll ended", wake)
			}
		})
	}
}

func TestThePlannerEndsAPollClosedWhileBenyBotWasAway(t *testing.T) {
	test := newPollWorld(t)
	poll := test.startPoll(test.dmA, false, 24)
	message := test.prompt(channelXGeneral)
	test.client.mutex.Lock()
	test.client.expireError = errors.New("HTTP 503 Service Unavailable")
	test.client.mutex.Unlock()
	if _, err := test.service.ClosePoll(test.context, test.dmA, poll.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	test.bot.posts.Wait()
	// A restart forgets every poll it was ending.
	test.bot.pollMutex.Lock()
	clear(test.bot.pollEnds)
	test.bot.pollMutex.Unlock()
	test.client.mutex.Lock()
	test.client.expireError = nil
	test.client.mutex.Unlock()

	test.plan()
	test.bot.posts.Wait()
	if expires := test.pollExpires(); !slices.Equal(expires, []string{message.ID, message.ID}) {
		t.Errorf("ended polls = %v, want the failed try and the planner's", expires)
	}
}

// deadlineFailingService can't read any campaign's poll deadlines.
type deadlineFailingService struct {
	colonyService
}

func (deadlineFailingService) PollDeadlines(context.Context, uuid.UUID) ([]core.PollDeadline, error) {
	return nil, errDatabaseDown
}

func TestAFailedPollReloadIsRetriedWithBackOff(t *testing.T) {
	test := newPollWorld(t)
	test.bot.mutex.Lock()
	test.bot.service = deadlineFailingService{colonyService: test.bot.service}
	test.bot.mutex.Unlock()
	if wake := test.plan(); wake == nil || !wake.Equal(test.clock.now().Add(firstRetryDelay)) {
		t.Errorf("plan wake = %v, want the reload's retry in %v", wake, firstRetryDelay)
	}
	if wake := test.plan(); wake == nil || !wake.Equal(test.clock.now().Add(2*firstRetryDelay)) {
		t.Errorf("second plan wake = %v, want the reload's retry in %v", wake, 2*firstRetryDelay)
	}
}
