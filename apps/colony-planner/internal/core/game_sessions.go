package core

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// SessionStatus is a session date's state (spec §4.5).
type SessionStatus = store.SessionStatus

const (
	SessionStatusScheduled = store.SessionStatusSCHEDULED
	SessionStatusCancelled = store.SessionStatusCANCELLED
	SessionStatusPlayed    = store.SessionStatusPLAYED
)

// AllSessionStatusValues lists every status, for the API's enum schema.
func AllSessionStatusValues() []SessionStatus {
	return store.AllSessionStatusValues()
}

const (
	SessionSeriesLimit          = 50
	sessionTitleMaxLength       = 80
	sessionDescriptionMaxLength = 2000
	sessionRecapMaxLength       = 20000
	minimumSessionMinutes       = 15
	maximumSessionMinutes       = 480
	upcomingSessionWeeks        = 12
	playedSessionListLimit      = 200
	discordEventLocationMax     = 100
	// The ICS feed covers the past 30 days and the next 6 months (spec §5.11).
	calendarPastDays    = 30
	calendarAheadMonths = 6
	// plannerLookBehind keeps a game in progress and its event in the planner's rows.
	plannerLookBehind = 24 * time.Hour
)

var (
	errSessionNotFound       = NotFound("That session doesn't exist.")
	errSessionSeriesNotFound = NotFound("That series doesn't exist.")
	errCalendarNotFound      = NotFound("That calendar doesn't exist.")
	// icsTokenPattern is the column's own CHECK, so bytes Postgres would refuse as text never reach it.
	icsTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43,64}$`)
)

// SessionPlanner is Beny Bot's side of sessions (spec §7.3, §7.4): a seam beside RequestNotifier.
// Core calls it after commit; it never blocks or fails the operation.
type SessionPlanner interface {
	// SessionsChanged asks the planner to re-plan after a series, session or settings write.
	SessionsChanged(campaignID uuid.UUID)
	// SessionCancelled posts the cancellation notice to the reminder channel, falling back to the
	// announce channel. The planner removes the date's Discord event itself.
	SessionCancelled(requestContext context.Context, cancelled CancelledSession)
	// DiscordEventsAbandoned deletes, best effort, the events Beny Bot kept for upcoming dates in a server
	// the campaign was just unlinked from; core has already forgotten them.
	DiscordEventsAbandoned(requestContext context.Context, abandoned AbandonedDiscordEvents)
}

// AbandonedDiscordEvents are a campaign's upcoming events in the server it was unlinked from.
type AbandonedDiscordEvents struct {
	CampaignID      uuid.UUID
	GuildID         string
	DiscordEventIDs []string
}

// CancelledSession is the notice's content: the series' title and the date that won't happen.
type CancelledSession struct {
	CampaignID uuid.UUID
	SessionID  uuid.UUID
	Title      string
	StartsAt   time.Time
}

// NoopSessionPlanner is used while Beny Bot is off and in tests.
type NoopSessionPlanner struct{}

func (NoopSessionPlanner) SessionsChanged(uuid.UUID) {}

func (NoopSessionPlanner) SessionCancelled(context.Context, CancelledSession) {}

func (NoopSessionPlanner) DiscordEventsAbandoned(context.Context, AbandonedDiscordEvents) {}

// SetSessionPlanner attaches Beny Bot's planner once the bot exists, like SetRequestNotifier.
func (service *Service) SetSessionPlanner(planner SessionPlanner) {
	service.sessionPlanner = planner
}

// SessionReference names a session where another page links to it: its title and date.
type SessionReference struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Title     string    `json:"title"`
	LocalDate string    `json:"localDate" doc:"YYYY-MM-DD in the table's timezone"`
	StartsAt  time.Time `json:"startsAt"`
}

// SessionSeries is a weekly or every-other-week slot, or a one-off (everyWeeks null), in local wall time.
type SessionSeries struct {
	ID              uuid.UUID `json:"id" format:"uuid"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Weekdays        []int64   `json:"weekdays" doc:"0 = Sunday … 6 = Saturday; empty for a one-off"`
	EveryWeeks      *int64    `json:"everyWeeks" doc:"1 or 2; null for a one-off on firstDate"`
	FirstDate       string    `json:"firstDate" doc:"YYYY-MM-DD"`
	LastDate        *string   `json:"lastDate" doc:"YYYY-MM-DD; null for none"`
	StartLocal      string    `json:"startLocal" doc:"HH:MM in the table's timezone"`
	DurationMinutes int64     `json:"durationMinutes"`
	Announce        bool      `json:"announce" doc:"Beny Bot keeps a Discord event on the next date"`
	Remind          bool      `json:"remind" doc:"Beny Bot posts a reminder 24 hours ahead"`
}

// SessionSeriesInput creates or replaces a series (PUT semantics).
type SessionSeriesInput struct {
	Title           string  `json:"title" doc:"1 to 80 characters after trimming"`
	Description     string  `json:"description" doc:"At most 2000 characters"`
	Weekdays        []int64 `json:"weekdays" doc:"0 = Sunday … 6 = Saturday; at least one for a series, none for a one-off"`
	EveryWeeks      *int64  `json:"everyWeeks" doc:"1 (weekly) or 2 (every other week); null for a one-off on firstDate"`
	FirstDate       string  `json:"firstDate" doc:"YYYY-MM-DD"`
	LastDate        *string `json:"lastDate" doc:"YYYY-MM-DD on or after firstDate; null for none, and always null for a one-off"`
	StartLocal      string  `json:"startLocal" doc:"HH:MM, 24-hour, in the table's timezone"`
	DurationMinutes int64   `json:"durationMinutes" doc:"15 to 480"`
	Announce        bool    `json:"announce"`
	Remind          bool    `json:"remind"`
}

