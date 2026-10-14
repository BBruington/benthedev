package game

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// LocalDate is a calendar date in the table's timezone, with no time or zone of its own.
type LocalDate struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf is the local date of an instant in location.
func DateOf(instant time.Time, location *time.Location) LocalDate {
	year, month, day := instant.In(location).Date()
	return LocalDate{Year: year, Month: month, Day: day}
}

// ParseLocalDate reads "2026-11-06".
func ParseLocalDate(text string) (LocalDate, error) {
	parsed, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return LocalDate{}, err
	}
	return DateOf(parsed, time.UTC), nil
}

func (date LocalDate) String() string {
	return date.midnightUTC().Format(time.DateOnly)
}

// midnightUTC carries the date in UTC so day arithmetic never meets a DST change.
func (date LocalDate) midnightUTC() time.Time {
	return time.Date(date.Year, date.Month, date.Day, 0, 0, 0, 0, time.UTC)
}

func (date LocalDate) AddDays(days int) LocalDate {
	return DateOf(date.midnightUTC().AddDate(0, 0, days), time.UTC)
}

func (date LocalDate) Weekday() time.Weekday {
	return date.midnightUTC().Weekday()
}

func (date LocalDate) Before(other LocalDate) bool {
	return date.midnightUTC().Before(other.midnightUTC())
}

func (date LocalDate) daysSince(other LocalDate) int {
	return int(date.midnightUTC().Sub(other.midnightUTC()) / (24 * time.Hour))
}

// SessionSeries is a slot in local wall time (spec §4.5). EveryWeeks is 1 or 2, or 0 for a one-off on
// FirstDate (whose Weekdays are then ignored).
type SessionSeries struct {
	Weekdays        []time.Weekday
	EveryWeeks      int
	FirstDate       LocalDate
	LastDate        *LocalDate
	StartHour       int
	StartMinute     int
	DurationMinutes int
}

// Occurrence is one date of a series as UTC instants.
type Occurrence struct {
	LocalDate LocalDate
	StartsAt  time.Time
	EndsAt    time.Time
}

// StartsAt is the instant the series starts on date: its wall time in the table's zone, so 8:00 PM
// stays 8:00 PM across DST changes (spec §5.11). A wall time the fall-back overlap reads twice is the
// first of the two; one inside a spring-forward gap is read with the offset before the gap, so it starts
// after the skipped hour, as RFC 5545 reads a missing local time.
func (series SessionSeries) StartsAt(date LocalDate, location *time.Location) time.Time {
	wallTime := time.Date(date.Year, date.Month, date.Day, series.StartHour, series.StartMinute, 0, 0, time.UTC)
	guess := time.Date(date.Year, date.Month, date.Day, series.StartHour, series.StartMinute, 0, 0, location)
	// Go picks the overlap's instant by the zone's sign, so try the offset on each side of the day.
	var earliestReading, latestCandidate time.Time
	for _, probe := range []time.Time{guess.Add(-12 * time.Hour), guess, guess.Add(12 * time.Hour)} {
		_, offsetSeconds := probe.In(location).Zone()
		candidate := wallTime.Add(-time.Duration(offsetSeconds) * time.Second)
		if latestCandidate.IsZero() || candidate.After(latestCandidate) {
			latestCandidate = candidate
		}
		local := candidate.In(location)
		readsAsWallTime := local.Day() == date.Day && local.Hour() == series.StartHour && local.Minute() == series.StartMinute
		if readsAsWallTime && (earliestReading.IsZero() || candidate.Before(earliestReading)) {
			earliestReading = candidate
		}
	}
	if !earliestReading.IsZero() {
		return earliestReading.UTC()
	}
	return latestCandidate.UTC()
}

func (series SessionSeries) occurrenceOn(date LocalDate, location *time.Location) Occurrence {
	startsAt := series.StartsAt(date, location)
	return Occurrence{LocalDate: date, StartsAt: startsAt, EndsAt: startsAt.Add(time.Duration(series.DurationMinutes) * time.Minute)}
}

