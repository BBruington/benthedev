package game

import (
	"strconv"
	"strings"
	"time"
)

// Poll limits and texts (spec §3.5): Discord's own poll limits, which every poll takes, so any poll
// could be posted there.
const (
	MaxPollQuestionLength = 300
	MaxPollOptionLength   = 55
	MinPollOptions        = 2
	MaxPollOptions        = 10
	MaxOpenPolls          = 10
	MinPollDurationHours  = 1
	MaxPollDurationHours  = 768
	// DiscordPollCloseGrace is how long past its deadline a posted Discord poll waits before a site
	// request closes it, so Beny Bot can record the final voters first.
	DiscordPollCloseGrace = 10 * time.Minute
)

// CheckPollOptions refuses too few or too many options and options that differ only in case or
// surrounding spaces. The labels arrive already trimmed and checked one by one.
func CheckPollOptions(labels []string) error {
	if len(labels) < MinPollOptions || len(labels) > MaxPollOptions {
		return invalid("options", "Give 2 to 10 options")
	}
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		folded := strings.ToLower(strings.TrimSpace(label))
		if _, duplicate := seen[folded]; duplicate {
			return invalid("options", "Each option must be different")
		}
		seen[folded] = struct{}{}
	}
	return nil
}

// CheckPollDuration refuses a duration outside Discord's 1 hour to 32 days.
func CheckPollDuration(hours int) error {
	if hours < MinPollDurationHours || hours > MaxPollDurationHours {
		return invalid("durationHours", "Pick a duration of 1 hour to 32 days")
	}
	return nil
}

// OpenPollLimitReached reports whether openPolls leaves no room for one more open poll.
func OpenPollLimitReached(openPolls int64) bool {
	return openPolls >= MaxOpenPolls
}

// PollClosed reports whether a poll is closed: its close is recorded or its deadline has passed.
func PollClosed(closedAt *time.Time, closesAt time.Time, now time.Time) bool {
	return closedAt != nil || !closesAt.After(now)
}

// PollOverdue reports whether a poll's deadline has passed but nothing has recorded its close yet.
func PollOverdue(closedAt *time.Time, closesAt time.Time, now time.Time) bool {
	return closedAt == nil && !closesAt.After(now)
}

// PollDueForLazyClose reports whether a site request should record an overdue poll's close. A poll
// posted to Discord waits DiscordPollCloseGrace past its deadline, since Beny Bot syncs its final votes.
func PollDueForLazyClose(closedAt *time.Time, closesAt time.Time, postedToDiscord bool, now time.Time) bool {
	if postedToDiscord {
		closesAt = closesAt.Add(DiscordPollCloseGrace)
	}
	return PollOverdue(closedAt, closesAt, now)
}

// CheckPollVote refuses more than one option on a single-answer poll. An empty vote withdraws.
func CheckPollVote(allowMultiple bool, optionCount int) error {
	if !allowMultiple && optionCount > 1 {
		return invalid("optionIds", "A single-answer poll takes one option")
	}
	return nil
}

// PollTally is one option's final count, in the poll's order.
type PollTally struct {
	Label string
	Count int64
}

// PollClosedTitle renders the POLL_CLOSED entry: the question, then every option's count in order,
// naming no winner ("Where do we build the mill? By the river 3, On the hill 2").
func PollClosedTitle(question string, tallies []PollTally) string {
	var builder strings.Builder
	builder.WriteString(question)
	for index, tally := range tallies {
		if index == 0 {
			builder.WriteString(" ")
		} else {
			builder.WriteString(", ")
		}
		builder.WriteString(tally.Label)
		builder.WriteString(" ")
		builder.WriteString(strconv.FormatInt(tally.Count, 10))
	}
	return builder.String()
}