// Session is one date: an upcoming occurrence (whose id is null until something is stored for it) or
// a stored row. Sessions have no secrets; every active member reads them.
type Session struct {
	ID              *uuid.UUID    `json:"id" format:"uuid" nullable:"true" doc:"null until the date has a stored row"`
	SeriesID        *uuid.UUID    `json:"seriesId" format:"uuid" nullable:"true" doc:"null once its series is deleted"`
	LocalDate       string        `json:"localDate" doc:"YYYY-MM-DD in the table's timezone"`
	StartsAt        time.Time     `json:"startsAt"`
	EndsAt          time.Time     `json:"endsAt"`
	DurationMinutes int64         `json:"durationMinutes"`
	Title           string        `json:"title"`
	Status          SessionStatus `json:"status"`
	Recap           string        `json:"recap"`
	ColonyDayStart  *int64        `json:"colonyDayStart" doc:"The colony day it was played on; null until played"`
	ColonyDayEnd    *int64        `json:"colonyDayEnd" doc:"The colony day it ended on; null until ended"`
}

// SessionDateInput names an occurrence by its series and local date, the key a cancellation survives on.
type SessionDateInput struct {
	SeriesID  uuid.UUID `json:"seriesId" format:"uuid"`
	LocalDate string    `json:"localDate" doc:"YYYY-MM-DD"`
}

// SessionDaysInput is the DM's edit of a played session's colony-day stamps.
type SessionDaysInput struct {
	ColonyDayStart int64  `json:"colonyDayStart" doc:"0 or more"`
	ColonyDayEnd   *int64 `json:"colonyDayEnd" doc:"On or after colonyDayStart; null while it hasn't ended"`
}

type SessionRecapInput struct {
	Recap string `json:"recap" doc:"At most 20000 characters; line breaks allowed"`
}

type SessionTitleInput struct {
	Title string `json:"title" doc:"1 to 80 characters after trimming"`
}

type NpcFirstSessionInput struct {
	SessionID *uuid.UUID `json:"sessionId" format:"uuid" nullable:"true" doc:"A session of this campaign; null clears it"`
}

// CalendarLink is the campaign's ICS feed, which every member may subscribe to.
type CalendarLink struct {
	Path string `json:"path" doc:"/calendar/{token}.ics on this site"`
}

func localDateText(date pgtype.Date) string {
	return date.Time.Format(time.DateOnly)
}

func toPgDate(date game.LocalDate) pgtype.Date {
	return pgtype.Date{Time: time.Date(date.Year, date.Month, date.Day, 0, 0, 0, 0, time.UTC), Valid: true}
}

func fromPgDate(date pgtype.Date) game.LocalDate {
	return game.DateOf(date.Time, time.UTC)
}

func int64PointerFrom16(value *int16) *int64 {
	if value == nil {
		return nil
	}
	converted := int64(*value)
	return &converted
}

func toSessionSeries(row store.SessionSeries) SessionSeries {
	weekdays := make([]int64, 0, len(row.Weekdays))
	for _, weekday := range row.Weekdays {
		weekdays = append(weekdays, int64(weekday))
	}
	series := SessionSeries{
		ID:              row.ID,
		Title:           row.Title,
		Description:     row.Description,
		Weekdays:        weekdays,
		EveryWeeks:      int64PointerFrom16(row.EveryWeeks),
		FirstDate:       localDateText(row.FirstDate),
		StartLocal:      fmt.Sprintf("%02d:%02d", row.StartLocal.Microseconds/3_600_000_000, row.StartLocal.Microseconds/60_000_000%60),
		DurationMinutes: int64(row.DurationMinutes),
		Announce:        row.Announce,
		Remind:          row.Remind,
	}
	if row.LastDate.Valid {
		lastDate := localDateText(row.LastDate)
		series.LastDate = &lastDate
	}
	return series
}

// gameSeries is the rule's view of a stored series.
func gameSeries(row store.SessionSeries) game.SessionSeries {
	series := game.SessionSeries{
		FirstDate:       fromPgDate(row.FirstDate),
		StartHour:       int(row.StartLocal.Microseconds / 3_600_000_000),
		StartMinute:     int(row.StartLocal.Microseconds / 60_000_000 % 60),
		DurationMinutes: int(row.DurationMinutes),
	}
	if row.EveryWeeks != nil {
		series.EveryWeeks = int(*row.EveryWeeks)
	}
	for _, weekday := range row.Weekdays {
		series.Weekdays = append(series.Weekdays, time.Weekday(weekday))
	}
	if row.LastDate.Valid {
		lastDate := fromPgDate(row.LastDate)
		series.LastDate = &lastDate
	}
	return series
}

func toSession(row store.Session) Session {
	sessionID := row.ID
	return Session{
		ID:              &sessionID,
		SeriesID:        row.SeriesID,
		LocalDate:       localDateText(row.LocalDate),
		StartsAt:        row.StartsAt.UTC(),
		EndsAt:          row.StartsAt.UTC().Add(time.Duration(row.DurationMinutes) * time.Minute),
		DurationMinutes: int64(row.DurationMinutes),
		Title:           row.Title,
		Status:          row.Status,
		Recap:           row.Recap,
		ColonyDayStart:  int64Pointer(row.ColonyDayStart),
		ColonyDayEnd:    int64Pointer(row.ColonyDayEnd),
	}
}