// Includes says whether the series meets on date. Every-other-week parity counts whole weeks from
// FirstDate itself, so the weekdays of one game week (a Saturday and the Sunday after it) stay together.
func (series SessionSeries) Includes(date LocalDate) bool {
	if date.Before(series.FirstDate) || (series.LastDate != nil && series.LastDate.Before(date)) {
		return false
	}
	if series.EveryWeeks == 0 {
		return date == series.FirstDate
	}
	if !slices.Contains(series.Weekdays, date.Weekday()) {
		return false
	}
	return (date.daysSince(series.FirstDate)/7)%series.EveryWeeks == 0
}

// Occurrences lists the series' dates that start in [from, to), oldest first, as UTC instants. The web,
// the ICS feed, reminders, Discord events and /nextsession all call it.
func Occurrences(series SessionSeries, location *time.Location, from time.Time, to time.Time) []Occurrence {
	occurrences := []Occurrence{}
	if !from.Before(to) {
		return occurrences
	}
	// A day either side covers every zone offset between the instants and their local dates.
	first := DateOf(from, location).AddDays(-1)
	if first.Before(series.FirstDate) {
		first = series.FirstDate
	}
	last := DateOf(to, location).AddDays(1)
	if series.LastDate != nil && series.LastDate.Before(last) {
		last = *series.LastDate
	}
	if series.EveryWeeks == 0 && series.FirstDate.Before(last) {
		last = series.FirstDate
	}
	for date := first; !last.Before(date); date = date.AddDays(1) {
		if !series.Includes(date) {
			continue
		}
		occurrence := series.occurrenceOn(date, location)
		if !occurrence.StartsAt.Before(from) && occurrence.StartsAt.Before(to) {
			occurrences = append(occurrences, occurrence)
		}
	}
	return occurrences
}

// SessionState is a session row's status as the planner sees it; no row means scheduled.
type SessionState string

const (
	SessionScheduled SessionState = "SCHEDULED"
	SessionCancelled SessionState = "CANCELLED"
	SessionPlayed    SessionState = "PLAYED"
)

const (
	// DiscordEventLease is how long a claim to create a Discord event holds before another planner may
	// try again (spec §7.3).
	DiscordEventLease = 5 * time.Minute
	// ReminderLead is how long before the start a reminder is due.
	ReminderLead = 24 * time.Hour
	// plannerHorizon bounds how far ahead the planner looks for a series' next date; a series whose
	// next uncancelled date is further away than this is left alone until it comes into range.
	plannerHorizon = 2 * 366 * 24 * time.Hour
	// longestSession is the longest a session lasts (480 minutes), so a game in progress is still found.
	longestSession = 8 * time.Hour
)

// PlannerSeries is one series the planner looks after.
type PlannerSeries struct {
	ID       uuid.UUID
	Title    string
	Series   SessionSeries
	Announce bool
	Remind   bool
}

// PlannerSession is one stored session row. EventStartsAt, EventTitle, EventDurationMinutes and
// EventLocation are what the row's Discord event was made with, so the planner sees when an edit left
// the event behind.
type PlannerSession struct {
	ID                   uuid.UUID
	SeriesID             *uuid.UUID
	LocalDate            LocalDate
	Status               SessionState
	HasDiscordEvent      bool
	EventStartsAt        time.Time
	EventTitle           string
	EventDurationMinutes int
	EventLocation        string
	DiscordClaimedAt     *time.Time
	RemindedAt           *time.Time
}

// SessionJobKind is what a due job asks Beny Bot to do.
type SessionJobKind string

