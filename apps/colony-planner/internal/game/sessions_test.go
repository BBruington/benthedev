package game_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/game"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func date(t *testing.T, text string) game.LocalDate {
	t.Helper()
	parsed, err := game.ParseLocalDate(text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func instant(t *testing.T, text string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestOccurrencesKeepLocalTimeAcrossDst(t *testing.T) {
	newYork := mustLocation(t, "America/New_York")
	berlin := mustLocation(t, "Europe/Berlin")
	lastDate := date(t, "2026-10-21")
	tests := []struct {
		name     string
		series   game.SessionSeries
		location *time.Location
		from     string
		to       string
		want     []string
		// gapDate is a local date whose start time doesn't exist there; it starts an hour later.
		gapDate string
	}{
		{
			name:     "Friday 8:00 PM New York across the November change",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Friday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-02"), StartHour: 20, DurationMinutes: 240},
			location: newYork,
			from:     "2026-10-22T00:00:00Z",
			to:       "2026-11-21T00:00:00Z",
			// EDT is UTC−4 until Sunday 1 November, then EST is UTC−5: the first Friday after is 01:00 UTC.
			want: []string{"2026-10-24T00:00:00Z", "2026-10-31T00:00:00Z", "2026-11-07T01:00:00Z", "2026-11-14T01:00:00Z"},
		},
		{
			name:     "Friday 8:00 PM New York across the March change",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Friday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-02"), StartHour: 20, DurationMinutes: 240},
			location: newYork,
			from:     "2027-03-05T00:00:00Z",
			to:       "2027-03-20T00:00:01Z",
			// EST until Sunday 14 March 2027, then EDT: 8:00 PM moves from 01:00 to 00:00 UTC.
			want: []string{"2027-03-06T01:00:00Z", "2027-03-13T01:00:00Z", "2027-03-20T00:00:00Z"},
		},
		{
			name:     "Saturday 7:00 PM Berlin across the October change",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Saturday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-03"), StartHour: 19, DurationMinutes: 180},
			location: berlin,
			from:     "2026-10-17T00:00:00Z",
			to:       "2026-11-01T00:00:00Z",
			want:     []string{"2026-10-17T17:00:00Z", "2026-10-24T17:00:00Z", "2026-10-31T18:00:00Z"},
		},
		{
			name:     "Saturday 7:00 PM Berlin across the March change",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Saturday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-03"), StartHour: 19, DurationMinutes: 180},
			location: berlin,
			from:     "2027-03-20T00:00:00Z",
			to:       "2027-04-04T00:00:00Z",
			want:     []string{"2027-03-20T18:00:00Z", "2027-03-27T18:00:00Z", "2027-04-03T17:00:00Z"},
		},
		{
			name: "every other week counts whole weeks from first_date",
			// First date Wednesday 7 October: 7–13 October is on (Friday the 9th, Monday the 12th), 14–20 is
			// off, 21–27 is on again.
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Monday, time.Friday}, EveryWeeks: 2, FirstDate: date(t, "2026-10-07"), StartHour: 18, StartMinute: 30, DurationMinutes: 120},
			location: time.UTC,
			from:     "2026-10-01T00:00:00Z",
			to:       "2026-11-03T00:00:00Z",
			want:     []string{"2026-10-09T18:30:00Z", "2026-10-12T18:30:00Z", "2026-10-23T18:30:00Z", "2026-10-26T18:30:00Z"},
		},
		{
			name:     "every other weekend keeps a Saturday and the Sunday after it together",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Saturday, time.Sunday}, EveryWeeks: 2, FirstDate: date(t, "2026-10-17"), StartHour: 18, DurationMinutes: 180},
			location: time.UTC,
			from:     "2026-10-01T00:00:00Z",
			to:       "2026-11-16T00:00:00Z",
			want: []string{
				"2026-10-17T18:00:00Z", "2026-10-18T18:00:00Z", "2026-10-31T18:00:00Z",
				"2026-11-01T18:00:00Z", "2026-11-14T18:00:00Z", "2026-11-15T18:00:00Z",
			},
		},
		{
			name:     "every other week keeps a Friday and the Sunday after it together",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Friday, time.Sunday}, EveryWeeks: 2, FirstDate: date(t, "2026-10-16"), StartHour: 18, DurationMinutes: 180},
			location: time.UTC,
			from:     "2026-10-01T00:00:00Z",
			to:       "2026-11-16T00:00:00Z",
			want: []string{
				"2026-10-16T18:00:00Z", "2026-10-18T18:00:00Z", "2026-10-30T18:00:00Z",
				"2026-11-01T18:00:00Z", "2026-11-13T18:00:00Z", "2026-11-15T18:00:00Z",
			},
		},
		{
			name:     "a start in the March gap begins after the skipped hour",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Sunday}, EveryWeeks: 1, FirstDate: date(t, "2027-03-07"), StartHour: 2, StartMinute: 30, DurationMinutes: 120},
			location: newYork,
			from:     "2027-03-07T00:00:00Z",
			to:       "2027-03-22T00:00:00Z",
			// 2:30 AM doesn't exist on 14 March 2027; it reads as 2:30 EST, which is 3:30 AM EDT.
			want:    []string{"2027-03-07T07:30:00Z", "2027-03-14T07:30:00Z", "2027-03-21T06:30:00Z"},
			gapDate: "2027-03-14",
		},
		{
			name:     "a start in the November overlap is the first 1:30 AM",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Sunday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-25"), StartHour: 1, StartMinute: 30, DurationMinutes: 120},
			location: newYork,
			from:     "2026-10-25T00:00:00Z",
			to:       "2026-11-09T00:00:00Z",
			want:     []string{"2026-10-25T05:30:00Z", "2026-11-01T05:30:00Z", "2026-11-08T06:30:00Z"},
		},
		{
			name:     "a start in London's October overlap is the first 1:30 AM, in summer time",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Sunday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-18"), StartHour: 1, StartMinute: 30, DurationMinutes: 120},
			location: mustLocation(t, "Europe/London"),
			from:     "2026-10-18T00:00:00Z",
			to:       "2026-11-02T00:00:00Z",
			// 1:30 AM BST is 00:30 UTC; the second 1:30 AM on 25 October is GMT, 01:30 UTC.
			want: []string{"2026-10-18T00:30:00Z", "2026-10-25T00:30:00Z", "2026-11-01T01:30:00Z"},
		},
		{
			name:     "a start in Berlin's October overlap is the first 2:30 AM",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Sunday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-25"), StartHour: 2, StartMinute: 30, DurationMinutes: 120},
			location: berlin,
			from:     "2026-10-25T00:00:00Z",
			to:       "2026-11-02T00:00:00Z",
			want:     []string{"2026-10-25T00:30:00Z", "2026-11-01T01:30:00Z"},
		},
		{
			name:     "several weekdays and an inclusive last date",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Tuesday, time.Wednesday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-13"), LastDate: &lastDate, StartHour: 9, DurationMinutes: 60},
			location: time.UTC,
			from:     "2026-10-01T00:00:00Z",
			to:       "2026-12-01T00:00:00Z",
			want:     []string{"2026-10-13T09:00:00Z", "2026-10-14T09:00:00Z", "2026-10-20T09:00:00Z", "2026-10-21T09:00:00Z"},
		},
		{
			name:     "a one-off on its date only",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Monday}, FirstDate: date(t, "2026-10-14"), StartHour: 20, DurationMinutes: 240},
			location: newYork,
			from:     "2026-10-01T00:00:00Z",
			to:       "2026-12-01T00:00:00Z",
			want:     []string{"2026-10-15T00:00:00Z"},
		},
		{
			name:     "a start at from is in, a start at to is out",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Friday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-02"), StartHour: 20, DurationMinutes: 240},
			location: newYork,
			from:     "2026-10-24T00:00:00Z",
			to:       "2026-10-31T00:00:00Z",
			want:     []string{"2026-10-24T00:00:00Z"},
		},
		{
			name:     "nothing before the first date",
			series:   game.SessionSeries{Weekdays: []time.Weekday{time.Friday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-02"), StartHour: 20, DurationMinutes: 240},
			location: newYork,
			from:     "2026-09-01T00:00:00Z",
			to:       "2026-10-02T00:00:00Z",
			want:     []string{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			occurrences := game.Occurrences(test.series, test.location, instant(t, test.from), instant(t, test.to))
			if len(occurrences) != len(test.want) {
				t.Fatalf("got %d occurrences %v, want %v", len(occurrences), occurrences, test.want)
			}
			for index, occurrence := range occurrences {
				if got := occurrence.StartsAt.Format(time.RFC3339); got != test.want[index] {
					t.Errorf("occurrence %d starts %s, want %s", index, got, test.want[index])
				}
				if occurrence.StartsAt.Location() != time.UTC {
					t.Errorf("occurrence %d is not UTC", index)
				}
				local := occurrence.StartsAt.In(test.location)
				wantHour := test.series.StartHour
				if occurrence.LocalDate.String() == test.gapDate {
					wantHour++
				}
				if local.Hour() != wantHour || local.Minute() != test.series.StartMinute {
					t.Errorf("occurrence %d is %s locally, want %02d:%02d", index, local.Format(time.Kitchen), wantHour, test.series.StartMinute)
				}
				if occurrence.LocalDate != game.DateOf(occurrence.StartsAt, test.location) {
					t.Errorf("occurrence %d's local date %s doesn't match its start", index, occurrence.LocalDate)
				}
				if got := occurrence.EndsAt.Sub(occurrence.StartsAt); got != time.Duration(test.series.DurationMinutes)*time.Minute {
					t.Errorf("occurrence %d lasts %s", index, got)
				}
			}
		})
	}
}