// occurrenceSession merges an occurrence with its stored row, if any. A played row keeps the time and
// title it was played with; otherwise the series decides them.
func occurrenceSession(series store.SessionSeries, occurrence game.Occurrence, row *store.Session) Session {
	if row != nil && row.Status == SessionStatusPlayed {
		return toSession(*row)
	}
	seriesID := series.ID
	session := Session{
		SeriesID:        &seriesID,
		LocalDate:       occurrence.LocalDate.String(),
		StartsAt:        occurrence.StartsAt,
		EndsAt:          occurrence.EndsAt,
		DurationMinutes: int64(series.DurationMinutes),
		Title:           series.Title,
		Status:          SessionStatusScheduled,
	}
	if row != nil {
		rowID := row.ID
		session.ID = &rowID
		session.Status = row.Status
		session.Recap = row.Recap
	}
	return session
}

// tableLocation is the campaign's zone; settings only ever hold a name UpdateSettings checked.
func tableLocation(timezone string) (*time.Location, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load the table's timezone: %w", err)
	}
	return location, nil
}

func (service *Service) now() time.Time {
	return service.clock().UTC()
}

func (service *Service) afterSessionWrite(campaignID uuid.UUID, topics ...Topic) {
	service.broker.Publish(campaignID, append([]Topic{TopicSessions}, topics...)...)
	service.sessionPlanner.SessionsChanged(campaignID)
}

// cleanSeriesInput checks a series form (spec §4.5) and returns the row's values.
func cleanSeriesInput(input SessionSeriesInput) (store.CreateSessionSeriesParams, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	var parameters store.CreateSessionSeriesParams
	var fieldError *FieldError
	parameters.Title, fieldError = cleanText("title", input.Title, sessionTitleMaxLength)
	collect(fieldError)
	if fieldError == nil && parameters.Title == "" {
		collect(&FieldError{Field: "title", Message: "Give the series a title."})
	}
	parameters.Description, fieldError = cleanMultilineText("description", input.Description, sessionDescriptionMaxLength)
	collect(fieldError)
	parameters.Weekdays = []int16{}
	switch {
	case input.EveryWeeks == nil:
		if len(input.Weekdays) > 0 {
			collect(&FieldError{Field: "weekdays", Message: "A one-off happens on its date, so it has no weekdays."})
		}
		if input.LastDate != nil {
			collect(&FieldError{Field: "lastDate", Message: "A one-off has no last date."})
		}
	case *input.EveryWeeks != 1 && *input.EveryWeeks != 2:
		collect(&FieldError{Field: "everyWeeks", Message: "Repeat every week or every other week."})
	default:
		everyWeeks := int16(*input.EveryWeeks)
		parameters.EveryWeeks = &everyWeeks
		if len(input.Weekdays) == 0 {
			collect(&FieldError{Field: "weekdays", Message: "Pick at least one weekday."})
		}
		for _, weekday := range input.Weekdays {
			if weekday < 0 || weekday > 6 {
				collect(&FieldError{Field: "weekdays", Message: "Weekdays run from 0 (Sunday) to 6 (Saturday)."})
				break
			}
			if !slices.Contains(parameters.Weekdays, int16(weekday)) {
				parameters.Weekdays = append(parameters.Weekdays, int16(weekday))
			}
		}
		slices.Sort(parameters.Weekdays)
	}
	firstDate, err := game.ParseLocalDate(input.FirstDate)
	if err != nil {
		collect(&FieldError{Field: "firstDate", Message: "Use a date like 2026-11-06."})
	} else {
		parameters.FirstDate = toPgDate(firstDate)
	}
	if input.LastDate != nil && input.EveryWeeks != nil {
		lastDate, err := game.ParseLocalDate(*input.LastDate)
		switch {
		case err != nil:
			collect(&FieldError{Field: "lastDate", Message: "Use a date like 2026-12-18."})
		case parameters.FirstDate.Valid && lastDate.Before(firstDate):
			collect(&FieldError{Field: "lastDate", Message: "The last date can't be before the first."})
		default:
			parameters.LastDate = toPgDate(lastDate)
		}
	}
	startLocal, startError := parseStartLocal(input.StartLocal)
	if startError != nil {
		collect(startError)
	} else {
		parameters.StartLocal = startLocal
	}
	if input.DurationMinutes < minimumSessionMinutes || input.DurationMinutes > maximumSessionMinutes {
		collect(&FieldError{Field: "durationMinutes", Message: "A session lasts 15 to 480 minutes."})
	} else {
		parameters.DurationMinutes = int16(input.DurationMinutes)
	}
	parameters.Announce = input.Announce
	parameters.Remind = input.Remind
	if len(fieldErrors) > 0 {
		return store.CreateSessionSeriesParams{}, Invalid("The series needs fixing.", fieldErrors...)
	}
	return parameters, nil
}

func parseStartLocal(text string) (pgtype.Time, *FieldError) {
	hours, minutes, found := strings.Cut(strings.TrimSpace(text), ":")
	hour, hourError := strconv.Atoi(hours)
	minute, minuteError := strconv.Atoi(minutes)
	if !found || len(hours) != 2 || len(minutes) != 2 || hourError != nil || minuteError != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return pgtype.Time{}, &FieldError{Field: "startLocal", Message: "Use a 24-hour time like 20:00."}
	}
	return pgtype.Time{Microseconds: int64(hour*3600+minute*60) * 1_000_000, Valid: true}, nil
}

