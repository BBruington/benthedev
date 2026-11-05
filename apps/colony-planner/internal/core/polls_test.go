package core_test

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
	"github.com/BBruington/colony-planner/internal/game"
)

const (
	pollGuildID          = "900000000000000071"
	pollAnnounceChannel  = "910000000000000071"
	pollDiscordMessageID = "920000000000000071"
)

func (test *colonyTest) poll(allowMultiple bool, durationHours int, options ...string) core.Poll {
	test.t.Helper()
	poll, err := test.service.CreatePoll(test.context, test.dm, core.CreatePollInput{
		Question: "Where do we build the mill?", Options: options, AllowMultiple: allowMultiple, DurationHours: durationHours,
	})
	if err != nil {
		test.t.Fatalf("create a poll: %v", err)
	}
	return poll
}

// linkPollServer links the campaign to a Discord server with an announcements channel, so new polls
// are DISCORD polls.
func (test *colonyTest) linkPollServer() {
	test.t.Helper()
	test.directory.channels[pollGuildID] = []core.DiscordChannel{{ID: pollAnnounceChannel, Name: "announcements"}}
	code, err := test.service.CreateLinkCode(test.context, test.dm)
	if err != nil {
		test.t.Fatalf("link code: %v", err)
	}
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: code.Code, GuildID: pollGuildID, GuildName: "Test"}); err != nil {
		test.t.Fatalf("link: %v", err)
	}
	announce := pollAnnounceChannel
	if _, err := test.service.UpdateSettings(test.context, test.dm, core.SettingsInput{
		CampaignName: "Sample", ColonyName: "Sample", Timezone: "UTC", DiscordAnnounceChannelID: &announce,
	}); err != nil {
		test.t.Fatalf("settings: %v", err)
	}
}

// postToDiscord records the poll's Discord message as Beny Bot would, with answer IDs 1, 2, 3...
func (test *colonyTest) postToDiscord(poll core.Poll, expiry time.Time) {
	test.t.Helper()
	answerIDs := make([]int, 0, len(poll.Options))
	for index := range poll.Options {
		answerIDs = append(answerIDs, index+1)
	}
	if err := test.service.RecordPollMessage(test.context, test.campaignID, poll.ID, core.RecordPollMessageInput{
		GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: pollDiscordMessageID, Expiry: expiry, AnswerIDs: answerIDs,
	}); err != nil {
		test.t.Fatalf("record the Discord poll: %v", err)
	}
}

func (test *colonyTest) sync(pollID uuid.UUID, votes map[int][]string) core.Poll {
	test.t.Helper()
	if err := test.service.SyncDiscordPollVotes(test.context, test.campaignID, pollID, votes); err != nil {
		test.t.Fatalf("sync: %v", err)
	}
	discord, err := test.service.PollForDiscord(test.context, test.campaignID, pollID)
	if err != nil {
		test.t.Fatalf("poll for Discord: %v", err)
	}
	return discord.Poll
}

func (test *colonyTest) vote(viewer core.Viewer, pollID uuid.UUID, optionIDs ...uuid.UUID) core.Poll {
	test.t.Helper()
	poll, err := test.service.VotePoll(test.context, viewer, pollID, optionIDs)
	if err != nil {
		test.t.Fatalf("vote: %v", err)
	}
	return poll
}

func (test *colonyTest) polls(viewer core.Viewer) core.PollList {
	test.t.Helper()
	list, err := test.service.ListPolls(test.context, viewer)
	if err != nil {
		test.t.Fatalf("list polls: %v", err)
	}
	return list
}

func (test *colonyTest) pollClosedEntries() []core.TimelineEntry {
	test.t.Helper()
	day := test.board().Day
	closed := []core.TimelineEntry{}
	for _, entry := range test.timeline(test.player, day, day) {
		if entry.Kind == core.TimelineKindPollClosed {
			closed = append(closed, entry)
		}
	}
	return closed
}

func optionCounts(poll core.Poll) []int64 {
	counts := make([]int64, 0, len(poll.Options))
	for _, option := range poll.Options {
		counts = append(counts, option.Count)
	}
	return counts
}