type plannerCase struct {
	seriesID uuid.UUID
	location *time.Location
	series   game.PlannerSeries
}

// fridayGame is a weekly Friday 8:00 PM New York game with events and reminders.
func fridayGame(t *testing.T) plannerCase {
	t.Helper()
	seriesID := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	return plannerCase{
		seriesID: seriesID,
		location: mustLocation(t, "America/New_York"),
		series: game.PlannerSeries{ID: seriesID, Title: "Friday game", Announce: true, Remind: true,
			Series: game.SessionSeries{Weekdays: []time.Weekday{time.Friday}, EveryWeeks: 1, FirstDate: date(t, "2026-10-02"), StartHour: 20, DurationMinutes: 240}},
	}
}

func jobKinds(plan game.SessionPlan) []game.SessionJobKind {
	kinds := []game.SessionJobKind{}
	for _, job := range plan.Due {
		kinds = append(kinds, job.Kind)
	}
	return kinds
}

func requireJobs(t *testing.T, plan game.SessionPlan, want ...game.SessionJobKind) {
	t.Helper()
	got := jobKinds(plan)
	if len(got) != len(want) {
		t.Fatalf("due jobs = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("due jobs = %v, want %v", got, want)
		}
	}
}

func requireWake(t *testing.T, plan game.SessionPlan, want string) {
	t.Helper()
	if plan.NextWake == nil || plan.NextWake.Format(time.RFC3339) != want {
		t.Fatalf("next wake = %v, want %s", plan.NextWake, want)
	}
}