func (service *Service) ListSessionSeries(requestContext context.Context, viewer Viewer) ([]SessionSeries, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	rows, err := service.queries.ListSessionSeries(requestContext, viewer.campaignID)
	if err != nil {
		return nil, err
	}
	series := make([]SessionSeries, 0, len(rows))
	for _, row := range rows {
		series = append(series, toSessionSeries(row))
	}
	return series, nil
}

func (service *Service) CreateSessionSeries(requestContext context.Context, viewer Viewer, input SessionSeriesInput) (SessionSeries, error) {
	if err := requireDm(viewer); err != nil {
		return SessionSeries{}, err
	}
	parameters, err := cleanSeriesInput(input)
	if err != nil {
		return SessionSeries{}, err
	}
	parameters.CampaignID = viewer.campaignID
	var series SessionSeries
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		count, err := queries.CountSessionSeries(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if count >= SessionSeriesLimit {
			return Conflict("This campaign has " + formatNumber(SessionSeriesLimit) + " series, the most it can hold. Delete one first.")
		}
		row, err := queries.CreateSessionSeries(requestContext, parameters)
		if err != nil {
			return err
		}
		series = toSessionSeries(row)
		return nil
	})
	if err != nil {
		return SessionSeries{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony)
	return series, nil
}

// UpdateSessionSeries replaces a series. Its stored dates keep their status (a cancellation is keyed by
// local date, so it survives a new start time), and those without a Discord event follow the new time
// and title; the planner moves an event itself.
func (service *Service) UpdateSessionSeries(requestContext context.Context, viewer Viewer, seriesID uuid.UUID, input SessionSeriesInput) (SessionSeries, error) {
	if err := requireDm(viewer); err != nil {
		return SessionSeries{}, err
	}
	parameters, err := cleanSeriesInput(input)
	if err != nil {
		return SessionSeries{}, err
	}
	var series SessionSeries
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		row, err := queries.UpdateSessionSeries(requestContext, store.UpdateSessionSeriesParams{
			CampaignID:      viewer.campaignID,
			ID:              seriesID,
			Title:           parameters.Title,
			Description:     parameters.Description,
			Weekdays:        parameters.Weekdays,
			EveryWeeks:      parameters.EveryWeeks,
			FirstDate:       parameters.FirstDate,
			LastDate:        parameters.LastDate,
			StartLocal:      parameters.StartLocal,
			DurationMinutes: parameters.DurationMinutes,
			Announce:        parameters.Announce,
			Remind:          parameters.Remind,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errSessionSeriesNotFound
		}
		if err != nil {
			return err
		}
		series = toSessionSeries(row)
		return refreshFollowingSessions(requestContext, queries, viewer.campaignID)
	})
	if err != nil {
		return SessionSeries{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony)
	return series, nil
}

// DeleteSessionSeries deletes a series. Its played dates, recaps and the dates NPCs were met on stay as
// history; its other dates go, except one with a Discord event, which the planner removes first.
func (service *Service) DeleteSessionSeries(requestContext context.Context, viewer Viewer, seriesID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if err := queries.DeleteIdleSeriesSessions(requestContext, store.DeleteIdleSeriesSessionsParams{CampaignID: viewer.campaignID, SeriesID: &seriesID}); err != nil {
			return err
		}
		deleted, err := queries.DeleteSessionSeries(requestContext, store.DeleteSessionSeriesParams{CampaignID: viewer.campaignID, ID: seriesID})
		if err != nil {
			return err
		}
		if deleted == 0 {
			return errSessionSeriesNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony)
	return nil
}

// refreshFollowingSessions moves every stored date that follows its series (not played, no Discord
// event) to the series' current time and title in the table's current zone.
func refreshFollowingSessions(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) error {
	settings, err := queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return campaignGone(err)
	}
	location, err := tableLocation(settings.Timezone)
	if err != nil {
		return err
	}
	rows, err := queries.ListFollowingSessions(requestContext, campaignID)
	if err != nil {
		return err
	}
	seriesRows, err := queries.ListSessionSeries(requestContext, campaignID)
	if err != nil {
		return err
	}
	seriesByID := map[uuid.UUID]store.SessionSeries{}
	for _, series := range seriesRows {
		seriesByID[series.ID] = series
	}
	for _, row := range rows {
		series, ok := seriesByID[*row.SeriesID]
		if !ok {
			continue
		}
		startsAt := gameSeries(series).StartsAt(fromPgDate(row.LocalDate), location)
		if startsAt.Equal(row.StartsAt) && row.Title == series.Title && row.DurationMinutes == series.DurationMinutes {
			continue
		}
		if err := queries.SetSessionSchedule(requestContext, store.SetSessionScheduleParams{
			CampaignID: campaignID, ID: row.ID, StartsAt: startsAt, DurationMinutes: series.DurationMinutes, Title: series.Title,
		}); err != nil {
			return err
		}
	}
	return nil
}

