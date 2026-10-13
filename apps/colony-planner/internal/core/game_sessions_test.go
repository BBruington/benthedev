package core_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// recordingPlanner counts re-plan signals and cancellation notices, as Beny Bot's planner receives them.
type recordingPlanner struct {
	mutex     sync.Mutex
	changes   map[uuid.UUID]int
	cancelled []core.CancelledSession
	abandoned []core.AbandonedDiscordEvents
}

func (planner *recordingPlanner) SessionsChanged(campaignID uuid.UUID) {
	planner.mutex.Lock()
	defer planner.mutex.Unlock()
	planner.changes[campaignID]++
}

func (planner *recordingPlanner) SessionCancelled(_ context.Context, cancelled core.CancelledSession) {
	planner.mutex.Lock()
	defer planner.mutex.Unlock()
	planner.cancelled = append(planner.cancelled, cancelled)
}

func (planner *recordingPlanner) DiscordEventsAbandoned(_ context.Context, abandoned core.AbandonedDiscordEvents) {
	planner.mutex.Lock()
	defer planner.mutex.Unlock()
	planner.abandoned = append(planner.abandoned, abandoned)
}

func (planner *recordingPlanner) changesFor(campaignID uuid.UUID) int {
	planner.mutex.Lock()
	defer planner.mutex.Unlock()
	return planner.changes[campaignID]
}

// sessionTest is a New York campaign with a player and a second campaign, for the M5 session rules. The
// fixture's clock reads Sunday 11 October 2026, 12:00 UTC.
type sessionTest struct {
	*fixture
	campaignID uuid.UUID
	dm         core.Viewer
	player     core.Viewer
	otherDm    core.Viewer
	planner    *recordingPlanner
}

func newSessionTest(t *testing.T) *sessionTest {
	t.Helper()
	test := newFixture(t)
	campaignID, dm := test.campaign(test.caller(discordID(1), "dm"), "Sample", "sample")
	_, player := test.player(dm, discordID(2), "ana", true)
	_, otherDm := test.campaign(test.caller(discordID(4), "hollis"), "Second", "second")
	planner := &recordingPlanner{changes: map[uuid.UUID]int{}}
	test.service.SetSessionPlanner(planner)
	for _, viewer := range []core.Viewer{dm, otherDm} {
		if _, err := test.service.UpdateSettings(test.context, viewer, core.SettingsInput{CampaignName: "Sample", ColonyName: "Sample", Timezone: "America/New_York"}); err != nil {
			t.Fatal(err)
		}
	}
	return &sessionTest{fixture: test, campaignID: campaignID, dm: dm, player: player, otherDm: otherDm, planner: planner}
}

func weekly(title string, firstDate string, startLocal string, weekdays ...int64) core.SessionSeriesInput {
	everyWeeks := int64(1)
	return core.SessionSeriesInput{Title: title, Weekdays: weekdays, EveryWeeks: &everyWeeks, FirstDate: firstDate, StartLocal: startLocal,
		DurationMinutes: 240, Announce: true, Remind: true}
}

func (test *sessionTest) series(viewer core.Viewer, input core.SessionSeriesInput) core.SessionSeries {
	test.t.Helper()
	series, err := test.service.CreateSessionSeries(test.context, viewer, input)
	if err != nil {
		test.t.Fatalf("create series: %v", err)
	}
	return series
}

func (test *sessionTest) upcoming(viewer core.Viewer) []core.Session {
	test.t.Helper()
	sessions, err := test.service.ListUpcomingSessions(test.context, viewer)
	if err != nil {
		test.t.Fatalf("list upcoming: %v", err)
	}
	return sessions
}

func sessionOn(t *testing.T, sessions []core.Session, localDate string) core.Session {
	t.Helper()
	for _, session := range sessions {
		if session.LocalDate == localDate {
			return session
		}
	}
	t.Fatalf("no session on %s in %d sessions", localDate, len(sessions))
	return core.Session{}
}

func requireKind(t *testing.T, err error, kind core.ErrorKind, field string) {
	t.Helper()
	if !isKind(err, kind) {
		t.Fatalf("got %v, want kind %d", err, kind)
	}
	if field == "" {
		return
	}
	coreError, _ := core.AsError(err)
	for _, fieldError := range coreError.Fields {
		if fieldError.Field == field {
			return
		}
	}
	t.Fatalf("got fields %+v, want one on %s", coreError.Fields, field)
}

