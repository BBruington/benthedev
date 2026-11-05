package game

import (
	"testing"
	"time"
)

func TestCheckPollOptions(t *testing.T) {
	tests := []struct {
		name    string
		labels  []string
		message string
	}{
		{name: "two options", labels: []string{"Yes", "No"}},
		{name: "ten options", labels: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}},
		{name: "one option", labels: []string{"Yes"}, message: "Give 2 to 10 options"},
		{name: "no options", labels: nil, message: "Give 2 to 10 options"},
		{name: "eleven options", labels: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}, message: "Give 2 to 10 options"},
		{name: "same ignoring case", labels: []string{"By the river", "by THE river"}, message: "Each option must be different"},
		{name: "same ignoring spaces", labels: []string{"Mill", " Mill "}, message: "Each option must be different"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireInvalid(t, CheckPollOptions(test.labels), "options", test.message)
		})
	}
}

func TestCheckPollDuration(t *testing.T) {
	tests := []struct {
		name    string
		hours   int
		message string
	}{
		{name: "one hour", hours: 1},
		{name: "one day", hours: 24},
		{name: "32 days", hours: 768},
		{name: "zero", hours: 0, message: "Pick a duration of 1 hour to 32 days"},
		{name: "negative", hours: -1, message: "Pick a duration of 1 hour to 32 days"},
		{name: "past 32 days", hours: 769, message: "Pick a duration of 1 hour to 32 days"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireInvalid(t, CheckPollDuration(test.hours), "durationHours", test.message)
		})
	}
}

func TestOpenPollLimitReached(t *testing.T) {
	for openPolls, want := range map[int64]bool{0: false, 9: false, 10: true, 11: true} {
		if got := OpenPollLimitReached(openPolls); got != want {
			t.Errorf("OpenPollLimitReached(%d) = %v, want %v", openPolls, got, want)
		}
	}
}

func TestPollClosedAndOverdue(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	tests := []struct {
		name     string
		closedAt *time.Time
		closesAt time.Time
		closed   bool
		overdue  bool
	}{
		{name: "open before its deadline", closesAt: future},
		{name: "deadline passed, not recorded", closesAt: past, closed: true, overdue: true},
		{name: "deadline exactly now", closesAt: now, closed: true, overdue: true},
		{name: "closed by hand before its deadline", closedAt: &past, closesAt: future, closed: true},
		{name: "closed at its deadline", closedAt: &past, closesAt: past, closed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PollClosed(test.closedAt, test.closesAt, now); got != test.closed {
				t.Errorf("PollClosed = %v, want %v", got, test.closed)
			}
			if got := PollOverdue(test.closedAt, test.closesAt, now); got != test.overdue {
				t.Errorf("PollOverdue = %v, want %v", got, test.overdue)
			}
		})
	}
}

func TestPollDueForLazyClose(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	closed := now.Add(-time.Hour)
	tests := []struct {
		name     string
		closedAt *time.Time
		closesAt time.Time
		posted   bool
		due      bool
	}{
		{name: "site poll before its deadline", closesAt: now.Add(time.Minute)},
		{name: "site poll at its deadline", closesAt: now, due: true},
		{name: "unposted Discord poll past its deadline", closesAt: now.Add(-time.Minute), due: true},
		{name: "posted poll inside the grace", closesAt: now.Add(-DiscordPollCloseGrace + time.Second), posted: true},
		{name: "posted poll at the end of the grace", closesAt: now.Add(-DiscordPollCloseGrace), posted: true, due: true},
		{name: "posted poll long past", closesAt: now.Add(-time.Hour), posted: true, due: true},
		{name: "already closed", closedAt: &closed, closesAt: now.Add(-time.Hour), due: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := PollDueForLazyClose(test.closedAt, test.closesAt, test.posted, now); got != test.due {
				t.Errorf("PollDueForLazyClose = %v, want %v", got, test.due)
			}
		})
	}
}

func TestCheckPollVote(t *testing.T) {
	tests := []struct {
		name          string
		allowMultiple bool
		optionCount   int
		message       string
	}{
		{name: "single, withdraw", optionCount: 0},
		{name: "single, one", optionCount: 1},
		{name: "single, two", optionCount: 2, message: "A single-answer poll takes one option"},
		{name: "several, withdraw", allowMultiple: true, optionCount: 0},
		{name: "several, three", allowMultiple: true, optionCount: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requireInvalid(t, CheckPollVote(test.allowMultiple, test.optionCount), "optionIds", test.message)
		})
	}
}

func TestPollClosedTitle(t *testing.T) {
	got := PollClosedTitle("Where do we build the mill?", []PollTally{
		{Label: "By the river", Count: 3},
		{Label: "On the hill", Count: 2},
		{Label: "Near the docks", Count: 0},
	})
	want := "Where do we build the mill? By the river 3, On the hill 2, Near the docks 0"
	if got != want {
		t.Errorf("PollClosedTitle = %q, want %q", got, want)
	}
}

func requireInvalid(t *testing.T, err error, field string, message string) {
	t.Helper()
	if message == "" {
		if err != nil {
			t.Fatalf("got %v, want no error", err)
		}
		return
	}
	invalidError, ok := err.(*InvalidError)
	if !ok || invalidError.Field != field || invalidError.Message != message {
		t.Fatalf("got %#v, want %s: %q", err, field, message)
	}
}