// loadOccurrence loads a series with the viewer's campaign and checks the date is one of its dates.
func loadOccurrence(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, input SessionDateInput) (store.SessionSeries, game.Occurrence, error) {
	series, err := queries.GetSessionSeries(requestContext, store.GetSessionSeriesParams{CampaignID: campaignID, ID: input.SeriesID})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.SessionSeries{}, game.Occurrence{}, Invalid("That series doesn't exist.", FieldError{Field: "seriesId", Message: "Pick a series of this campaign."})
	}
	if err != nil {
		return store.SessionSeries{}, game.Occurrence{}, err
	}
	date, err := game.ParseLocalDate(input.LocalDate)
	if err != nil {
		return store.SessionSeries{}, game.Occurrence{}, Invalid("That date can't be read.", FieldError{Field: "localDate", Message: "Use a date like 2026-11-06."})
	}
	rules := gameSeries(series)
	if !rules.Includes(date) {
		return store.SessionSeries{}, game.Occurrence{}, Invalid("The series doesn't meet on that date.", FieldError{Field: "localDate", Message: "Pick one of the series' dates."})
	}
	settings, err := queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return store.SessionSeries{}, game.Occurrence{}, campaignGone(err)
	}
	location, err := tableLocation(settings.Timezone)
	if err != nil {
		return store.SessionSeries{}, game.Occurrence{}, err
	}
	startsAt := rules.StartsAt(date, location)
	return series, game.Occurrence{LocalDate: date, StartsAt: startsAt, EndsAt: startsAt.Add(time.Duration(series.DurationMinutes) * time.Minute)}, nil
}

// ensureSession creates an occurrence's row on demand, or returns the stored one.
func ensureSession(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, series store.SessionSeries, occurrence game.Occurrence) (store.Session, error) {
	return queries.EnsureSession(requestContext, store.EnsureSessionParams{
		CampaignID:      campaignID,
		SeriesID:        &series.ID,
		LocalDate:       toPgDate(occurrence.LocalDate),
		StartsAt:        occurrence.StartsAt,
		DurationMinutes: series.DurationMinutes,
		Title:           series.Title,
	})
}

// CancelSessionDate cancels one date of a series. Beny Bot posts the notice, and the planner removes
// the date's event and gives the following date one.
func (service *Service) CancelSessionDate(requestContext context.Context, viewer Viewer, input SessionDateInput) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	var session Session
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		series, occurrence, err := loadOccurrence(requestContext, queries, viewer.campaignID, input)
		if err != nil {
			return err
		}
		row, err := ensureSession(requestContext, queries, viewer.campaignID, series, occurrence)
		if err != nil {
			return err
		}
		cancelled, err := queries.CancelSession(requestContext, store.CancelSessionParams{CampaignID: viewer.campaignID, ID: row.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Conflict(sessionStatusConflict(row.Status, "cancelled"))
		}
		if err != nil {
			return err
		}
		session = occurrenceSession(series, occurrence, &cancelled)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony)
	service.sessionPlanner.SessionCancelled(requestContext, CancelledSession{
		CampaignID: viewer.campaignID, SessionID: *session.ID, Title: session.Title, StartsAt: session.StartsAt,
	})
	return session, nil
}

// RestoreSessionDate puts a cancelled date back on the schedule; the planner recreates its event if it
// is now the next date.
func (service *Service) RestoreSessionDate(requestContext context.Context, viewer Viewer, input SessionDateInput) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	var session Session
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		series, occurrence, err := loadOccurrence(requestContext, queries, viewer.campaignID, input)
		if err != nil {
			return err
		}
		row, err := ensureSession(requestContext, queries, viewer.campaignID, series, occurrence)
		if err != nil {
			return err
		}
		restored, err := queries.RestoreSession(requestContext, store.RestoreSessionParams{CampaignID: viewer.campaignID, ID: row.ID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Conflict(sessionStatusConflict(row.Status, "restored"))
		}
		if err != nil {
			return err
		}
		session = occurrenceSession(series, occurrence, &restored)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony)
	return session, nil
}

func sessionStatusConflict(status SessionStatus, wanted string) string {
	switch status {
	case SessionStatusPlayed:
		return "That session was played, so it can't be " + wanted + "."
	case SessionStatusCancelled:
		return "That date is already cancelled."
	default:
		return "That date is already on the schedule."
	}
}

// MarkSessionPlayed is the DM's Mark played: the date becomes PLAYED on the colony's current day, and a
// PUBLIC SESSION_PLAYED entry links to it. It stamps a fact, not stock, so it takes the colony lock (the
// day can't move under it) but leaves the version and the undo slot alone: Undo still reverses the DM's
// last colony change, and the DM edits a wrong stamp directly.
func (service *Service) MarkSessionPlayed(requestContext context.Context, viewer Viewer, input SessionDateInput) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	var session Session
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		series, occurrence, err := loadOccurrence(requestContext, queries, viewer.campaignID, input)
		if err != nil {
			return err
		}
		row, err := ensureSession(requestContext, queries, viewer.campaignID, series, occurrence)
		if err != nil {
			return err
		}
		played, err := markPlayed(requestContext, queries, viewer, row, int64(colony.Day))
		if err != nil {
			return err
		}
		session = toSession(played)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicTimeline, TopicColony)
	return session, nil
}

func markPlayed(requestContext context.Context, queries *store.Queries, viewer Viewer, row store.Session, day int64) (store.Session, error) {
	played, err := queries.MarkSessionPlayed(requestContext, store.MarkSessionPlayedParams{CampaignID: viewer.campaignID, ID: row.ID, ColonyDayStart: int32Pointer(day)})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Session{}, Conflict(sessionStatusConflict(row.Status, "played"))
	}
	if err != nil {
		return store.Session{}, err
	}
	err = writeLooseEntry(requestContext, queries, viewer, plannedEntry{
		day:        day,
		kind:       TimelineKindSessionPlayed,
		title:      "Session played: " + played.Title,
		visibility: TimelineVisibilityPublic,
		sessionID:  &played.ID,
	})
	return played, err
}