func TestSeriesValidation(t *testing.T) {
	test := newSessionTest(t)
	two := int64(2)
	three := int64(3)
	lastDate := "2026-10-01"
	cases := map[string]struct {
		input core.SessionSeriesInput
		field string
	}{
		"blank title":            {weekly(" ", "2026-10-16", "20:00", 5), "title"},
		"no weekdays":            {weekly("Game", "2026-10-16", "20:00"), "weekdays"},
		"weekday 7":              {weekly("Game", "2026-10-16", "20:00", 7), "weekdays"},
		"every three weeks":      {core.SessionSeriesInput{Title: "Game", Weekdays: []int64{5}, EveryWeeks: &three, FirstDate: "2026-10-16", StartLocal: "20:00", DurationMinutes: 240}, "everyWeeks"},
		"one-off with weekdays":  {core.SessionSeriesInput{Title: "Game", Weekdays: []int64{5}, FirstDate: "2026-10-16", StartLocal: "20:00", DurationMinutes: 240}, "weekdays"},
		"one-off with last date": {core.SessionSeriesInput{Title: "Game", FirstDate: "2026-10-16", LastDate: &lastDate, StartLocal: "20:00", DurationMinutes: 240}, "lastDate"},
		"last before first":      {core.SessionSeriesInput{Title: "Game", Weekdays: []int64{5}, EveryWeeks: &two, FirstDate: "2026-10-16", LastDate: &lastDate, StartLocal: "20:00", DurationMinutes: 240}, "lastDate"},
		"bad date":               {weekly("Game", "16/10/2026", "20:00", 5), "firstDate"},
		"bad time":               {weekly("Game", "2026-10-16", "8pm", 5), "startLocal"},
		"25 o'clock":             {weekly("Game", "2026-10-16", "25:00", 5), "startLocal"},
		"too short":              {core.SessionSeriesInput{Title: "Game", Weekdays: []int64{5}, EveryWeeks: &two, FirstDate: "2026-10-16", StartLocal: "20:00", DurationMinutes: 14}, "durationMinutes"},
		"too long":               {core.SessionSeriesInput{Title: "Game", Weekdays: []int64{5}, EveryWeeks: &two, FirstDate: "2026-10-16", StartLocal: "20:00", DurationMinutes: 481}, "durationMinutes"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := test.service.CreateSessionSeries(test.context, test.dm, testCase.input)
			requireKind(t, err, core.KindInvalid, testCase.field)
		})
	}
	_, err := test.service.CreateSessionSeries(test.context, test.player, weekly("Game", "2026-10-16", "20:00", 5))
	requireKind(t, err, core.KindForbidden, "")
	series := test.series(test.dm, weekly("Game", "2026-10-16", "20:00", 5, 1, 5))
	if len(series.Weekdays) != 2 || series.Weekdays[0] != 1 || series.Weekdays[1] != 5 || series.StartLocal != "20:00" {
		t.Fatalf("series = %+v; want weekdays sorted without repeats", series)
	}
	oneOff := test.series(test.dm, core.SessionSeriesInput{Title: "Finale", FirstDate: "2026-10-20", StartLocal: "19:30", DurationMinutes: 300})
	if oneOff.EveryWeeks != nil || len(oneOff.Weekdays) != 0 {
		t.Fatalf("one-off = %+v", oneOff)
	}
}

func TestUpcomingDatesCancelAndRestoreByLocalDate(t *testing.T) {
	test := newSessionTest(t)
	series := test.series(test.dm, weekly("Friday game", "2026-10-16", "20:00", 5))
	if test.planner.changesFor(test.campaignID) == 0 {
		t.Fatal("creating a series sent no re-plan signal")
	}
	upcoming := test.upcoming(test.player)
	if len(upcoming) != 12 || upcoming[0].LocalDate != "2026-10-16" || upcoming[0].ID != nil {
		t.Fatalf("upcoming = %d dates starting %+v", len(upcoming), upcoming[0])
	}
	if got := upcoming[0].StartsAt.Format(time.RFC3339); got != "2026-10-17T00:00:00Z" {
		t.Fatalf("first Friday starts %s", got)
	}
	// The first Friday after the November DST change is still 8:00 PM in New York.
	if got := sessionOn(t, upcoming, "2026-11-06").StartsAt.Format(time.RFC3339); got != "2026-11-07T01:00:00Z" {
		t.Fatalf("6 November starts %s", got)
	}

	date := core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-23"}
	cancelled, err := test.service.CancelSessionDate(test.context, test.dm, date)
	if err != nil || cancelled.Status != core.SessionStatusCancelled || cancelled.ID == nil {
		t.Fatalf("cancel = %+v, %v", cancelled, err)
	}
	if len(test.planner.cancelled) != 1 || test.planner.cancelled[0].Title != "Friday game" {
		t.Fatalf("cancellation notices = %+v", test.planner.cancelled)
	}
	_, err = test.service.CancelSessionDate(test.context, test.dm, date)
	requireKind(t, err, core.KindConflict, "")
	_, err = test.service.CancelSessionDate(test.context, test.player, date)
	requireKind(t, err, core.KindForbidden, "")
	_, err = test.service.CancelSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-22"})
	requireKind(t, err, core.KindInvalid, "localDate")

	// A new start time keeps the cancellation, which is keyed by local date.
	moved := weekly("Friday game", "2026-10-16", "19:00", 5)
	if _, err := test.service.UpdateSessionSeries(test.context, test.dm, series.ID, moved); err != nil {
		t.Fatal(err)
	}
	upcoming = test.upcoming(test.player)
	still := sessionOn(t, upcoming, "2026-10-23")
	if still.Status != core.SessionStatusCancelled || still.StartsAt.Format(time.RFC3339) != "2026-10-23T23:00:00Z" {
		t.Fatalf("after the move, 23 October = %+v", still)
	}
	next, err := test.service.NextSession(test.context, test.player)
	if err != nil || next == nil || next.LocalDate != "2026-10-16" {
		t.Fatalf("next = %+v, %v", next, err)
	}
	test.clock.advance(6 * 24 * time.Hour)
	if next, err = test.service.NextSession(test.context, test.player); err != nil || next == nil || next.LocalDate != "2026-10-30" {
		t.Fatalf("next after the 16th, skipping the cancelled 23rd = %+v, %v", next, err)
	}
	restored, err := test.service.RestoreSessionDate(test.context, test.dm, date)
	if err != nil || restored.Status != core.SessionStatusScheduled || *restored.ID != *cancelled.ID {
		t.Fatalf("restore = %+v, %v", restored, err)
	}
	_, err = test.service.RestoreSessionDate(test.context, test.dm, date)
	requireKind(t, err, core.KindConflict, "")
}