func TestPlanCreatesTheNextEventAndRollsAfterItEnds(t *testing.T) {
	friday := fridayGame(t)
	// Monday 2 November: the next Friday is the 6th, the first after the DST change, at 01:00 UTC.
	now := instant(t, "2026-11-02T15:00:00Z")
	plan := planFor(now, friday, nil)
	requireJobs(t, plan, game.JobCreateEvent)
	if job := plan.Due[0]; job.Occurrence.StartsAt.Format(time.RFC3339) != "2026-11-07T01:00:00Z" || job.SessionID != nil || *job.SeriesID != friday.seriesID {
		t.Fatalf("event job = %+v", job)
	}
	requireWake(t, plan, "2026-11-06T01:00:00Z")

	withEvent := []testSession{{date: "2026-11-06", status: "SCHEDULED", eventStartsAt: "2026-11-07T01:00:00Z"}}
	plan = planFor(now, friday, withEvent)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-06T01:00:00Z")

	// During the game the event stays; once it ends the series rolls to the following Friday.
	playing := []testSession{{date: "2026-11-06", status: "PLAYED", eventStartsAt: "2026-11-07T01:00:00Z"}}
	plan = planFor(instant(t, "2026-11-07T02:00:00Z"), friday, playing)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-07T05:00:00Z")
	plan = planFor(instant(t, "2026-11-07T05:00:00Z"), friday, playing)
	requireJobs(t, plan, game.JobCreateEvent)
	if got := plan.Due[0].Occurrence.LocalDate.String(); got != "2026-11-13" {
		t.Fatalf("rolled to %s", got)
	}
}