func int32Pointer(value int64) *int32 {
	converted := int32(value)
	return &converted
}

// EndSession is the DM's End session: the played date's last colony day is today's.
func (service *Service) EndSession(requestContext context.Context, viewer Viewer, sessionID uuid.UUID) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	var session Session
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		row, err := loadSession(requestContext, queries, viewer.campaignID, sessionID)
		if err != nil {
			return err
		}
		switch {
		case row.Status != SessionStatusPlayed:
			return Conflict("Mark the session played first.")
		case row.ColonyDayEnd != nil:
			return Conflict("That session has already ended. Edit its days instead.")
		case row.ColonyDayStart != nil && colony.Day < *row.ColonyDayStart:
			return Conflict("The colony's day is before the session's first day. Edit its days instead.")
		}
		ended, err := queries.EndSession(requestContext, store.EndSessionParams{CampaignID: viewer.campaignID, ID: row.ID, ColonyDayEnd: int32Pointer(int64(colony.Day))})
		if err != nil {
			return err
		}
		session = toSession(ended)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicTimeline, TopicColony)
	return session, nil
}

func loadSession(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, sessionID uuid.UUID) (store.Session, error) {
	row, err := queries.GetSessionForUpdate(requestContext, store.GetSessionForUpdateParams{CampaignID: campaignID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Session{}, errSessionNotFound
	}
	return row, err
}

// SetSessionDays edits a played session's colony-day stamps; its SESSION_PLAYED entry moves to the new
// first day.
func (service *Service) SetSessionDays(requestContext context.Context, viewer Viewer, sessionID uuid.UUID, input SessionDaysInput) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	var fieldErrors []FieldError
	if input.ColonyDayStart < 0 || input.ColonyDayStart > game.MaxStored {
		fieldErrors = append(fieldErrors, FieldError{Field: "colonyDayStart", Message: "Pick a day from 0 up."})
	}
	if input.ColonyDayEnd != nil && (*input.ColonyDayEnd < input.ColonyDayStart || *input.ColonyDayEnd > game.MaxStored) {
		fieldErrors = append(fieldErrors, FieldError{Field: "colonyDayEnd", Message: "The last day can't be before the first."})
	}
	if len(fieldErrors) > 0 {
		return Session{}, Invalid("Those days can't be stamped.", fieldErrors...)
	}
	var dayEnd *int32
	if input.ColonyDayEnd != nil {
		dayEnd = int32Pointer(*input.ColonyDayEnd)
	}
	var session Session
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		row, err := loadSession(requestContext, queries, viewer.campaignID, sessionID)
		if err != nil {
			return err
		}
		updated, err := queries.SetSessionDays(requestContext, store.SetSessionDaysParams{
			CampaignID: viewer.campaignID, ID: row.ID, ColonyDayStart: int32Pointer(input.ColonyDayStart), ColonyDayEnd: dayEnd,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return Conflict("Only a played session has colony days.")
		}
		if err != nil {
			return err
		}
		if err := queries.MoveSessionPlayedEntries(requestContext, store.MoveSessionPlayedEntriesParams{
			CampaignID: viewer.campaignID, SessionID: &updated.ID, Day: int32(input.ColonyDayStart),
		}); err != nil {
			return err
		}
		session = toSession(updated)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicTimeline)
	return session, nil
}

// SetSessionRecap writes the DM's recap, which every member reads on the session page.
func (service *Service) SetSessionRecap(requestContext context.Context, viewer Viewer, sessionID uuid.UUID, input SessionRecapInput) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	recap, fieldError := cleanMultilineText("recap", input.Recap, sessionRecapMaxLength)
	if fieldError != nil {
		return Session{}, Invalid("The recap needs fixing.", *fieldError)
	}
	row, err := service.queries.SetSessionRecap(requestContext, store.SetSessionRecapParams{CampaignID: viewer.campaignID, ID: sessionID, Recap: recap})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, errSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony)
	return toSession(row), nil
}

// SetSessionTitle names a played session in the history ("The Wolf Den"). Upcoming dates take their
// series' title, so only a played one has its own.
func (service *Service) SetSessionTitle(requestContext context.Context, viewer Viewer, sessionID uuid.UUID, input SessionTitleInput) (Session, error) {
	if err := requireDm(viewer); err != nil {
		return Session{}, err
	}
	title, fieldError := cleanText("title", input.Title, sessionTitleMaxLength)
	if fieldError == nil && title == "" {
		fieldError = &FieldError{Field: "title", Message: "Give the session a title."}
	}
	if fieldError != nil {
		return Session{}, Invalid("The title needs fixing.", *fieldError)
	}
	var session Session
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		row, err := loadSession(requestContext, queries, viewer.campaignID, sessionID)
		if err != nil {
			return err
		}
		updated, err := queries.SetSessionTitle(requestContext, store.SetSessionTitleParams{CampaignID: viewer.campaignID, ID: row.ID, Title: title})
		if errors.Is(err, pgx.ErrNoRows) {
			return Conflict("Upcoming dates take their series' title. Rename the series instead.")
		}
		if err != nil {
			return err
		}
		session = toSession(updated)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	service.afterSessionWrite(viewer.campaignID, TopicColony, TopicNpcs)
	return session, nil
}