const (
	// JobCreateEvent: the series' next date has no Discord event and no live claim.
	JobCreateEvent SessionJobKind = "CREATE_EVENT"
	// JobUpdateEvent: the next date's event no longer matches its title, start, length or location.
	JobUpdateEvent SessionJobKind = "UPDATE_EVENT"
	// JobMoveEvent: the next date has no event, and an edit of the series left a later date that is no
	// longer one of its dates holding one; that event moves to the next date, keeping its RSVPs.
	JobMoveEvent SessionJobKind = "MOVE_EVENT"
	// JobRemoveEvent: a future event whose date was cancelled, whose series is gone, or which is no
	// longer its series' next date.
	JobRemoveEvent SessionJobKind = "REMOVE_EVENT"
	// JobRemind: the next scheduled date starts within 24 hours and nobody has reminded it.
	JobRemind SessionJobKind = "REMIND"
)

// SessionJob is one due job. SessionID is set when its row exists (always for update and remove);
// FromSessionID is the row whose event a move takes.
type SessionJob struct {
	Kind          SessionJobKind
	SeriesID      *uuid.UUID
	SessionID     *uuid.UUID
	FromSessionID *uuid.UUID
	Title         string
	Occurrence    Occurrence
}

// SessionPlan is what is due now and when to look again; NextWake is nil when nothing is ahead.
type SessionPlan struct {
	Due      []SessionJob
	NextWake *time.Time
}

func (plan *SessionPlan) wakeAt(instant time.Time) {
	if plan.NextWake == nil || instant.Before(*plan.NextWake) {
		wake := instant
		plan.NextWake = &wake
	}
}

type occurrenceKey struct {
	seriesID uuid.UUID
	date     LocalDate
}

// PlanSessions is the planner's pure step (spec §7.3) for one campaign: given the clock, the table's
// zone, the events' location, its series and its stored rows, it returns the jobs due now and the next
// instant anything falls due (an occurrence ending, a reminder's time, a lease expiring).
func PlanSessions(now time.Time, location *time.Location, eventLocation string, seriesList []PlannerSeries, sessions []PlannerSession) SessionPlan {
	plan := SessionPlan{Due: []SessionJob{}}
	rows := map[occurrenceKey]PlannerSession{}
	for _, session := range sessions {
		if session.SeriesID != nil {
			rows[occurrenceKey{seriesID: *session.SeriesID, date: session.LocalDate}] = session
		}
	}
	eventTargets := map[uuid.UUID]bool{}
	seriesByID := map[uuid.UUID]PlannerSeries{}
	for _, series := range seriesList {
		seriesByID[series.ID] = series
		occurrences := Occurrences(series.Series, location, now.Add(-longestSession), now.Add(plannerHorizon))
		if series.Announce {
			planEvent(&plan, now, eventLocation, series, occurrences, sessions, rows, eventTargets)
		}
		if series.Remind {
			planReminder(&plan, now, series, occurrences, rows)
		}
	}
	for _, session := range sessions {
		if !session.HasDiscordEvent || session.Status == SessionPlayed || eventTargets[session.ID] || !now.Before(session.EventStartsAt) {
			continue
		}
		job := SessionJob{Kind: JobRemoveEvent, SeriesID: session.SeriesID, SessionID: &session.ID, Title: session.EventTitle,
			Occurrence: Occurrence{LocalDate: session.LocalDate, StartsAt: session.EventStartsAt,
				EndsAt: session.EventStartsAt.Add(time.Duration(session.EventDurationMinutes) * time.Minute)}}
		if session.SeriesID != nil {
			if series, ok := seriesByID[*session.SeriesID]; ok {
				job.Title = series.Title
			}
		}
		plan.Due = append(plan.Due, job)
	}
	return plan
}