func TestPlanHonoursTheEventLease(t *testing.T) {
	friday := fridayGame(t)
	now := instant(t, "2026-11-02T15:00:00Z")
	claimed := []testSession{{date: "2026-11-06", status: "SCHEDULED", claimedAt: "2026-11-02T14:58:00Z"}}
	plan := planFor(now, friday, claimed)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-02T15:03:00Z")
	plan = planFor(instant(t, "2026-11-02T15:03:00Z"), friday, claimed)
	requireJobs(t, plan, game.JobCreateEvent)
}

func TestPlanUpdatesAnOutdatedEventAndRemovesCancelledOnes(t *testing.T) {
	friday := fridayGame(t)
	now := instant(t, "2026-11-02T15:00:00Z")
	moved := []testSession{{date: "2026-11-06", status: "SCHEDULED", eventStartsAt: "2026-11-07T00:00:00Z"}}
	plan := planFor(now, friday, moved)
	requireJobs(t, plan, game.JobUpdateEvent)

	cancelled := []testSession{{date: "2026-11-06", status: "CANCELLED", eventStartsAt: "2026-11-07T01:00:00Z"}}
	plan = planFor(now, friday, cancelled)
	requireJobs(t, plan, game.JobCreateEvent, game.JobRemoveEvent)
	if got := plan.Due[0].Occurrence.LocalDate.String(); got != "2026-11-13" {
		t.Fatalf("the event moved to %s, want the following Friday", got)
	}

	orphaned := []testSession{{date: "2026-11-06", status: "SCHEDULED", eventStartsAt: "2026-11-07T01:00:00Z", orphan: true}}
	plan = planForSeries(now, friday.location, nil, orphaned, friday.seriesID)
	requireJobs(t, plan, game.JobRemoveEvent)
}

func TestPlanUpdatesAnEventWhoseLocationChanged(t *testing.T) {
	friday := fridayGame(t)
	elsewhere := []testSession{{date: "2026-11-06", status: "SCHEDULED", eventStartsAt: "2026-11-07T01:00:00Z", eventLocation: "Forge + Discord Voice"}}
	plan := planFor(instant(t, "2026-11-02T15:00:00Z"), friday, elsewhere)
	requireJobs(t, plan, game.JobUpdateEvent)
}

func TestPlanMovesTheEventOfADateTheSeriesNoLongerMeetsOn(t *testing.T) {
	friday := fridayGame(t)
	now := instant(t, "2026-11-02T15:00:00Z")
	// The series moved from Fridays to Saturdays, so Friday the 6th's event moves to Saturday the 7th.
	friday.series.Series.Weekdays = []time.Weekday{time.Saturday}
	leftBehind := []testSession{{date: "2026-11-06", status: "SCHEDULED", eventStartsAt: "2026-11-07T01:00:00Z"}}
	plan := planFor(now, friday, leftBehind)
	requireJobs(t, plan, game.JobMoveEvent)
	if job := plan.Due[0]; job.Occurrence.LocalDate.String() != "2026-11-07" || job.FromSessionID == nil || job.SessionID != nil {
		t.Fatalf("move job = %+v, want Friday's row moved to Saturday the 7th", job)
	}

	// A later date that is still one of the series' dates keeps its own event's RSVPs: restoring an
	// earlier date makes that date a new event and removes the later one.
	weekly := fridayGame(t)
	restored := []testSession{
		{date: "2026-11-06", status: "SCHEDULED"},
		{date: "2026-11-13", status: "SCHEDULED", eventStartsAt: "2026-11-14T01:00:00Z"},
	}
	plan = planFor(now, weekly, restored)
	requireJobs(t, plan, game.JobCreateEvent, game.JobRemoveEvent)
}

func TestPlanKeepsTheNextEventWhileARestoredDateIsInPlay(t *testing.T) {
	friday := fridayGame(t)
	// Friday the 6th was cancelled, its event moved to the 13th, and the DM restored the 6th mid-game.
	restoredMidGame := []testSession{
		{date: "2026-11-06", status: "SCHEDULED"},
		{date: "2026-11-13", status: "SCHEDULED", eventStartsAt: "2026-11-14T01:00:00Z"},
	}
	plan := planFor(instant(t, "2026-11-07T01:30:00Z"), friday, restoredMidGame)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-13T01:00:00Z")
	// Marked played mid-game, it still doesn't hold the series back.
	restoredMidGame[0].status = "PLAYED"
	plan = planFor(instant(t, "2026-11-07T01:30:00Z"), friday, restoredMidGame)
	requireJobs(t, plan)
}