func (service *Service) GetSession(requestContext context.Context, viewer Viewer, sessionID uuid.UUID) (Session, error) {
	if err := requireActive(viewer); err != nil {
		return Session{}, err
	}
	row, err := service.queries.GetSession(requestContext, store.GetSessionParams{CampaignID: viewer.campaignID, ID: sessionID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, errSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

// ListUpcomingSessions is every series' dates from now (a game in progress included) through the next
// 12 weeks, oldest first, with cancelled ones marked.
func (service *Service) ListUpcomingSessions(requestContext context.Context, viewer Viewer) ([]Session, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	now := service.now()
	return service.upcomingSessions(requestContext, viewer.campaignID, now, now.AddDate(0, 0, 7*upcomingSessionWeeks), true)
}

// upcomingSessions merges every series' occurrences that haven't ended by `from` and start before `to`
// with their stored rows.
func (service *Service) upcomingSessions(requestContext context.Context, campaignID uuid.UUID, from time.Time, to time.Time, includeCancelled bool) ([]Session, error) {
	settings, err := service.queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return nil, campaignGone(err)
	}
	location, err := tableLocation(settings.Timezone)
	if err != nil {
		return nil, err
	}
	seriesRows, err := service.queries.ListSessionSeries(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	searchFrom := from.Add(-maximumSessionMinutes * time.Minute)
	rows, err := service.queries.ListSessionsByDate(requestContext, store.ListSessionsByDateParams{
		CampaignID: campaignID,
		FromDate:   toPgDate(game.DateOf(searchFrom, location).AddDays(-1)),
		ToDate:     toPgDate(game.DateOf(to, location).AddDays(1)),
	})
	if err != nil {
		return nil, err
	}
	stored := map[string]store.Session{}
	for _, row := range rows {
		if row.SeriesID != nil {
			stored[row.SeriesID.String()+localDateText(row.LocalDate)] = row
		}
	}
	sessions := []Session{}
	for _, series := range seriesRows {
		for _, occurrence := range game.Occurrences(gameSeries(series), location, searchFrom, to) {
			var row *store.Session
			if found, ok := stored[series.ID.String()+occurrence.LocalDate.String()]; ok {
				row = &found
			}
			session := occurrenceSession(series, occurrence, row)
			if !session.EndsAt.After(from) || (!includeCancelled && session.Status == SessionStatusCancelled) {
				continue
			}
			sessions = append(sessions, session)
		}
	}
	slices.SortStableFunc(sessions, func(first Session, second Session) int {
		return first.StartsAt.Compare(second.StartsAt)
	})
	return sessions, nil
}

// ListPlayedSessions is the history, newest first.
func (service *Service) ListPlayedSessions(requestContext context.Context, viewer Viewer) ([]Session, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	rows, err := service.queries.ListPlayedSessions(requestContext, store.ListPlayedSessionsParams{CampaignID: viewer.campaignID, RowLimit: playedSessionListLimit})
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSession(row))
	}
	return sessions, nil
}

// NextSession is the next scheduled date that hasn't started, for the board and /nextsession; nil
// when there is none in the next two years.
func (service *Service) NextSession(requestContext context.Context, viewer Viewer) (*Session, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	now := service.now()
	sessions, err := service.upcomingSessions(requestContext, viewer.campaignID, now, now.AddDate(2, 0, 0), false)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session.Status == SessionStatusScheduled && session.StartsAt.After(now) {
			return &session, nil
		}
	}
	return nil, nil
}

// LatestRecap is the newest played session with a recap, for the board's snippet; nil when none.
func (service *Service) LatestRecap(requestContext context.Context, viewer Viewer) (*Session, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	row, err := service.queries.LatestRecap(requestContext, viewer.campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session := toSession(row)
	return &session, nil
}

// ListSessionBands is the played sessions whose colony days meet a timeline range, for its bands.
func (service *Service) ListSessionBands(requestContext context.Context, viewer Viewer, input TimelineRangeInput) ([]Session, error) {
	if err := requireActive(viewer); err != nil {
		return nil, err
	}
	if err := validateTimelineRange(input); err != nil {
		return nil, err
	}
	rows, err := service.queries.ListSessionBands(requestContext, store.ListSessionBandsParams{
		CampaignID: viewer.campaignID,
		FromDay:    int32Pointer(input.FromDay),
		ToDay:      int32Pointer(min(input.ToDay, game.MaxStored)),
	})
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, toSession(row))
	}
	return sessions, nil
}

// SetNpcFirstSession sets or clears where the party first met an NPC (DM only).
func (service *Service) SetNpcFirstSession(requestContext context.Context, viewer Viewer, npcID uuid.UUID, input NpcFirstSessionInput) (Npc, error) {
	if err := requireDm(viewer); err != nil {
		return Npc{}, err
	}
	var npc Npc
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		if input.SessionID != nil {
			session, err := queries.GetSession(requestContext, store.GetSessionParams{CampaignID: viewer.campaignID, ID: *input.SessionID})
			if errors.Is(err, pgx.ErrNoRows) {
				return Invalid("That session doesn't exist.", FieldError{Field: "sessionId", Message: "Pick a session of this campaign."})
			}
			if err != nil {
				return err
			}
			// A first meeting happened at a game night that was played, never a cancelled or future one.
			if session.Status != SessionStatusPlayed {
				return Invalid("That session hasn't been played.", FieldError{Field: "sessionId", Message: "Pick a played session."})
			}
		}
		updated, err := queries.SetNpcFirstSession(requestContext, store.SetNpcFirstSessionParams{CampaignID: viewer.campaignID, ID: npcID, FirstSessionID: input.SessionID})
		if err != nil {
			return err
		}
		if updated == 0 {
			return errNpcNotFound
		}
		npc, err = readNpc(requestContext, queries, viewer.campaignID, npcID, true)
		return err
	})
	if err != nil {
		return Npc{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicNpcs)
	return npc, nil
}