func TestMarkPlayedAndEndStampColonyDays(t *testing.T) {
	test := newSessionTest(t)
	series := test.series(test.dm, weekly("Friday game", "2026-10-09", "20:00", 5))
	board, err := test.service.GetColonyBoard(test.context, test.dm)
	if err != nil {
		t.Fatal(err)
	}
	change, err := test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: 3, ExpectedVersion: board.Version})
	if err != nil {
		t.Fatal(err)
	}
	before, err := test.service.GetColonyBoard(test.context, test.dm)
	if err != nil {
		t.Fatal(err)
	}
	played, err := test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-09"})
	if err != nil || played.Status != core.SessionStatusPlayed || played.ColonyDayStart == nil || *played.ColonyDayStart != 3 || played.ColonyDayEnd != nil {
		t.Fatalf("mark played = %+v, %v", played, err)
	}
	after, err := test.service.GetColonyBoard(test.context, test.dm)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.Dm == nil || after.Dm.LastChange == nil || after.Dm.LastChange.GroupID != change.GroupID {
		t.Fatalf("mark played moved the version or the undo slot: %+v → %+v", before, after)
	}
	page, err := test.service.ListTimeline(test.context, test.player, core.TimelineRangeInput{FromDay: 0, ToDay: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Kind == core.TimelineKindSessionPlayed && entry.SessionID != nil && *entry.SessionID == *played.ID && entry.Title == "Session played: Friday game" {
			found = true
		}
	}
	if !found {
		t.Fatalf("players don't see the SESSION_PLAYED entry: %+v", page.Entries)
	}
	_, err = test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-09"})
	requireKind(t, err, core.KindConflict, "")
	// Still in play, the band runs on to today, so a timeline page after its first day still has it.
	inPlay, err := test.service.ListSessionBands(test.context, test.player, core.TimelineRangeInput{FromDay: 7, ToDay: 13})
	if err != nil || len(inPlay) != 1 || *inPlay[0].ID != *played.ID {
		t.Fatalf("a later page's bands while the session is in play = %+v, %v", inPlay, err)
	}

	if _, err := test.service.Advance(test.context, test.dm, core.AdvanceInput{Days: 2, ExpectedVersion: after.Version}); err != nil {
		t.Fatal(err)
	}
	ended, err := test.service.EndSession(test.context, test.dm, *played.ID)
	if err != nil || ended.ColonyDayEnd == nil || *ended.ColonyDayEnd != 5 {
		t.Fatalf("end = %+v, %v", ended, err)
	}
	_, err = test.service.EndSession(test.context, test.dm, *played.ID)
	requireKind(t, err, core.KindConflict, "")
	end := int64(2)
	_, err = test.service.SetSessionDays(test.context, test.dm, *played.ID, core.SessionDaysInput{ColonyDayStart: 3, ColonyDayEnd: &end})
	requireKind(t, err, core.KindInvalid, "colonyDayEnd")
	end = 6
	edited, err := test.service.SetSessionDays(test.context, test.dm, *played.ID, core.SessionDaysInput{ColonyDayStart: 2, ColonyDayEnd: &end})
	if err != nil || *edited.ColonyDayStart != 2 || *edited.ColonyDayEnd != 6 {
		t.Fatalf("edit days = %+v, %v", edited, err)
	}
	bands, err := test.service.ListSessionBands(test.context, test.player, core.TimelineRangeInput{FromDay: 6, ToDay: 12})
	if err != nil || len(bands) != 1 || *bands[0].ID != *played.ID {
		t.Fatalf("bands = %+v, %v", bands, err)
	}
	if bands, err = test.service.ListSessionBands(test.context, test.player, core.TimelineRangeInput{FromDay: 7, ToDay: 12}); err != nil || len(bands) != 0 {
		t.Fatalf("bands after the session = %+v, %v", bands, err)
	}
	moved, err := test.service.ListTimeline(test.context, test.player, core.TimelineRangeInput{FromDay: 0, ToDay: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range moved.Entries {
		if entry.Kind == core.TimelineKindSessionPlayed && entry.Day != 2 {
			t.Errorf("the SESSION_PLAYED entry stayed on day %d after the first day moved to 2", entry.Day)
		}
	}

	recapped, err := test.service.SetSessionRecap(test.context, test.dm, *played.ID, core.SessionRecapInput{Recap: "  The party cleared the wolf den.\nThorin kept the map.  "})
	if err != nil || recapped.Recap != "The party cleared the wolf den.\nThorin kept the map." {
		t.Fatalf("recap = %q, %v", recapped.Recap, err)
	}
	latest, err := test.service.LatestRecap(test.context, test.player)
	if err != nil || latest == nil || *latest.ID != *played.ID {
		t.Fatalf("latest recap = %+v, %v", latest, err)
	}
	titled, err := test.service.SetSessionTitle(test.context, test.dm, *played.ID, core.SessionTitleInput{Title: "The Wolf Den"})
	if err != nil || titled.Title != "The Wolf Den" {
		t.Fatalf("title = %+v, %v", titled, err)
	}
	cancelled, err := test.service.CancelSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = test.service.SetSessionTitle(test.context, test.dm, *cancelled.ID, core.SessionTitleInput{Title: "Renamed"})
	requireKind(t, err, core.KindConflict, "")
	_, err = test.service.EndSession(test.context, test.dm, *cancelled.ID)
	requireKind(t, err, core.KindConflict, "")
	history, err := test.service.ListPlayedSessions(test.context, test.player)
	if err != nil || len(history) != 1 || history[0].Title != "The Wolf Den" {
		t.Fatalf("history = %+v, %v", history, err)
	}

	// Deleting the series keeps the played date and its recap; the cancelled date goes.
	if err := test.service.DeleteSessionSeries(test.context, test.dm, series.ID); err != nil {
		t.Fatal(err)
	}
	kept, err := test.service.GetSession(test.context, test.player, *played.ID)
	if err != nil || kept.SeriesID != nil || kept.Recap == "" {
		t.Fatalf("played date after the series' delete = %+v, %v", kept, err)
	}
	_, err = test.service.GetSession(test.context, test.player, *cancelled.ID)
	requireKind(t, err, core.KindNotFound, "")
}

func TestSessionIDsStayInTheirCampaign(t *testing.T) {
	test := newSessionTest(t)
	series := test.series(test.dm, weekly("Friday game", "2026-10-09", "20:00", 5))
	played, err := test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-09"})
	if err != nil {
		t.Fatal(err)
	}
	foreignDate := core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}
	_, err = test.service.CancelSessionDate(test.context, test.otherDm, foreignDate)
	requireKind(t, err, core.KindInvalid, "seriesId")
	_, err = test.service.MarkSessionPlayed(test.context, test.otherDm, foreignDate)
	requireKind(t, err, core.KindInvalid, "seriesId")
	_, err = test.service.UpdateSessionSeries(test.context, test.otherDm, series.ID, weekly("Taken", "2026-10-09", "20:00", 5))
	requireKind(t, err, core.KindNotFound, "")
	requireKind(t, test.service.DeleteSessionSeries(test.context, test.otherDm, series.ID), core.KindNotFound, "")
	for name, call := range map[string]func() error{
		"get": func() error { _, err := test.service.GetSession(test.context, test.otherDm, *played.ID); return err },
		"end": func() error { _, err := test.service.EndSession(test.context, test.otherDm, *played.ID); return err },
		"recap": func() error {
			_, err := test.service.SetSessionRecap(test.context, test.otherDm, *played.ID, core.SessionRecapInput{Recap: "x"})
			return err
		},
		"title": func() error {
			_, err := test.service.SetSessionTitle(test.context, test.otherDm, *played.ID, core.SessionTitleInput{Title: "x"})
			return err
		},
		"days": func() error {
			_, err := test.service.SetSessionDays(test.context, test.otherDm, *played.ID, core.SessionDaysInput{})
			return err
		},
	} {
		if err := call(); !isKind(err, core.KindNotFound) {
			t.Errorf("%s with another campaign's session = %v, want not found", name, err)
		}
	}
	otherNpc, err := test.service.CreateNpc(test.context, test.otherDm, core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Pell"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = test.service.SetNpcFirstSession(test.context, test.otherDm, otherNpc.ID, core.NpcFirstSessionInput{SessionID: played.ID})
	requireKind(t, err, core.KindInvalid, "sessionId")
	if others, err := test.service.ListSessionSeries(test.context, test.otherDm); err != nil || len(others) != 0 {
		t.Fatalf("the other campaign lists %+v, %v", others, err)
	}
}

func TestNpcFirstEncounteredSession(t *testing.T) {
	test := newSessionTest(t)
	series := test.series(test.dm, weekly("Friday game", "2026-10-09", "20:00", 5))
	played, err := test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-09"})
	if err != nil {
		t.Fatal(err)
	}
	npc, err := test.service.CreateNpc(test.context, test.dm, core.NewNpcInput{Revealed: true, NpcInput: core.NpcInput{Name: "Brannoc"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.SetNpcFirstSession(test.context, test.player, npc.ID, core.NpcFirstSessionInput{SessionID: played.ID}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player set a first session: %v", err)
	}
	if _, err := test.service.SetNpcFirstSession(test.context, test.dm, npc.ID, core.NpcFirstSessionInput{SessionID: played.ID}); err != nil {
		t.Fatal(err)
	}
	seen, err := test.service.GetNpc(test.context, test.player, npc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if seen.FirstSession == nil || seen.FirstSession.Title != "Friday game" || seen.FirstSession.LocalDate != "2026-10-09" || seen.FirstSession.ID != *played.ID {
		t.Fatalf("player's first session = %+v", seen.FirstSession)
	}
	cancelled, err := test.service.CancelSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = test.service.SetNpcFirstSession(test.context, test.dm, npc.ID, core.NpcFirstSessionInput{SessionID: cancelled.ID})
	requireKind(t, err, core.KindInvalid, "sessionId")
	scheduled, err := test.service.RestoreSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = test.service.SetNpcFirstSession(test.context, test.dm, npc.ID, core.NpcFirstSessionInput{SessionID: scheduled.ID})
	requireKind(t, err, core.KindInvalid, "sessionId")
	cleared, err := test.service.SetNpcFirstSession(test.context, test.dm, npc.ID, core.NpcFirstSessionInput{})
	if err != nil || cleared.FirstSession != nil {
		t.Fatalf("cleared = %+v, %v", cleared.FirstSession, err)
	}
	_, err = test.service.SetNpcFirstSession(test.context, test.dm, uuid.New(), core.NpcFirstSessionInput{})
	requireKind(t, err, core.KindNotFound, "")
}

func TestCalendarFeed(t *testing.T) {
	test := newSessionTest(t)
	input := weekly("Friday game; the Ridge, part 2", "2026-10-16", "20:00", 5)
	series := test.series(test.dm, input)
	if _, err := test.service.CancelSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-23"}); err != nil {
		t.Fatal(err)
	}
	if _, err := test.service.UpdateSettings(test.context, test.dm, core.SettingsInput{CampaignName: "Sample", ColonyName: "Sample", Timezone: "America/New_York",
		DiscordEventLocation: "Forge, then Discord Voice — " + strings.Repeat("long ", 12)}); err != nil {
		t.Fatal(err)
	}
	link, err := test.service.GetCalendarLink(test.context, test.player)
	if err != nil || !strings.HasPrefix(link.Path, "/calendar/") || !strings.HasSuffix(link.Path, ".ics") {
		t.Fatalf("link = %+v, %v", link, err)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(link.Path, "/calendar/"), ".ics")
	feed, err := test.service.CalendarFeed(test.context, token)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(feed, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n") || !strings.HasSuffix(feed, "END:VCALENDAR\r\n") {
		t.Fatalf("feed frame:\n%s", feed)
	}
	for _, line := range strings.Split(strings.TrimSuffix(feed, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("line of %d octets: %q", len(line), line)
		}
		if strings.Contains(line, "\n") {
			t.Errorf("bare line feed in %q", line)
		}
	}
	unfolded := strings.ReplaceAll(feed, "\r\n ", "")
	for _, want := range []string{
		"DTSTART:20261017T000000Z\r\nDTEND:20261017T040000Z\r\n",
		"DTSTART:20261107T010000Z\r\n",
		"UID:" + series.ID.String() + "-20261016@colony-planner\r\n",
		`SUMMARY:Friday game\; the Ridge\, part 2` + "\r\n",
		`LOCATION:Forge\, then Discord Voice — long`,
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("feed lacks %q", want)
		}
	}
	if strings.Contains(unfolded, "20261023") {
		t.Error("the cancelled date is in the feed")
	}
	// Played dates stay, a deleted series' ones too, under a UID their row keeps.
	if _, err := test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}); err != nil {
		t.Fatal(err)
	}
	tideMill := test.series(test.dm, core.SessionSeriesInput{Title: "The Tide Mill", FirstDate: "2026-10-14", StartLocal: "19:00", DurationMinutes: 180})
	tideMillPlayed, err := test.service.MarkSessionPlayed(test.context, test.dm, core.SessionDateInput{SeriesID: tideMill.ID, LocalDate: "2026-10-14"})
	if err != nil {
		t.Fatal(err)
	}
	if err := test.service.DeleteSessionSeries(test.context, test.dm, tideMill.ID); err != nil {
		t.Fatal(err)
	}
	if feed, err = test.service.CalendarFeed(test.context, token); err != nil {
		t.Fatal(err)
	}
	unfolded = strings.ReplaceAll(feed, "\r\n ", "")
	for _, want := range []string{
		"UID:" + series.ID.String() + "-20261016@colony-planner\r\n",
		"UID:session-" + tideMillPlayed.ID.String() + "@colony-planner\r\n",
		"DTSTART:20261014T230000Z\r\n",
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("the feed lost a played date: it lacks %q", want)
		}
	}
	// The window is the past 30 days through the next 6 months.
	if strings.Contains(unfolded, "DTSTART:20270417") || !strings.Contains(unfolded, "DTSTART:20270410") {
		t.Error("the feed's window isn't the next 6 months")
	}

	regenerated, err := test.service.RegenerateCalendarToken(test.context, test.dm)
	if err != nil || regenerated.Path == link.Path {
		t.Fatalf("regenerate = %+v, %v", regenerated, err)
	}
	_, err = test.service.CalendarFeed(test.context, token)
	requireKind(t, err, core.KindNotFound, "")
	_, err = test.service.RegenerateCalendarToken(test.context, test.player)
	requireKind(t, err, core.KindForbidden, "")
	_, err = test.service.CalendarFeed(test.context, "")
	requireKind(t, err, core.KindNotFound, "")
	if _, err := test.service.SuspendCampaign(test.context, test.owner, test.campaignID); err != nil {
		t.Fatal(err)
	}
	_, err = test.service.CalendarFeed(test.context, strings.TrimSuffix(strings.TrimPrefix(regenerated.Path, "/calendar/"), ".ics"))
	requireKind(t, err, core.KindNotFound, "")
}

func (test *sessionTest) link(dm core.Viewer, guildID string) {
	test.t.Helper()
	code, err := test.service.CreateLinkCode(test.context, dm)
	if err != nil {
		test.t.Fatal(err)
	}
	if _, err := test.service.LinkGuild(test.context, core.LinkGuildInput{Code: code.Code, GuildID: guildID, GuildName: "Table", DiscordUserID: discordID(1)}); err != nil {
		test.t.Fatal(err)
	}
}

func (test *sessionTest) plan() []core.CampaignSessionPlan {
	test.t.Helper()
	plans, err := test.service.PlanSessions(test.context)
	if err != nil {
		test.t.Fatal(err)
	}
	return plans
}

func jobOf(t *testing.T, plans []core.CampaignSessionPlan, campaignID uuid.UUID, kind core.SessionJobKind) core.SessionJob {
	t.Helper()
	for _, plan := range plans {
		if plan.CampaignID != campaignID {
			continue
		}
		for _, job := range plan.Jobs {
			if job.Kind == kind {
				return job
			}
		}
	}
	t.Fatalf("no %s job for %s in %+v", kind, campaignID, plans)
	return core.SessionJob{}
}

// claimTwice runs two planners' claims at once and returns how many won.
func claimTwice(claim func() (*core.ClaimedSession, error)) (int, []*core.ClaimedSession, error) {
	var wait sync.WaitGroup
	results := make([]*core.ClaimedSession, 2)
	errs := make([]error, 2)
	for index := range 2 {
		wait.Go(func() { results[index], errs[index] = claim() })
	}
	wait.Wait()
	won := 0
	for index := range 2 {
		if errs[index] != nil {
			return 0, nil, errs[index]
		}
		if results[index] != nil {
			won++
		}
	}
	return won, results, nil
}

func TestPlannerClaimsLeasesAndEventStatus(t *testing.T) {
	test := newSessionTest(t)
	// The fixture's clock is Sunday 11 October 2026, 12:00 UTC: a one-off 23 hours later is due a reminder now.
	oneOff := test.series(test.dm, core.SessionSeriesInput{Title: "Finale", FirstDate: "2026-10-12", StartLocal: "07:00", DurationMinutes: 120, Announce: true, Remind: true})
	test.series(test.otherDm, weekly("Ridge nights", "2026-10-16", "20:00", 5))
	if plans := test.plan(); len(plans) != 0 {
		t.Fatalf("unlinked campaigns were planned: %+v", plans)
	}
	test.link(test.dm, "300000000000000001")
	plans := test.plan()
	if len(plans) != 1 || plans[0].CampaignID != test.campaignID || plans[0].GuildID != "300000000000000001" || plans[0].EventLocation != core.DefaultDiscordEventLocation {
		t.Fatalf("plans = %+v", plans)
	}

	remind := jobOf(t, plans, test.campaignID, core.SessionJobRemind)
	won, results, err := claimTwice(func() (*core.ClaimedSession, error) { return test.service.ClaimSessionReminder(test.context, remind) })
	if err != nil || won != 1 {
		t.Fatalf("two planners claimed the reminder %d times: %v", won, err)
	}
	var reminded *core.ClaimedSession
	for _, result := range results {
		if result != nil {
			reminded = result
		}
	}
	if reminded.Title != "Finale" || reminded.StartsAt.Format(time.RFC3339) != "2026-10-12T11:00:00Z" {
		t.Fatalf("claimed reminder = %+v", reminded)
	}
	if again, err := test.service.ClaimSessionReminder(test.context, remind); err != nil || again != nil {
		t.Fatalf("a restart claimed the reminder again: %+v, %v", again, err)
	}
	if err := test.service.ReleaseSessionReminder(test.context, test.campaignID, reminded.SessionID); err != nil {
		t.Fatal(err)
	}
	if retried, err := test.service.ClaimSessionReminder(test.context, remind); err != nil || retried == nil {
		t.Fatalf("a released reminder can't be claimed: %+v, %v", retried, err)
	}

	create := jobOf(t, test.plan(), test.campaignID, core.SessionJobCreateEvent)
	won, results, err = claimTwice(func() (*core.ClaimedSession, error) {
		return test.service.ClaimSessionDiscordEvent(test.context, create)
	})
	if err != nil || won != 1 {
		t.Fatalf("two planners took the event lease %d times: %v", won, err)
	}
	var leased *core.ClaimedSession
	for _, result := range results {
		if result != nil {
			leased = result
		}
	}
	for _, plan := range test.plan() {
		for _, job := range plan.Jobs {
			if job.Kind == core.SessionJobCreateEvent {
				t.Fatalf("a leased date is due again: %+v", job)
			}
		}
	}
	test.clock.advance(4 * time.Minute)
	if held, err := test.service.ClaimSessionDiscordEvent(test.context, create); err != nil || held != nil {
		t.Fatalf("a live lease was taken: %+v, %v", held, err)
	}
	// Exactly when the plan calls the lease over, the claim must agree, or the planner backs off for nothing.
	test.clock.advance(time.Minute)
	jobOf(t, test.plan(), test.campaignID, core.SessionJobCreateEvent)
	expired, err := test.service.ClaimSessionDiscordEvent(test.context, create)
	if err != nil || expired == nil {
		t.Fatalf("an expired lease couldn't be taken: %+v, %v", expired, err)
	}
	if err := test.service.ReleaseSessionDiscordClaim(test.context, test.campaignID, expired.SessionID); err != nil {
		t.Fatal(err)
	}
	if leased, err = test.service.ClaimSessionDiscordEvent(test.context, create); err != nil || leased == nil {
		t.Fatalf("a released lease couldn't be taken: %+v, %v", leased, err)
	}
	eventID := "400000000000000001"
	if _, err := test.service.RecordSessionDiscordEvent(test.context, test.campaignID, leased.SessionID, core.DiscordEventRecord{
		GuildID: "300000000000000001", DiscordEventID: eventID, StartsAt: leased.StartsAt, Title: leased.Title, DurationMinutes: leased.DurationMinutes, Location: core.DefaultDiscordEventLocation,
	}); err != nil {
		t.Fatal(err)
	}
	if held, err := test.service.ClaimSessionDiscordEvent(test.context, create); err != nil || held != nil {
		t.Fatalf("a date with an event was claimed again: %+v, %v", held, err)
	}
	for _, plan := range test.plan() {
		if len(plan.Jobs) != 0 {
			t.Fatalf("jobs after the event and reminder: %+v", plan.Jobs)
		}
	}

	// Discord's ACTIVE and COMPLETED drive the stamps, only from the linked server and only for this event.
	if err := test.service.RecordDiscordEventStatus(test.context, "300000000000000009", eventID, core.DiscordEventActive); err != nil {
		t.Fatal(err)
	}
	if err := test.service.RecordDiscordEventStatus(test.context, "300000000000000001", "400000000000000009", core.DiscordEventActive); err != nil {
		t.Fatal(err)
	}
	if session, _ := test.service.GetSession(test.context, test.dm, leased.SessionID); session.Status != core.SessionStatusScheduled {
		t.Fatalf("an unknown event or server changed the session: %+v", session)
	}
	before := test.planner.changesFor(test.campaignID)
	if err := test.service.RecordDiscordEventStatus(test.context, "300000000000000001", eventID, core.DiscordEventActive); err != nil {
		t.Fatal(err)
	}
	if err := test.service.RecordDiscordEventStatus(test.context, "300000000000000001", eventID, core.DiscordEventCompleted); err != nil {
		t.Fatal(err)
	}
	session, err := test.service.GetSession(test.context, test.player, leased.SessionID)
	if err != nil || session.Status != core.SessionStatusPlayed || session.ColonyDayStart == nil || session.ColonyDayEnd == nil {
		t.Fatalf("after ACTIVE and COMPLETED = %+v, %v", session, err)
	}
	if test.planner.changesFor(test.campaignID) <= before {
		t.Fatal("the event's status sent no re-plan signal")
	}
	// A removed event is forgotten; a deleted series' date that held nothing else goes with it.
	if err := test.service.DeleteSessionSeries(test.context, test.dm, oneOff.ID); err != nil {
		t.Fatal(err)
	}
	if err := test.service.ForgetSessionDiscordEvent(test.context, test.campaignID, leased.SessionID, eventID); err != nil {
		t.Fatal(err)
	}
	if kept, err := test.service.GetSession(test.context, test.player, leased.SessionID); err != nil || kept.Status != core.SessionStatusPlayed {
		t.Fatalf("a played date was deleted with its event: %+v, %v", kept, err)
	}
}

// Unlinking forgets the campaign's events, hands Beny Bot the upcoming ones to delete in the old server,
// and refuses a claim the plan made before the unlink; a relinked server gets its own events.
func TestUnlinkingForgetsTheCampaignsEvents(t *testing.T) {
	test := newSessionTest(t)
	test.link(test.dm, "300000000000000001")
	test.series(test.dm, weekly("Friday game", "2026-10-16", "20:00", 5))
	create := jobOf(t, test.plan(), test.campaignID, core.SessionJobCreateEvent)
	claimed, err := test.service.ClaimSessionDiscordEvent(test.context, create)
	if err != nil || claimed == nil {
		t.Fatalf("claim = %+v, %v", claimed, err)
	}
	if _, err := test.service.RecordSessionDiscordEvent(test.context, test.campaignID, claimed.SessionID, core.DiscordEventRecord{
		GuildID: "300000000000000001", DiscordEventID: "400000000000000001", StartsAt: claimed.StartsAt, Title: claimed.Title,
		DurationMinutes: claimed.DurationMinutes, Location: core.DefaultDiscordEventLocation,
	}); err != nil {
		t.Fatal(err)
	}
	if err := test.service.UnlinkGuild(test.context, test.dm); err != nil {
		t.Fatal(err)
	}
	test.planner.mutex.Lock()
	abandoned := test.planner.abandoned
	test.planner.mutex.Unlock()
	if len(abandoned) != 1 || abandoned[0].GuildID != "300000000000000001" || len(abandoned[0].DiscordEventIDs) != 1 ||
		abandoned[0].DiscordEventIDs[0] != "400000000000000001" || abandoned[0].CampaignID != test.campaignID {
		t.Fatalf("abandoned = %+v, want the Friday's event in the old server", abandoned)
	}
	// A plan read before the unlink can't claim or record an event in the old server any more.
	if held, err := test.service.ClaimSessionDiscordEvent(test.context, create); err != nil || held != nil {
		t.Fatalf("a claim for an unlinked server = %+v, %v", held, err)
	}
	_, err = test.service.RecordSessionDiscordEvent(test.context, test.campaignID, claimed.SessionID, core.DiscordEventRecord{
		GuildID: "300000000000000001", DiscordEventID: "400000000000000002", StartsAt: claimed.StartsAt, Title: claimed.Title,
		DurationMinutes: claimed.DurationMinutes, Location: core.DefaultDiscordEventLocation,
	})
	requireKind(t, err, core.KindNotFound, "")

	test.link(test.dm, "300000000000000002")
	relinked := jobOf(t, test.plan(), test.campaignID, core.SessionJobCreateEvent)
	if relinked.LocalDate != "2026-10-16" || relinked.GuildID != "300000000000000002" {
		t.Fatalf("after relinking = %+v, want a new event for the Friday in the new server", relinked)
	}
}

func TestSessionBandsTakeTheTimelinesRange(t *testing.T) {
	test := newSessionTest(t)
	_, err := test.service.ListSessionBands(test.context, test.player, core.TimelineRangeInput{FromDay: -1, ToDay: 5})
	requireKind(t, err, core.KindInvalid, "fromDay")
	_, err = test.service.ListSessionBands(test.context, test.player, core.TimelineRangeInput{FromDay: 0, ToDay: 2_000_000_000})
	requireKind(t, err, core.KindInvalid, "toDay")
}

func TestPlannerRemovesEventsOfCancelledDatesAndDeletedSeries(t *testing.T) {
	test := newSessionTest(t)
	test.link(test.dm, "300000000000000001")
	series := test.series(test.dm, weekly("Friday game", "2026-10-16", "20:00", 5))
	withEvent := func(eventID string) core.ClaimedSession {
		t.Helper()
		create := jobOf(t, test.plan(), test.campaignID, core.SessionJobCreateEvent)
		claimed, err := test.service.ClaimSessionDiscordEvent(test.context, create)
		if err != nil || claimed == nil {
			t.Fatalf("claim = %+v, %v", claimed, err)
		}
		if _, err := test.service.RecordSessionDiscordEvent(test.context, test.campaignID, claimed.SessionID, core.DiscordEventRecord{
			GuildID: "300000000000000001", DiscordEventID: eventID, StartsAt: claimed.StartsAt, Title: claimed.Title, DurationMinutes: claimed.DurationMinutes, Location: core.DefaultDiscordEventLocation,
		}); err != nil {
			t.Fatal(err)
		}
		return *claimed
	}
	first := withEvent("400000000000000001")
	if _, err := test.service.CancelSessionDate(test.context, test.dm, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}); err != nil {
		t.Fatal(err)
	}
	plans := test.plan()
	remove := jobOf(t, plans, test.campaignID, core.SessionJobRemoveEvent)
	if remove.SessionID == nil || *remove.SessionID != first.SessionID || remove.DiscordEventID == nil || *remove.DiscordEventID != "400000000000000001" {
		t.Fatalf("remove job = %+v", remove)
	}
	if create := jobOf(t, plans, test.campaignID, core.SessionJobCreateEvent); create.LocalDate != "2026-10-23" {
		t.Fatalf("the event moved to %s, want the following Friday", create.LocalDate)
	}
	if err := test.service.ForgetSessionDiscordEvent(test.context, test.campaignID, first.SessionID, "400000000000000001"); err != nil {
		t.Fatal(err)
	}
	if cancelled, err := test.service.GetSession(test.context, test.dm, first.SessionID); err != nil || cancelled.Status != core.SessionStatusCancelled {
		t.Fatalf("the cancelled date after its event went = %+v, %v", cancelled, err)
	}

	// A new start time leaves the next date's event behind: the planner asks for an update.
	second := withEvent("400000000000000002")
	if _, err := test.service.UpdateSessionSeries(test.context, test.dm, series.ID, weekly("Friday game", "2026-10-16", "19:00", 5)); err != nil {
		t.Fatal(err)
	}
	update := jobOf(t, test.plan(), test.campaignID, core.SessionJobUpdateEvent)
	if *update.SessionID != second.SessionID || update.StartsAt.Format(time.RFC3339) != "2026-10-23T23:00:00Z" {
		t.Fatalf("update job = %+v", update)
	}

	// Deleting the series keeps the date with an event until the planner removes it, then it goes.
	if err := test.service.DeleteSessionSeries(test.context, test.dm, series.ID); err != nil {
		t.Fatal(err)
	}
	orphan := jobOf(t, test.plan(), test.campaignID, core.SessionJobRemoveEvent)
	if *orphan.SessionID != second.SessionID {
		t.Fatalf("orphan remove job = %+v", orphan)
	}
	if err := test.service.ForgetSessionDiscordEvent(test.context, test.campaignID, second.SessionID, "400000000000000002"); err != nil {
		t.Fatal(err)
	}
	_, err := test.service.GetSession(test.context, test.dm, second.SessionID)
	requireKind(t, err, core.KindNotFound, "")
	_, err = test.service.GetSession(test.context, test.dm, first.SessionID)
	requireKind(t, err, core.KindNotFound, "")
}