func TestPlanRemindsADayAheadAndAfterADowntime(t *testing.T) {
	friday := fridayGame(t)
	friday.series.Announce = false
	plan := planFor(instant(t, "2026-11-02T15:00:00Z"), friday, nil)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-06T01:00:00Z")
	// Down at the reminder's time and back three hours before the game: it is sent at startup.
	plan = planFor(instant(t, "2026-11-06T22:00:00Z"), friday, nil)
	requireJobs(t, plan, game.JobRemind)
	reminded := []testSession{{date: "2026-11-06", status: "SCHEDULED", remindedAt: "2026-11-06T22:00:01Z"}}
	plan = planFor(instant(t, "2026-11-06T22:00:02Z"), friday, reminded)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-13T01:00:00Z")
	// Back only after the game started: that reminder is skipped, and the next one waits its turn.
	plan = planFor(instant(t, "2026-11-07T01:30:00Z"), friday, nil)
	requireJobs(t, plan)
	requireWake(t, plan, "2026-11-13T01:00:00Z")
	// A cancelled date gets no reminder.
	cancelled := []testSession{{date: "2026-11-06", status: "CANCELLED"}}
	plan = planFor(instant(t, "2026-11-06T22:00:00Z"), friday, cancelled)
	requireJobs(t, plan)
}

func TestPlanRemindsAOneOffStartingWithinADayExactlyOnce(t *testing.T) {
	friday := fridayGame(t)
	friday.series.Announce = false
	friday.series.Series = oneOff(t)
	// 23 hours before it starts: the reminder is due at once.
	now := instant(t, "2026-10-13T01:00:00Z")
	plan := planFor(now, friday, nil)
	requireJobs(t, plan, game.JobRemind)
	reminded := []testSession{{date: "2026-10-13", status: "SCHEDULED", remindedAt: "2026-10-13T01:00:00Z"}}
	plan = planFor(now.Add(time.Minute), friday, reminded)
	requireJobs(t, plan)
	if plan.NextWake != nil {
		t.Fatalf("a reminded one-off still wakes at %v", plan.NextWake)
	}
}

// oneOff is a one-off at 8:00 PM New York on Tuesday 13 October 2026.
func oneOff(t *testing.T) game.SessionSeries {
	t.Helper()
	return game.SessionSeries{FirstDate: date(t, "2026-10-13"), StartHour: 20, DurationMinutes: 240}
}

type testSession struct {
	date          string
	status        game.SessionState
	eventStartsAt string
	claimedAt     string
	remindedAt    string
	orphan        bool
	// eventLocation is where the row's event was made; blank means plannerLocation.
	eventLocation string
}

const plannerLocation = "Online"

func planFor(now time.Time, test plannerCase, sessions []testSession) game.SessionPlan {
	return planForSeries(now, test.location, []game.PlannerSeries{test.series}, sessions, test.seriesID)
}

func planForSeries(now time.Time, location *time.Location, series []game.PlannerSeries, sessions []testSession, seriesID uuid.UUID) game.SessionPlan {
	rows := []game.PlannerSession{}
	for _, session := range sessions {
		localDate, _ := game.ParseLocalDate(session.date)
		row := game.PlannerSession{ID: uuid.New(), LocalDate: localDate, Status: session.status, EventTitle: "Friday game", EventDurationMinutes: 240, EventLocation: plannerLocation}
		if session.eventLocation != "" {
			row.EventLocation = session.eventLocation
		}
		if !session.orphan {
			row.SeriesID = &seriesID
		}
		if session.eventStartsAt != "" {
			row.HasDiscordEvent = true
			row.EventStartsAt, _ = time.Parse(time.RFC3339, session.eventStartsAt)
		}
		if session.claimedAt != "" {
			claimedAt, _ := time.Parse(time.RFC3339, session.claimedAt)
			row.DiscordClaimedAt = &claimedAt
		}
		if session.remindedAt != "" {
			remindedAt, _ := time.Parse(time.RFC3339, session.remindedAt)
			row.RemindedAt = &remindedAt
		}
		rows = append(rows, row)
	}
	return game.PlanSessions(now, location, plannerLocation, series, rows)
}