func sameIDs(got []uuid.UUID, want ...uuid.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func TestCreatingAPollChecksItsInputAndTheOpenLimit(t *testing.T) {
	test := newColonyTest(t)
	cases := []struct {
		name  string
		input core.CreatePollInput
		field string
	}{
		{"blank question", core.CreatePollInput{Question: "  ", Options: []string{"A", "B"}, DurationHours: 24}, "question"},
		{"long question", core.CreatePollInput{Question: strings.Repeat("q", 301), Options: []string{"A", "B"}, DurationHours: 24}, "question"},
		{"one option", core.CreatePollInput{Question: "Q", Options: []string{"A"}, DurationHours: 24}, "options"},
		{"eleven options", core.CreatePollInput{Question: "Q", Options: strings.Split("a,b,c,d,e,f,g,h,i,j,k", ","), DurationHours: 24}, "options"},
		{"blank option", core.CreatePollInput{Question: "Q", Options: []string{"A", " "}, DurationHours: 24}, "options"},
		{"long option", core.CreatePollInput{Question: "Q", Options: []string{"A", strings.Repeat("o", 56)}, DurationHours: 24}, "options"},
		{"duplicate options", core.CreatePollInput{Question: "Q", Options: []string{"Mill", " mill "}, DurationHours: 24}, "options"},
		{"no duration", core.CreatePollInput{Question: "Q", Options: []string{"A", "B"}}, "durationHours"},
		{"too long", core.CreatePollInput{Question: "Q", Options: []string{"A", "B"}, DurationHours: 769}, "durationHours"},
	}
	for _, testCase := range cases {
		_, err := test.service.CreatePoll(test.context, test.dm, testCase.input)
		requireKind(t, err, core.KindInvalid, testCase.field)
	}
	if _, err := test.service.CreatePoll(test.context, test.player, core.CreatePollInput{Question: "Q", Options: []string{"A", "B"}, DurationHours: 24}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player created a poll: %v", err)
	}

	first := test.poll(false, 48, " By the river ", "On the hill")
	if first.Question != "Where do we build the mill?" || len(first.Options) != 2 || first.Options[0].Label != "By the river" || first.Closed || first.ClosedAt != nil ||
		first.MyOptionIDs == nil || first.Voting != core.PollVotingSite || first.DiscordURL != nil || !first.ClosesAt.Equal(test.clock.Now().Add(48*time.Hour)) {
		t.Fatalf("created poll = %+v", first)
	}
	for range 9 {
		test.poll(false, 24, "A", "B")
	}
	_, err := test.service.CreatePoll(test.context, test.dm, core.CreatePollInput{Question: "Q", Options: []string{"A", "B"}, DurationHours: 24})
	requireMessage(t, err, core.KindConflict, "There are already 10 open polls. Close one before starting another.")
	if _, err := test.service.ClosePoll(test.context, test.dm, first.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	test.poll(false, 24, "A", "B")
	if list := test.polls(test.player); len(list.Open) != 10 || len(list.Closed) != 1 || list.NewPollVoting != core.PollVotingSite {
		t.Fatalf("polls = %d open, %d closed; want 10 and 1", len(list.Open), len(list.Closed))
	}
}

func TestOnlyActivePlayersVote(t *testing.T) {
	test := newColonyTest(t)
	poll := test.poll(false, 24, "A", "B")
	optionID := poll.Options[0].ID

	_, err := test.service.VotePoll(test.context, test.dm, poll.ID, []uuid.UUID{optionID})
	requireMessage(t, err, core.KindForbidden, "The DM runs polls and doesn't vote.")
	_, err = test.service.VotePoll(test.context, test.dm.AsPlayer(), poll.ID, []uuid.UUID{optionID})
	requireMessage(t, err, core.KindForbidden, "The DM runs polls and doesn't vote.")

	_, pending := test.fixture.player(test.dm, "seed:bram", "bram", false)
	if _, err := test.service.VotePoll(test.context, pending, poll.ID, []uuid.UUID{optionID}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a pending member voted: %v", err)
	}
	leaverCaller, leaver := test.fixture.player(test.dm, "seed:cora", "cora", true)
	if _, err := test.service.LeaveCampaign(test.context, leaver); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := test.service.VotePoll(test.context, test.enter(leaverCaller, test.campaignID), poll.ID, []uuid.UUID{optionID}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a removed member voted: %v", err)
	}
}

func TestAVoteReplacesTheLastAndAnEmptyVoteWithdraws(t *testing.T) {
	test := newColonyTest(t)
	single := test.poll(false, 24, "A", "B", "C")
	first, second := single.Options[0].ID, single.Options[1].ID

	voted := test.vote(test.player, single.ID, first)
	if !sameIDs(voted.MyOptionIDs, first) || voted.VoterCount != 1 || voted.Options[0].Count != 1 || len(voted.Options[0].Voters) != 1 {
		t.Fatalf("after a vote = %+v", voted)
	}
	voted = test.vote(test.player, single.ID, second)
	if !sameIDs(voted.MyOptionIDs, second) || voted.VoterCount != 1 || voted.Options[0].Count != 0 || voted.Options[1].Count != 1 {
		t.Fatalf("after changing the vote = %+v", voted)
	}
	if dmView := test.polls(test.dm).Open[0]; len(dmView.MyOptionIDs) != 0 || dmView.Options[1].Count != 1 {
		t.Fatalf("the DM's view = %+v", dmView)
	}
	withdrawn := test.vote(test.player, single.ID)
	if len(withdrawn.MyOptionIDs) != 0 || withdrawn.VoterCount != 0 || withdrawn.Options[1].Count != 0 {
		t.Fatalf("after withdrawing = %+v", withdrawn)
	}

	_, err := test.service.VotePoll(test.context, test.player, single.ID, []uuid.UUID{first, second})
	requireKind(t, err, core.KindInvalid, "optionIds")
	other := test.poll(false, 24, "X", "Y")
	_, err = test.service.VotePoll(test.context, test.player, single.ID, []uuid.UUID{other.Options[0].ID})
	requireFieldMessage(t, err, "optionIds", "That option isn't on this poll.")
	_, err = test.service.VotePoll(test.context, test.player, single.ID, []uuid.UUID{uuid.New()})
	requireFieldMessage(t, err, "optionIds", "That option isn't on this poll.")

	multiple := test.poll(true, 24, "A", "B", "C")
	both := test.vote(test.player, multiple.ID, multiple.Options[2].ID, multiple.Options[0].ID)
	if len(both.MyOptionIDs) != 2 || both.VoterCount != 1 || optionCounts(both)[0] != 1 || optionCounts(both)[2] != 1 {
		t.Fatalf("a several-answer vote = %+v", both)
	}
}

func TestClosingAPollWritesOnePublicEntryAndLeavesTheVersion(t *testing.T) {
	test := newColonyTest(t)
	poll := test.poll(false, 24, "By the river", "On the hill")
	test.vote(test.player, poll.ID, poll.Options[0].ID)
	version := test.board().Version

	_, err := test.service.ClosePoll(test.context, test.player, poll.ID)
	requireKind(t, err, core.KindForbidden, "")
	closed, err := test.service.ClosePoll(test.context, test.dm, poll.ID)
	if err != nil || !closed.Closed || closed.ClosedAt == nil || !closed.ClosedAt.Equal(test.clock.Now()) {
		t.Fatalf("close = %+v, %v", closed, err)
	}
	again, err := test.service.ClosePoll(test.context, test.dm, poll.ID)
	if err != nil || !again.Closed || !again.ClosedAt.Equal(*closed.ClosedAt) {
		t.Fatalf("second close = %+v, %v", again, err)
	}
	entries := test.pollClosedEntries()
	if len(entries) != 1 || entries[0].Title != "Where do we build the mill? By the river 1, On the hill 0" {
		t.Fatalf("POLL_CLOSED entries = %+v", entries)
	}
	if after := test.board().Version; after != version {
		t.Fatalf("version %d after closing; want %d", after, version)
	}
	_, err = test.service.VotePoll(test.context, test.player, poll.ID, []uuid.UUID{poll.Options[1].ID})
	requireMessage(t, err, core.KindConflict, "This poll is closed.")

	if err := test.service.DeletePoll(test.context, test.dm, poll.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if list := test.polls(test.dm); len(list.Open)+len(list.Closed) != 0 {
		t.Fatalf("polls after delete = %+v", list)
	}
	if entries := test.pollClosedEntries(); len(entries) != 1 {
		t.Fatalf("POLL_CLOSED entries after delete = %d; want 1", len(entries))
	}
	_, err = test.service.ClosePoll(test.context, test.dm, poll.ID)
	requireMessage(t, err, core.KindNotFound, "That poll doesn't exist.")
}

func TestAPollPastItsDeadlineIsClosed(t *testing.T) {
	test := newColonyTest(t)
	closesAt := test.clock.Now().Add(time.Hour)
	poll := test.poll(false, 1, "A", "B")
	test.vote(test.player, poll.ID, poll.Options[1].ID)

	if result, err := test.service.ClosePollAtDeadline(test.context, test.campaignID, poll.ID); err != nil || result.Closed || !result.ClosesAt.Equal(closesAt) {
		t.Fatalf("early deadline close = %+v, %v; want nothing closed and the deadline", result, err)
	}
	if list := test.polls(test.player); len(list.Open) != 1 || len(test.pollClosedEntries()) != 0 {
		t.Fatalf("a deadline close before the deadline closed the poll: %+v", list)
	}
	deadlines, err := test.service.PollDeadlines(test.context, test.campaignID)
	if err != nil || len(deadlines) != 1 || deadlines[0].PollID != poll.ID || !deadlines[0].ClosesAt.Equal(closesAt) {
		t.Fatalf("deadlines = %+v, %v", deadlines, err)
	}

	test.clock.advance(2 * time.Hour)
	if list := test.polls(test.player); len(list.Open) != 0 || len(list.Closed) != 1 || !list.Closed[0].Closed || list.Closed[0].CloseRecorded || len(test.pollClosedEntries()) != 0 {
		t.Fatalf("a read recorded the close: %+v", list)
	}
	_, err = test.service.VotePoll(test.context, test.player, poll.ID, []uuid.UUID{poll.Options[0].ID})
	requireMessage(t, err, core.KindConflict, "This poll is closed.")
	list := test.polls(test.player)
	if len(list.Open) != 0 || len(list.Closed) != 1 || !list.Closed[0].Closed || !list.Closed[0].ClosedAt.Equal(closesAt) || optionCounts(list.Closed[0])[1] != 1 {
		t.Fatalf("polls after the deadline = %+v", list)
	}
	if entries := test.pollClosedEntries(); len(entries) != 1 {
		t.Fatalf("the lazy close wrote %d POLL_CLOSED entries; want 1", len(entries))
	}
	for range 2 {
		if result, err := test.service.ClosePollAtDeadline(test.context, test.campaignID, poll.ID); err != nil || result.Closed {
			t.Fatalf("deadline close after the lazy one = %+v, %v; want nothing closed", result, err)
		}
	}
	if entries := test.pollClosedEntries(); len(entries) != 1 {
		t.Fatalf("the deadline close after the lazy one wrote %d entries; want 1", len(entries))
	}
}

func TestTheDeadlineTimerClosesAnOverduePoll(t *testing.T) {
	test := newColonyTest(t)
	poll := test.poll(false, 1, "A", "B")
	test.clock.advance(time.Hour)
	closedCount := 0
	for range 2 {
		result, err := test.service.ClosePollAtDeadline(test.context, test.campaignID, poll.ID)
		if err != nil {
			t.Fatalf("deadline close: %v", err)
		}
		if result.Closed {
			closedCount++
		}
	}
	if closedCount != 1 {
		t.Fatalf("deadline closes that closed the poll = %d; want 1", closedCount)
	}
	if result, err := test.service.ClosePollAtDeadline(test.context, test.campaignID, uuid.New()); err != nil || result.Closed {
		t.Fatalf("deadline close of a missing poll = %+v, %v", result, err)
	}
	if entries := test.pollClosedEntries(); len(entries) != 1 || entries[0].Title != "Where do we build the mill? A 0, B 0" {
		t.Fatalf("POLL_CLOSED entries = %+v", entries)
	}
	if deadlines, err := test.service.PollDeadlines(test.context, test.campaignID); err != nil || len(deadlines) != 0 {
		t.Fatalf("deadlines after the close = %+v, %v", deadlines, err)
	}
}

func TestAPollFromAnotherCampaignIsMissing(t *testing.T) {
	test := newColonyTest(t)
	_, otherDm := test.campaign(test.caller("600000000000000007", "other"), "Other", "other")
	foreign, err := test.service.CreatePoll(test.context, otherDm, core.CreatePollInput{Question: "Q", Options: []string{"A", "B"}, DurationHours: 24})
	if err != nil {
		t.Fatalf("create in the other campaign: %v", err)
	}
	_, err = test.service.VotePoll(test.context, test.player, foreign.ID, []uuid.UUID{foreign.Options[0].ID})
	requireMessage(t, err, core.KindNotFound, "That poll doesn't exist.")
	_, err = test.service.ClosePoll(test.context, test.dm, foreign.ID)
	requireMessage(t, err, core.KindNotFound, "That poll doesn't exist.")
	err = test.service.DeletePoll(test.context, test.dm, foreign.ID)
	requireMessage(t, err, core.KindNotFound, "That poll doesn't exist.")
	if _, err := test.service.PollForDiscord(test.context, test.campaignID, foreign.ID); !isKind(err, core.KindNotFound) {
		t.Fatalf("the other campaign's poll for Discord: %v", err)
	}
	if list := test.polls(test.player); len(list.Open) != 0 || list.Open == nil || list.Closed == nil {
		t.Fatalf("polls = %+v; want two empty lists", list)
	}
}

func TestConcurrentClosesWriteOneEntry(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	poll := test.poll(false, 1, "A", "B")
	test.clock.advance(2 * time.Hour)
	overdue, err := test.service.PollForDiscord(test.context, test.campaignID, poll.ID)
	if err != nil || !overdue.Poll.Closed || overdue.Poll.CloseRecorded {
		t.Fatalf("overdue poll = %+v, %v; want closed but not recorded", overdue.Poll, err)
	}
	var group sync.WaitGroup
	results := make([]error, 8)
	for index := range results {
		group.Go(func() {
			if index%2 == 0 {
				_, results[index] = test.service.ClosePoll(test.context, test.dm, poll.ID)
			} else {
				_, results[index] = test.service.ClosePollAtDeadline(test.context, test.campaignID, poll.ID)
			}
		})
	}
	group.Wait()
	for _, err := range results {
		if err != nil {
			t.Fatalf("a concurrent close failed with %v", err)
		}
	}
	if entries := test.pollClosedEntries(); len(entries) != 1 {
		t.Fatalf("POLL_CLOSED entries = %d; want 1", len(entries))
	}
	recorded, err := test.service.PollForDiscord(test.context, test.campaignID, poll.ID)
	if err != nil || !recorded.Poll.CloseRecorded {
		t.Fatalf("closed poll = %+v, %v; want its close recorded", recorded.Poll, err)
	}
}

func TestConcurrentCreatesKeepTheOpenLimit(t *testing.T) {
	test := newColonyTest(t)
	for range 9 {
		test.poll(false, 24, "A", "B")
	}
	var group sync.WaitGroup
	results := make([]error, 4)
	for index := range results {
		group.Go(func() {
			_, results[index] = test.service.CreatePoll(test.context, test.dm, core.CreatePollInput{Question: "Q", Options: []string{"A", "B"}, DurationHours: 24})
		})
	}
	group.Wait()
	for _, err := range results {
		if err != nil && !isKind(err, core.KindConflict) {
			t.Fatalf("a concurrent create failed with %v", err)
		}
	}
	if open := len(test.polls(test.player).Open); open != 10 {
		t.Fatalf("open polls = %d; want 10", open)
	}
}

func TestASecondDiscordMessageIsRefused(t *testing.T) {
	test := newColonyTest(t)
	site := test.poll(false, 24, "A", "B")
	test.linkPollServer()
	poll := test.poll(false, 24, "A", "B")
	expiry := test.clock.Now().Add(24*time.Hour + time.Second)
	test.postToDiscord(poll, expiry)
	err := test.service.RecordPollMessage(test.context, test.campaignID, poll.ID, core.RecordPollMessageInput{
		GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: "920000000000000072", Expiry: expiry, AnswerIDs: []int{7, 8},
	})
	if !errors.Is(err, core.ErrPollAlreadyPosted) {
		t.Fatalf("second record = %v; want ErrPollAlreadyPosted", err)
	}
	discord, err := test.service.PollForDiscord(test.context, test.campaignID, poll.ID)
	if err != nil || discord.GuildID != pollGuildID || discord.ChannelID != pollAnnounceChannel || discord.MessageID != pollDiscordMessageID ||
		!sameInts(discord.AnswerIDs, 1, 2) || !discord.Poll.ClosesAt.Equal(expiry) {
		t.Fatalf("recorded message = %+v, %v; want the first", discord, err)
	}
	wantURL := "https://discord.com/channels/" + pollGuildID + "/" + pollAnnounceChannel + "/" + pollDiscordMessageID
	if listed := test.polls(test.player).Open; len(listed) != 2 || listed[0].ID != poll.ID || listed[0].DiscordURL == nil || *listed[0].DiscordURL != wantURL || listed[1].DiscordURL != nil {
		t.Fatalf("player's polls = %+v; want the Discord link", listed)
	}
	same := core.RecordPollMessageInput{GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: pollDiscordMessageID, Expiry: expiry.Add(time.Hour), AnswerIDs: []int{7, 8}}
	if err := test.service.RecordPollMessage(test.context, test.campaignID, poll.ID, same); err != nil {
		t.Fatalf("recording the held message again = %v; want nil", err)
	}
	if again, err := test.service.PollForDiscord(test.context, test.campaignID, poll.ID); err != nil || !sameInts(again.AnswerIDs, 1, 2) || !again.Poll.ClosesAt.Equal(expiry) {
		t.Fatalf("after recording the held message again = %+v, %v; want it unchanged", again, err)
	}
	other := core.RecordPollMessageInput{GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: "920000000000000073", Expiry: expiry}
	if err := test.service.RecordPollMessage(test.context, test.campaignID, uuid.New(), other); !errors.Is(err, core.ErrNoSuchPoll) {
		t.Fatalf("record on a missing poll = %v; want ErrNoSuchPoll", err)
	}
	if err := test.service.RecordPollMessage(test.context, test.campaignID, site.ID, other); !errors.Is(err, core.ErrNoSuchPoll) {
		t.Fatalf("record on a site poll = %v; want ErrNoSuchPoll", err)
	}
	if _, err := test.service.PollForDiscord(test.context, test.campaignID, site.ID); !errors.Is(err, core.ErrNoSuchPoll) {
		t.Fatalf("a site poll for Discord = %v; want ErrNoSuchPoll", err)
	}
}

func TestARecordAfterACloseOrWithoutSnowflakesIsRefused(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	poll := test.poll(false, 24, "A", "B")
	expiry := test.clock.Now().Add(24 * time.Hour)
	for _, input := range []core.RecordPollMessageInput{
		{GuildID: "guild", ChannelID: pollAnnounceChannel, MessageID: pollDiscordMessageID, Expiry: expiry, AnswerIDs: []int{1, 2}},
		{GuildID: pollGuildID, ChannelID: "123", MessageID: pollDiscordMessageID, Expiry: expiry, AnswerIDs: []int{1, 2}},
		{GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: "", Expiry: expiry, AnswerIDs: []int{1, 2}},
	} {
		if err := test.service.RecordPollMessage(test.context, test.campaignID, poll.ID, input); !isKind(err, core.KindInvalid) {
			t.Fatalf("record of %+v = %v; want Invalid", input, err)
		}
	}
	before, err := test.service.ClosePoll(test.context, test.dm, poll.ID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	err = test.service.RecordPollMessage(test.context, test.campaignID, poll.ID, core.RecordPollMessageInput{
		GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: pollDiscordMessageID, Expiry: expiry, AnswerIDs: []int{1, 2},
	})
	if !errors.Is(err, core.ErrPollClosed) {
		t.Fatalf("record after a close = %v; want ErrPollClosed", err)
	}
	discord, err := test.service.PollForDiscord(test.context, test.campaignID, poll.ID)
	if err != nil || discord.MessageID != "" || discord.GuildID != "" || !sameInts(discord.AnswerIDs, 0, 0) || !discord.Poll.ClosesAt.Equal(before.ClosesAt) {
		t.Fatalf("closed poll after a refused record = %+v, %v; want it untouched", discord, err)
	}
}

func TestDiscordSyncsKeepTheVoterOrderAndPublishOnlyChanges(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	// aaron sorts before ana by name, so only a kept created_at puts ana first.
	test.fixture.player(test.dm, "seed:aaron", "aaron", true)
	poll := test.poll(false, 24, "A", "B")
	test.postToDiscord(poll, test.clock.Now().Add(24*time.Hour))
	stream, _, err := test.service.Subscribe(test.player, "")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer stream.Close()
	published := func() int {
		count := 0
		for {
			select {
			case <-stream.Messages():
				count++
			default:
				return count
			}
		}
	}
	published()

	if voters := test.sync(poll.ID, map[int][]string{1: {"seed:ana"}}).Options[0].Voters; len(voters) != 1 || voters[0].Name != "ana" {
		t.Fatalf("voters after the first sync = %+v", voters)
	}
	if count := published(); count != 1 {
		t.Fatalf("the first sync published %d messages; want 1", count)
	}
	for _, round := range []string{"second", "third"} {
		voters := test.sync(poll.ID, map[int][]string{1: {"seed:aaron", "seed:ana"}}).Options[0].Voters
		if len(voters) != 2 || voters[0].Name != "ana" || voters[1].Name != "aaron" {
			t.Fatalf("voters after the %s sync = %+v; want ana then aaron", round, voters)
		}
	}
	if count := published(); count != 1 {
		t.Fatalf("the second and third syncs published %d messages; want 1 (the third changed nothing)", count)
	}

	tied := test.poll(false, 24, "A", "B")
	tiedRecord := core.RecordPollMessageInput{GuildID: pollGuildID, ChannelID: pollAnnounceChannel, MessageID: "920000000000000074", Expiry: test.clock.Now().Add(24 * time.Hour), AnswerIDs: []int{1, 2}}
	if err := test.service.RecordPollMessage(test.context, test.campaignID, tied.ID, tiedRecord); err != nil {
		t.Fatalf("record the second Discord poll: %v", err)
	}
	voters := test.sync(tied.ID, map[int][]string{1: {"seed:ana", "seed:aaron"}}).Options[0].Voters
	if len(voters) != 2 || voters[0].Name != "aaron" || voters[1].Name != "ana" {
		t.Fatalf("voters added in one sync = %+v; want them by name", voters)
	}
}

func TestPollsVoteInDiscordOnceTheServerHasAnAnnouncementsChannel(t *testing.T) {
	test := newColonyTest(t)
	if voting := test.polls(test.dm).NewPollVoting; voting != core.PollVotingSite {
		t.Fatalf("new poll voting before linking = %s; want SITE", voting)
	}
	site := test.poll(false, 24, "A", "B")
	test.linkPollServer()
	if voting := test.polls(test.dm).NewPollVoting; voting != core.PollVotingDiscord {
		t.Fatalf("new poll voting after linking = %s; want DISCORD", voting)
	}
	poll := test.poll(false, 24, "A", "B")
	if poll.Voting != core.PollVotingDiscord || poll.DiscordURL != nil {
		t.Fatalf("created poll = %+v; want DISCORD without a link yet", poll)
	}
	_, err := test.service.VotePoll(test.context, test.player, poll.ID, []uuid.UUID{poll.Options[0].ID})
	requireMessage(t, err, core.KindConflict, "Vote on this poll in Discord.")
	if voted := test.vote(test.player, site.ID, site.Options[0].ID); voted.Voting != core.PollVotingSite || voted.VoterCount != 1 {
		t.Fatalf("a site poll made before linking = %+v; want it still voted on the site", voted)
	}
}

func TestDiscordVotesSyncNamedPlayersAndCountEveryoneElse(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	_, laterVoter := test.fixture.player(test.dm, "seed:dana", "dana", true)
	poll := test.poll(true, 24, "Mill", "Well", "Wall")
	test.postToDiscord(poll, test.clock.Now().Add(24*time.Hour))
	outsider := "930000000000000001"

	synced := test.sync(poll.ID, map[int][]string{
		1:  {"seed:ana", test.dmCaller.DiscordUserID, outsider},
		2:  {"seed:dana", "seed:ana"},
		99: {outsider},
	})
	if !slices.Equal(optionCounts(synced), []int64{3, 2, 0}) || synced.Options[0].OtherVotes != 2 || synced.Options[1].OtherVotes != 0 ||
		len(synced.Options[0].Voters) != 1 || len(synced.Options[1].Voters) != 2 || synced.VoterCount != 4 {
		t.Fatalf("after the first sync = %+v", synced)
	}
	if mine := test.polls(laterVoter).Open[0].MyOptionIDs; !sameIDs(mine, poll.Options[1].ID) {
		t.Fatalf("dana's vote = %v; want Well", mine)
	}

	synced = test.sync(poll.ID, map[int][]string{3: {"seed:ana"}})
	if !slices.Equal(optionCounts(synced), []int64{0, 0, 1}) || synced.VoterCount != 1 || synced.Options[0].OtherVotes != 0 {
		t.Fatalf("after a resync = %+v", synced)
	}

	if _, err := test.service.ClosePoll(test.context, test.dm, poll.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	if entries := test.pollClosedEntries(); len(entries) != 1 || entries[0].Title != "Where do we build the mill? Mill 0, Well 0, Wall 1" {
		t.Fatalf("POLL_CLOSED entries = %+v", entries)
	}
	after := test.sync(poll.ID, map[int][]string{1: {"seed:ana", outsider}})
	if !slices.Equal(optionCounts(after), []int64{0, 0, 1}) {
		t.Fatalf("a sync after the close changed the counts: %+v", after)
	}
}

func TestAPostedDiscordPollClosesLazilyOnlyAfterItsGrace(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	poll := test.poll(false, 1, "A", "B")
	expiry := test.clock.Now().Add(time.Hour)
	test.postToDiscord(poll, expiry)

	test.clock.advance(time.Hour + time.Minute)
	test.poll(false, 24, "C", "D")
	list := test.polls(test.dm)
	if len(list.Closed) != 1 || !list.Closed[0].Closed || list.Closed[0].CloseRecorded || len(test.pollClosedEntries()) != 0 {
		t.Fatalf("inside the grace = %+v; want closed but not recorded", list)
	}
	test.clock.advance(game.DiscordPollCloseGrace)
	test.poll(false, 24, "C", "D")
	list = test.polls(test.dm)
	if len(list.Closed) != 1 || !list.Closed[0].CloseRecorded || !list.Closed[0].ClosedAt.Equal(expiry) || len(test.pollClosedEntries()) != 1 {
		t.Fatalf("after the grace = %+v; want the close recorded at the expiry", list)
	}
}

func sameInts(got []int, want ...int) bool {
	return slices.Equal(got, want)
}

func TestChangingAVoteKeepsTheVoterOrder(t *testing.T) {
	test := newColonyTest(t)
	poll := test.poll(true, 24, "A", "B")
	first, second := poll.Options[0].ID, poll.Options[1].ID
	_, laterVoter := test.fixture.player(test.dm, "seed:dana", "dana", true)
	test.vote(test.player, poll.ID, first)
	before := test.vote(laterVoter, poll.ID, first).Options[0].Voters
	after := test.vote(test.player, poll.ID, first, second).Options[0].Voters
	if len(before) != 2 || len(after) != 2 || before[0] != after[0] || before[1] != after[1] {
		t.Fatalf("voters after a change = %+v; want %+v", after, before)
	}
}

func TestARecordInAServerNoLongerLinkedIsRefused(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	poll := test.poll(false, 24, "A", "B")
	err := test.service.RecordPollMessage(test.context, test.campaignID, poll.ID, core.RecordPollMessageInput{
		GuildID: "800000000000000099", ChannelID: pollAnnounceChannel, MessageID: pollDiscordMessageID, Expiry: test.clock.Now().Add(24 * time.Hour), AnswerIDs: []int{1, 2},
	})
	if !errors.Is(err, core.ErrPollServerChanged) {
		t.Fatalf("record in another server = %v; want ErrPollServerChanged", err)
	}
	discord, err := test.service.PollForDiscord(test.context, test.campaignID, poll.ID)
	if err != nil || discord.MessageID != "" || discord.GuildID != "" {
		t.Fatalf("poll after a refused record = %+v, %v; want it unposted", discord, err)
	}
}

func TestASingleAnswerSnapshotWithADoubleVoterWritesNothing(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	poll := test.poll(false, 24, "A", "B")
	test.postToDiscord(poll, test.clock.Now().Add(24*time.Hour))
	test.sync(poll.ID, map[int][]string{1: {"seed:ana"}})
	err := test.service.SyncDiscordPollVotes(test.context, test.campaignID, poll.ID, map[int][]string{1: {"seed:ana"}, 2: {"seed:ana"}})
	if !errors.Is(err, core.ErrPollVotesInconsistent) {
		t.Fatalf("sync with a double voter = %v; want ErrPollVotesInconsistent", err)
	}
	if counts := optionCounts(test.polls(test.dm).Open[0]); !slices.Equal(counts, []int64{1, 0}) {
		t.Fatalf("counts after a refused sync = %v; want the last sync's", counts)
	}
}

func TestAnEarlyClosedDiscordPollIsToEndUntilItsExpiry(t *testing.T) {
	test := newColonyTest(t)
	test.linkPollServer()
	poll := test.poll(false, 24, "A", "B")
	test.postToDiscord(poll, test.clock.Now().Add(24*time.Hour))
	toEnd := func() []core.PollDiscordMessage {
		t.Helper()
		messages, err := test.service.DiscordPollsToEnd(test.context, test.campaignID)
		if err != nil {
			t.Fatalf("polls to end: %v", err)
		}
		return messages
	}
	if messages := toEnd(); len(messages) != 0 {
		t.Fatalf("an open poll is to end: %+v", messages)
	}
	if _, err := test.service.ClosePoll(test.context, test.dm, poll.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	want := []core.PollDiscordMessage{{PollID: poll.ID, ChannelID: pollAnnounceChannel, MessageID: pollDiscordMessageID}}
	if messages := toEnd(); !slices.Equal(messages, want) {
		t.Fatalf("polls to end after an early close = %+v; want %+v", messages, want)
	}
	test.clock.advance(24 * time.Hour)
	if messages := toEnd(); messages == nil || len(messages) != 0 {
		t.Fatalf("polls to end after the expiry = %#v; want an empty list", messages)
	}
}