// GetCalendarLink is the ICS feed's address; every active member may subscribe.
func (service *Service) GetCalendarLink(requestContext context.Context, viewer Viewer) (CalendarLink, error) {
	if err := requireActive(viewer); err != nil {
		return CalendarLink{}, err
	}
	settings, err := service.queries.GetSettings(requestContext, viewer.campaignID)
	if err != nil {
		return CalendarLink{}, campaignGone(err)
	}
	return CalendarLink{Path: "/calendar/" + settings.IcsToken + ".ics"}, nil
}

// RegenerateCalendarToken replaces the ICS token, so every old subscription stops working.
func (service *Service) RegenerateCalendarToken(requestContext context.Context, viewer Viewer) (CalendarLink, error) {
	if err := requireDm(viewer); err != nil {
		return CalendarLink{}, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return CalendarLink{}, err
	}
	token, err := service.queries.SetIcsToken(requestContext, store.SetIcsTokenParams{CampaignID: viewer.campaignID, IcsToken: base64.RawURLEncoding.EncodeToString(secret)})
	if err != nil {
		return CalendarLink{}, campaignGone(err)
	}
	service.broker.Publish(viewer.campaignID, TopicSessions, TopicSettings)
	return CalendarLink{Path: "/calendar/" + token + ".ics"}, nil
}

// CalendarFeed is /calendar/{token}.ics (token level): the campaign's dates from 30 days ago through the
// next 6 months as RFC 5545 text, scheduled and played ones (a deleted series' played dates included),
// so only a cancelled date leaves subscribers' calendars. A wrong token and a paused campaign are the
// same 404.
func (service *Service) CalendarFeed(requestContext context.Context, token string) (string, error) {
	if !icsTokenPattern.MatchString(token) {
		return "", errCalendarNotFound
	}
	campaign, err := service.queries.CampaignByIcsToken(requestContext, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errCalendarNotFound
	}
	if err != nil {
		return "", err
	}
	now := service.now()
	from, to := now.AddDate(0, 0, -calendarPastDays), now.AddDate(0, calendarAheadMonths, 0)
	sessions, err := service.upcomingSessions(requestContext, campaign.CampaignID, from, to, false)
	if err != nil {
		return "", err
	}
	played, err := service.queries.ListPlayedSessionsBetween(requestContext, store.ListPlayedSessionsBetweenParams{CampaignID: campaign.CampaignID, FromInstant: from, ToInstant: to})
	if err != nil {
		return "", err
	}
	listed := map[uuid.UUID]bool{}
	for _, session := range sessions {
		if session.ID != nil {
			listed[*session.ID] = true
		}
	}
	for _, row := range played {
		if !listed[row.ID] {
			sessions = append(sessions, toSession(row))
		}
	}
	slices.SortStableFunc(sessions, func(first Session, second Session) int {
		return first.StartsAt.Compare(second.StartsAt)
	})
	return calendarText(campaign.CampaignName, campaign.DiscordEventLocation, sessions, now), nil
}

// calendarText writes RFC 5545: CRLF lines folded at 75 octets, escaped text, UTC times, and a UID per
// series and local date, so a calendar keeps the same event when its time changes.
func calendarText(calendarName string, location string, sessions []Session, now time.Time) string {
	var builder strings.Builder
	write := func(line string) {
		builder.WriteString(foldCalendarLine(line))
		builder.WriteString("\r\n")
	}
	const stampLayout = "20060102T150405Z"
	write("BEGIN:VCALENDAR")
	write("VERSION:2.0")
	write("PRODID:-//Colony Planner//Sessions//EN")
	write("CALSCALE:GREGORIAN")
	write("METHOD:PUBLISH")
	write("X-WR-CALNAME:" + escapeCalendarText(calendarName))
	for _, session := range sessions {
		uid := "session-" + strings.ReplaceAll(session.LocalDate, "-", "")
		switch {
		case session.SeriesID != nil:
			uid = session.SeriesID.String() + "-" + strings.ReplaceAll(session.LocalDate, "-", "")
		case session.ID != nil:
			// A deleted series' played date: its row is the only stable name it has left.
			uid = "session-" + session.ID.String()
		}
		write("BEGIN:VEVENT")
		write("UID:" + uid + "@colony-planner")
		write("DTSTAMP:" + now.UTC().Format(stampLayout))
		write("DTSTART:" + session.StartsAt.UTC().Format(stampLayout))
		write("DTEND:" + session.EndsAt.UTC().Format(stampLayout))
		write("SUMMARY:" + escapeCalendarText(session.Title))
		write("LOCATION:" + escapeCalendarText(location))
		write("END:VEVENT")
	}
	write("END:VCALENDAR")
	return builder.String()
}

// escapeCalendarText escapes a TEXT value (RFC 5545 §3.3.11).
func escapeCalendarText(text string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(text)
}

// foldCalendarLine splits a content line into 75-octet pieces, never inside a UTF-8 character, each
// continuation starting with a space (RFC 5545 §3.1).
func foldCalendarLine(line string) string {
	var builder strings.Builder
	limit := 75
	width := 0
	for _, character := range line {
		size := len(string(character))
		if width+size > limit {
			builder.WriteString("\r\n ")
			width = 1
		}
		builder.WriteRune(character)
		width += size
	}
	return builder.String()
}