// planEvent keeps one Discord event on the series' next date that hasn't ended and isn't cancelled.
// A played date keeps its event until it ends; then the series rolls to the next date. A date that has
// started without an event can't get one (Discord refuses events in the past), so it doesn't hold the
// series back: the date after it keeps or gets its event.
func planEvent(plan *SessionPlan, now time.Time, eventLocation string, series PlannerSeries, occurrences []Occurrence, sessions []PlannerSession, rows map[occurrenceKey]PlannerSession, eventTargets map[uuid.UUID]bool) {
	for _, occurrence := range occurrences {
		if !now.Before(occurrence.EndsAt) {
			continue
		}
		row, hasRow := rows[occurrenceKey{seriesID: series.ID, date: occurrence.LocalDate}]
		if hasRow && row.Status == SessionCancelled {
			continue
		}
		hasEvent := hasRow && row.HasDiscordEvent
		if !hasEvent && !now.Before(occurrence.StartsAt) {
			continue
		}
		plan.wakeAt(occurrence.EndsAt)
		seriesID := series.ID
		job := SessionJob{SeriesID: &seriesID, Title: series.Title, Occurrence: occurrence}
		if hasRow {
			eventTargets[row.ID] = true
			job.SessionID = &row.ID
		}
		switch {
		case hasRow && row.Status == SessionPlayed:
		case hasEvent:
			if !row.EventStartsAt.Equal(occurrence.StartsAt) || row.EventTitle != series.Title ||
				row.EventDurationMinutes != series.Series.DurationMinutes || row.EventLocation != eventLocation {
				job.Kind = JobUpdateEvent
				plan.Due = append(plan.Due, job)
			}
		case hasRow && row.DiscordClaimedAt != nil && now.Before(row.DiscordClaimedAt.Add(DiscordEventLease)):
			plan.wakeAt(row.DiscordClaimedAt.Add(DiscordEventLease))
		default:
			job.Kind = JobCreateEvent
			if left, found := eventLeftBehind(now, series, sessions); found {
				job.Kind = JobMoveEvent
				job.FromSessionID = &left.ID
				eventTargets[left.ID] = true
			}
			plan.Due = append(plan.Due, job)
		}
		return
	}
}

// eventLeftBehind finds the series' upcoming event on a scheduled date the series no longer meets on
// (the DM changed its weekdays or dates), the earliest if several. A date that is still one of the
// series' dates, such as a later date whose event stood in while an earlier one was cancelled, keeps
// its own event's RSVPs and is removed instead.
func eventLeftBehind(now time.Time, series PlannerSeries, sessions []PlannerSession) (PlannerSession, bool) {
	var left PlannerSession
	found := false
	for _, session := range sessions {
		if session.SeriesID == nil || *session.SeriesID != series.ID || !session.HasDiscordEvent || session.Status != SessionScheduled ||
			!now.Before(session.EventStartsAt) || series.Series.Includes(session.LocalDate) {
			continue
		}
		if !found || session.EventStartsAt.Before(left.EventStartsAt) {
			left = session
			found = true
		}
	}
	return left, found
}

// planReminder reminds the series' next scheduled date that hasn't started, 24 hours ahead. A reminder
// missed while the service was down is due at once, as long as the game hasn't started.
func planReminder(plan *SessionPlan, now time.Time, series PlannerSeries, occurrences []Occurrence, rows map[occurrenceKey]PlannerSession) {
	for _, occurrence := range occurrences {
		if !now.Before(occurrence.StartsAt) {
			continue
		}
		row, hasRow := rows[occurrenceKey{seriesID: series.ID, date: occurrence.LocalDate}]
		if hasRow && (row.Status != SessionScheduled || row.RemindedAt != nil) {
			continue
		}
		dueAt := occurrence.StartsAt.Add(-ReminderLead)
		if now.Before(dueAt) {
			plan.wakeAt(dueAt)
			return
		}
		seriesID := series.ID
		job := SessionJob{Kind: JobRemind, SeriesID: &seriesID, Title: series.Title, Occurrence: occurrence}
		if hasRow {
			job.SessionID = &row.ID
		}
		plan.Due = append(plan.Due, job)
		return
	}
}
