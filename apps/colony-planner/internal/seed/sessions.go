package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// sampleTable is one campaign's sample game: its series and the sessions played on it, which the
// colony history marks played and ends between its advances, so each band covers the days it did.
type sampleTable struct {
	service  *core.Service
	dm       core.Viewer
	series   core.SessionSeries
	sessions map[string]core.Session
}

// lastWeekday is the newest local date before today, in location, that falls on weekday.
func lastWeekday(now time.Time, location *time.Location, weekday time.Weekday) time.Time {
	local := now.In(location)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	back := (int(today.Weekday()) - int(weekday) + 7) % 7
	if back == 0 {
		back = 7
	}
	return today.AddDate(0, 0, -back)
}

func dateText(date time.Time) string {
	return date.Format(time.DateOnly)
}

func newSampleTable(requestContext context.Context, service *core.Service, dm core.Viewer, input core.SessionSeriesInput) (*sampleTable, error) {
	series, err := service.CreateSessionSeries(requestContext, dm, input)
	if err != nil {
		return nil, fmt.Errorf("series %s: %w", input.Title, err)
	}
	return &sampleTable{service: service, dm: dm, series: series, sessions: map[string]core.Session{}}, nil
}

// play marks a past date played on the colony's current day; it is not a colony mutation, so the
// version and the undo slot the later steps expect stay as they are.
func (table *sampleTable) play(requestContext context.Context, localDate string) func(int64) (core.ColonyChange, error) {
	return func(int64) (core.ColonyChange, error) {
		session, err := table.service.MarkSessionPlayed(requestContext, table.dm, core.SessionDateInput{SeriesID: table.series.ID, LocalDate: localDate})
		table.sessions[localDate] = session
		return core.ColonyChange{}, err
	}
}

// end stamps a played date's last day and gives it its own title and the DM's recap.
func (table *sampleTable) end(requestContext context.Context, localDate string, title string, recap string) func(int64) (core.ColonyChange, error) {
	return func(int64) (core.ColonyChange, error) {
		sessionID := table.sessionID(localDate)
		if _, err := table.service.EndSession(requestContext, table.dm, sessionID); err != nil {
			return core.ColonyChange{}, err
		}
		if _, err := table.service.SetSessionTitle(requestContext, table.dm, sessionID, core.SessionTitleInput{Title: title}); err != nil {
			return core.ColonyChange{}, err
		}
		_, err := table.service.SetSessionRecap(requestContext, table.dm, sessionID, core.SessionRecapInput{Recap: recap})
		return core.ColonyChange{}, err
	}
}

func (table *sampleTable) sessionID(localDate string) uuid.UUID {
	session := table.sessions[localDate]
	if session.ID == nil {
		return uuid.Nil
	}
	return *session.ID
}

// sampleColonyDates are Port Eldari's weekly Friday game at 8:00 PM New York: its last three Fridays
// were played (Days 0–7, 14–19 and 19–22), and the Friday after next is cancelled.
type sampleColonyDates struct {
	first     string
	second    string
	third     string
	cancelled string
}

func sampleColonyTableDates(now time.Time) (sampleColonyDates, error) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		return sampleColonyDates{}, err
	}
	lastFriday := lastWeekday(now, newYork, time.Friday)
	return sampleColonyDates{
		first:     dateText(lastFriday.AddDate(0, 0, -14)),
		second:    dateText(lastFriday.AddDate(0, 0, -7)),
		third:     dateText(lastFriday),
		cancelled: dateText(lastFriday.AddDate(0, 0, 14)),
	}, nil
}

func sampleColonySeries(dates sampleColonyDates) core.SessionSeriesInput {
	weekly := int64(1)
	return core.SessionSeriesInput{
		Title:           "Port Eldari Fridays",
		Description:     "The weekly game. Bring your character sheet and a snack for the table.",
		Weekdays:        []int64{int64(time.Friday)},
		EveryWeeks:      &weekly,
		FirstDate:       dates.first,
		StartLocal:      "20:00",
		DurationMinutes: 240,
		Announce:        true,
		Remind:          true,
	}
}

// finishSampleColonyTable cancels the Friday after next and records where the party first met Brannoc.
func finishSampleColonyTable(requestContext context.Context, table *sampleTable, dates sampleColonyDates) error {
	if _, err := table.service.CancelSessionDate(requestContext, table.dm, core.SessionDateInput{SeriesID: table.series.ID, LocalDate: dates.cancelled}); err != nil {
		return fmt.Errorf("cancel %s: %w", dates.cancelled, err)
	}
	found, err := table.service.ListNpcs(requestContext, table.dm, core.NpcFilter{Search: "Brannoc"})
	if err != nil {
		return err
	}
	if len(found.Npcs) != 1 {
		return fmt.Errorf("found %d NPCs named Brannoc", len(found.Npcs))
	}
	firstSession := table.sessionID(dates.first)
	if _, err := table.service.SetNpcFirstSession(requestContext, table.dm, found.Npcs[0].ID, core.NpcFirstSessionInput{SessionID: &firstSession}); err != nil {
		return fmt.Errorf("first session of the sample NPC: %w", err)
	}
	return nil
}

// secondSampleSeries is Ridgehold's game every other Saturday at 7:00 PM London; its first Saturday was
// played over Days 0–9.
func secondSampleSeries(now time.Time) (core.SessionSeriesInput, error) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		return core.SessionSeriesInput{}, err
	}
	everyOtherWeek := int64(2)
	return core.SessionSeriesInput{
		Title:           "Ridgehold Saturdays",
		Weekdays:        []int64{int64(time.Saturday)},
		EveryWeeks:      &everyOtherWeek,
		FirstDate:       dateText(lastWeekday(now, london, time.Saturday).AddDate(0, 0, -14)),
		StartLocal:      "19:00",
		DurationMinutes: 180,
		Announce:        true,
	}, nil
}
