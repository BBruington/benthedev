package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

const (
	tableTimezone      = "America/New_York"
	tableEventLocation = "Table 4"
	fridaysTitle       = "Port Eldari Fridays"
	saturdaysTitle     = "Ridgehold Saturdays"
)

// The world's clock starts on Saturday 2026-10-10 at 20:00 UTC, 4 PM in New York. Friday 2026-10-16
// at 8 PM New York is 2026-10-17 00:00 UTC.
var firstFridayStart = time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC)

// tableReady links the server to the DM's campaign and picks, as the DM would on Settings, New York
// time, the announce channel, the reminder channel (nil for none) and the event location.
func (test *world) tableReady(guildID string, dm core.Viewer, announceChannelID string, reminderChannelID *string) {
	test.t.Helper()
	test.link(guildID, dm)
	settings, err := test.service.GetSettings(test.context, dm)
	if err != nil {
		test.t.Fatalf("settings: %v", err)
	}
	if _, err := test.service.UpdateSettings(test.context, dm, core.SettingsInput{
		CampaignName:             settings.CampaignName,
		ColonyName:               settings.ColonyName,
		Timezone:                 tableTimezone,
		DiscordAnnounceChannelID: &announceChannelID,
		DiscordReminderChannelID: reminderChannelID,
		DiscordEventLocation:     tableEventLocation,
	}); err != nil {
		test.t.Fatalf("settings: %v", err)
	}
}

// weekly is a weekly 8 PM, four-hour series on one weekday that Beny Bot announces.
func weekly(title string, weekday int64, firstDate string) core.SessionSeriesInput {
	return core.SessionSeriesInput{
		Title: title, Weekdays: []int64{weekday}, EveryWeeks: new(int64(1)), FirstDate: firstDate,
		StartLocal: "20:00", DurationMinutes: 240, Announce: true,
	}
}

// oneOff is a three-hour one-off at a New York date and time.
func oneOff(title string, firstDate string, startLocal string, announce bool, remind bool) core.SessionSeriesInput {
	return core.SessionSeriesInput{
		Title: title, Weekdays: []int64{}, FirstDate: firstDate, StartLocal: startLocal,
		DurationMinutes: 180, Announce: announce, Remind: remind,
	}
}

func (test *world) createSeries(dm core.Viewer, input core.SessionSeriesInput) core.SessionSeries {
	test.t.Helper()
	series, err := test.service.CreateSessionSeries(test.context, dm, input)
	if err != nil {
		test.t.Fatalf("create series: %v", err)
	}
	return series
}

func (test *world) plan() *time.Time {
	test.t.Helper()
	return test.bot.planSessions(test.context)
}

// advanceTo moves the fake clock to instant.
func (test *world) advanceTo(instant time.Time) {
	test.clock.advance(instant.Sub(test.clock.now()))
}

// secondPlanner is another process on the same token and database, as during a deploy overlap: it
// sees the same Discord and the same core.
func (test *world) secondPlanner() *Bot {
	test.t.Helper()
	other := newBot(Options{Token: "test-token", PublicURL: testPublicURL, Logger: quietLogger()}, test.client, test.state, test.clock.now)
	if err := other.start(test.service); err != nil {
		test.t.Fatalf("start: %v", err)
	}
	other.botUserID = benyBotUserID
	test.t.Cleanup(other.stop)
	return other
}

func contents(messages []sentMessage) []string {
	texts := make([]string, 0, len(messages))
	for _, message := range messages {
		texts = append(texts, message.message.Content)
	}
	return texts
}

func announcement(title string, startsAt time.Time) string {
	return fmt.Sprintf("**%s** is scheduled for <t:%d:F> (<t:%d:R>)", title, startsAt.Unix(), startsAt.Unix())
}

func (test *world) requireOneEventAt(guildID string, title string, startsAt time.Time) discordgo.GuildScheduledEvent {
	test.t.Helper()
	events := test.client.eventsIn(guildID)
	if len(events) != 1 {
		test.t.Fatalf("server %s has %d events, want 1: %+v", guildID, len(events), events)
	}
	event := events[0]
	if event.Name != title || !event.ScheduledStartTime.Equal(startsAt) {
		test.t.Fatalf("event is %q at %s, want %q at %s", event.Name, event.ScheduledStartTime, title, startsAt)
	}
	return event
}

func TestThePlannerKeepsOneExternalEventOnTheNextDate(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))

	wake := test.plan()
	event := test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)
	end := firstFridayStart.Add(4 * time.Hour)
	if event.EntityType != discordgo.GuildScheduledEventEntityTypeExternal || event.EntityMetadata.Location != tableEventLocation ||
		event.ScheduledEndTime == nil || !event.ScheduledEndTime.Equal(end) || event.PrivacyLevel != discordgo.GuildScheduledEventPrivacyLevelGuildOnly {
		t.Errorf("event = %+v, want an external event at %q ending %s", event, tableEventLocation, end)
	}
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 || got[0] != announcement(fridaysTitle, firstFridayStart) {
		t.Errorf("announce channel got %q", got)
	}
	if wake == nil || !wake.Equal(end) {
		t.Errorf("next wake = %v, want the game's end %s", wake, end)
	}

	test.plan()
	if test.client.eventCreates != 1 || len(test.client.messagesTo(channelXGeneral)) != 1 {
		t.Errorf("planning again made %d events and %d announcements, want 1 and 1", test.client.eventCreates, len(test.client.messagesTo(channelXGeneral)))
	}
}

func TestTheEventRollsToTheNextFridayAcrossTheClockChange(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-30"))
	lastSummerFriday := time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC)
	test.plan()
	test.requireOneEventAt(guildX, fridaysTitle, lastSummerFriday)

	test.advanceTo(lastSummerFriday.Add(4*time.Hour + time.Minute))
	test.plan()
	// 8 PM New York on the first Friday after the November change is 01:00 UTC.
	firstWinterFriday := time.Date(2026, time.November, 7, 1, 0, 0, 0, time.UTC)
	events := test.client.eventsIn(guildX)
	if len(events) != 2 || !events[1].ScheduledStartTime.Equal(firstWinterFriday) {
		t.Fatalf("events = %+v, want the ended one and one at %s", events, firstWinterFriday)
	}
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 2 || got[1] != announcement(fridaysTitle, firstWinterFriday) {
		t.Errorf("announce channel got %q", got)
	}
}

func TestCancellingTheNextDateMovesTheEventAndRestoringBringsItBack(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	series := test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.plan()
	test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)

	date := core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}
	if _, err := test.service.CancelSessionDate(test.context, test.dmA, date); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	test.bot.posts.Wait()
	expectedNotice := fmt.Sprintf("❌ **%s** on <t:%d:F> has been cancelled.", fridaysTitle, firstFridayStart.Unix())
	if got := contents(test.client.messagesTo(channelXDesk)); len(got) != 1 || got[0] != expectedNotice {
		t.Errorf("reminder channel got %q, want the cancellation notice", got)
	}
	test.plan()
	secondFridayStart := firstFridayStart.AddDate(0, 0, 7)
	test.requireOneEventAt(guildX, fridaysTitle, secondFridayStart)
	if test.client.eventDeletes != 1 {
		t.Errorf("deleted %d events, want the cancelled date's", test.client.eventDeletes)
	}

	if _, err := test.service.RestoreSessionDate(test.context, test.dmA, date); err != nil {
		t.Fatalf("restore: %v", err)
	}
	test.plan()
	test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 3 || got[2] != announcement(fridaysTitle, firstFridayStart) {
		t.Errorf("announce channel got %q", got)
	}
}

func TestEditingASeriesUpdatesItsEvent(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	input := weekly(fridaysTitle, 5, "2026-10-16")
	series := test.createSeries(test.dmA, input)
	test.plan()
	original := test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)

	input.Title = "Port Eldari Nights"
	input.StartLocal = "19:00"
	if _, err := test.service.UpdateSessionSeries(test.context, test.dmA, series.ID, input); err != nil {
		t.Fatalf("update: %v", err)
	}
	test.plan()
	updated := test.requireOneEventAt(guildX, "Port Eldari Nights", firstFridayStart.Add(-time.Hour))
	if updated.ID != original.ID || test.client.eventEdits != 1 || test.client.eventCreates != 1 {
		t.Errorf("the edit made event %s (%d edits, %d creates), want %s edited in place", updated.ID, test.client.eventEdits, test.client.eventCreates, original.ID)
	}
	test.plan()
	if test.client.eventEdits != 1 {
		t.Errorf("planning again edited %d times, want 1", test.client.eventEdits)
	}

	// Moving the game to Saturdays moves the same event (spec §7.4), so its "interested" list stays.
	input.Weekdays = []int64{6}
	if _, err := test.service.UpdateSessionSeries(test.context, test.dmA, series.ID, input); err != nil {
		t.Fatalf("update: %v", err)
	}
	test.plan()
	// Saturday 2026-10-17 at 7 PM New York.
	saturdayStart := time.Date(2026, time.October, 17, 23, 0, 0, 0, time.UTC)
	moved := test.requireOneEventAt(guildX, "Port Eldari Nights", saturdayStart)
	if moved.ID != original.ID || test.client.eventCreates != 1 || test.client.eventDeletes != 0 {
		t.Errorf("the weekday change made event %s (%d creates, %d deletes), want %s moved", moved.ID, test.client.eventCreates, test.client.eventDeletes, original.ID)
	}
	edits := test.client.eventEdits
	test.plan()
	if test.client.eventEdits != edits || test.client.eventCreates != 1 {
		t.Errorf("planning again after the move edited or made events (%d edits, %d creates)", test.client.eventEdits, test.client.eventCreates)
	}
	sessions, err := test.service.ListUpcomingSessions(test.context, test.dmA)
	if err != nil || len(sessions) == 0 || sessions[0].LocalDate != "2026-10-17" || sessions[0].ID == nil {
		t.Fatalf("upcoming = %+v, %v, want Saturday first with the event's row", sessions, err)
	}
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 {
		t.Errorf("announce channel got %q, want only the first announcement", got)
	}
}

func TestANewEventLocationReachesTheExistingEvent(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.plan()
	original := test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)

	settings, err := test.service.GetSettings(test.context, test.dmA)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if _, err := test.service.UpdateSettings(test.context, test.dmA, core.SettingsInput{
		CampaignName: settings.CampaignName, ColonyName: settings.ColonyName, Timezone: settings.Timezone,
		DiscordAnnounceChannelID: settings.DiscordAnnounceChannelID, DiscordEventLocation: "Foundry, table 2",
	}); err != nil {
		t.Fatalf("settings: %v", err)
	}
	test.plan()
	updated := test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)
	if updated.ID != original.ID || updated.EntityMetadata.Location != "Foundry, table 2" || test.client.eventEdits != 1 {
		t.Errorf("event = %s at %q (%d edits), want %s edited to the new location", updated.ID, updated.EntityMetadata.Location, test.client.eventEdits, original.ID)
	}
	test.plan()
	if test.client.eventEdits != 1 {
		t.Errorf("planning again edited %d times, want 1", test.client.eventEdits)
	}
}

// The DM cancels tonight's game, the group plays after all, and the DM restores the date mid-game: the
// next Friday keeps its event and is announced once.
func TestRestoringADateMidGameKeepsTheNextFridaysEvent(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	series := test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.plan()
	date := core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}
	if _, err := test.service.CancelSessionDate(test.context, test.dmA, date); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	test.plan()
	secondFridayStart := firstFridayStart.AddDate(0, 0, 7)
	next := test.requireOneEventAt(guildX, fridaysTitle, secondFridayStart)

	test.advanceTo(firstFridayStart.Add(30 * time.Minute))
	if _, err := test.service.RestoreSessionDate(test.context, test.dmA, date); err != nil {
		t.Fatalf("restore: %v", err)
	}
	test.plan()
	if kept := test.requireOneEventAt(guildX, fridaysTitle, secondFridayStart); kept.ID != next.ID {
		t.Fatalf("the next Friday's event became %s, want %s kept", kept.ID, next.ID)
	}
	test.advanceTo(firstFridayStart.Add(4*time.Hour + time.Minute))
	test.plan()
	if kept := test.requireOneEventAt(guildX, fridaysTitle, secondFridayStart); kept.ID != next.ID || test.client.eventCreates != 2 {
		t.Fatalf("after the restored game ended: event %s (%d creates), want %s kept", kept.ID, test.client.eventCreates, next.ID)
	}
	announcements := 0
	for _, message := range contents(test.client.messagesTo(channelXGeneral)) {
		if message == announcement(fridaysTitle, secondFridayStart) {
			announcements++
		}
	}
	if announcements != 1 {
		t.Errorf("the next Friday was announced %d times, want once", announcements)
	}
}

// A one-off with the same title and start as the weekly game gets its own event; it doesn't adopt the
// weekly game's.
func TestTwoDatesWithTheSameTitleAndStartGetTwoEvents(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.plan()
	test.createSeries(test.dmA, oneOff(fridaysTitle, "2026-10-16", "20:00", true, false))
	if wake := test.plan(); wake == nil || wake.Before(firstFridayStart) {
		t.Errorf("next wake = %v, want no retry before the game", wake)
	}
	events := test.client.eventsIn(guildX)
	if len(events) != 2 || events[0].ID == events[1].ID || test.client.eventCreates != 2 {
		t.Fatalf("events = %+v (%d creates), want one for each date", events, test.client.eventCreates)
	}
}

func TestDeletingASeriesDeletesItsEvent(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	series := test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.plan()
	test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)

	if err := test.service.DeleteSessionSeries(test.context, test.dmA, series.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	test.plan()
	if events := test.client.eventsIn(guildX); len(events) != 0 || test.client.eventDeletes != 1 {
		t.Errorf("events after the delete = %+v (%d deletes), want none", events, test.client.eventDeletes)
	}
	if wake := test.plan(); wake != nil {
		t.Errorf("next wake = %s, want none with no series", wake)
	}
}

func TestDeletingTheSeriesWhileItsEventIsMadeLeavesNoEventBehind(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	series := test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.client.mutex.Lock()
	test.client.duringEventCreate = func() {
		test.client.duringEventCreate = nil
		if err := test.service.DeleteSessionSeries(test.context, test.dmA, series.ID); err != nil {
			t.Errorf("delete: %v", err)
		}
	}
	test.client.mutex.Unlock()

	test.plan()
	test.clock.advance(6 * time.Minute)
	test.plan()
	test.secondPlanner().planSessions(test.context)
	if events := test.client.eventsIn(guildX); len(events) != 0 {
		t.Errorf("the server still has events nobody will remove: %+v", events)
	}
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 0 {
		t.Errorf("announce channel got %q for a deleted series", got)
	}
}

func TestCancellingTheDateWhileItsEventIsMadeAnnouncesOnlyTheNextDate(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	series := test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.client.mutex.Lock()
	test.client.duringEventCreate = func() {
		test.client.duringEventCreate = nil
		if _, err := test.service.CancelSessionDate(test.context, test.dmA, core.SessionDateInput{SeriesID: series.ID, LocalDate: "2026-10-16"}); err != nil {
			t.Errorf("cancel: %v", err)
		}
	}
	test.client.mutex.Unlock()

	test.plan()
	test.bot.posts.Wait()
	secondFridayStart := firstFridayStart.AddDate(0, 0, 7)
	test.requireOneEventAt(guildX, fridaysTitle, secondFridayStart)
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 || got[0] != announcement(fridaysTitle, secondFridayStart) {
		t.Errorf("announce channel got %q, want only the next Friday's announcement", got)
	}
}

// targetLookupFails is the planner's service while the database can't say where the campaign posts.
type targetLookupFails struct {
	colonyService
}

func (targetLookupFails) DiscordTargetFor(context.Context, uuid.UUID) (core.DiscordTarget, bool, error) {
	return core.DiscordTarget{}, false, errDatabaseDown
}

func TestAReminderWhoseChannelCouldNotBeReadIsRetried(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	test.createSeries(test.dmA, oneOff("Harbor Council", "2026-10-11", "15:00", false, true))
	unreadable := newBot(Options{Token: "test-token", PublicURL: testPublicURL, Logger: quietLogger()}, test.client, test.state, test.clock.now)
	if err := unreadable.start(targetLookupFails{colonyService: test.service}); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(unreadable.stop)

	unreadable.planSessions(test.context)
	if got := contents(test.client.messagesTo(channelXDesk)); len(got) != 0 {
		t.Fatalf("reminder channel got %q while the channel couldn't be read", got)
	}
	test.plan()
	if got := contents(test.client.messagesTo(channelXDesk)); len(got) != 1 {
		t.Errorf("reminder channel got %q, want the reminder the failed lookup gave back", got)
	}
}

// A one-off 23 hours away is inside the reminder's 24 hours, so its reminder is due at once.
func TestAReminderIsSentOnceEvenAcrossARestart(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	// 3 PM New York on Sunday is 23 hours after the clock's start.
	startsAt := time.Date(2026, time.October, 11, 19, 0, 0, 0, time.UTC)
	test.createSeries(test.dmA, oneOff("Harbor Council", "2026-10-11", "15:00", false, true))

	test.plan()
	expected := fmt.Sprintf("Reminder: **Harbor Council** is coming up <t:%d:R> (<t:%d:F>). Don't forget!", startsAt.Unix(), startsAt.Unix())
	if got := contents(test.client.messagesTo(channelXDesk)); len(got) != 1 || got[0] != expected {
		t.Fatalf("reminder channel got %q, want one reminder", got)
	}
	test.plan()
	test.secondPlanner().planSessions(test.context)
	if got := contents(test.client.messagesTo(channelXDesk)); len(got) != 1 {
		t.Errorf("reminder channel got %d reminders after planning again and a restart, want 1", len(got))
	}
	if events := test.client.eventsIn(guildX); len(events) != 0 {
		t.Errorf("a series that doesn't announce got %d events", len(events))
	}
}

func TestAReminderFallsBackToTheAnnounceChannelAndSkipsAStartedGame(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	// 6 PM New York, two hours after the clock's start: its reminder time passed while nothing ran.
	test.createSeries(test.dmA, oneOff("Harbor Council", "2026-10-10", "18:00", false, true))
	test.advanceTo(time.Date(2026, time.October, 10, 22, 30, 0, 0, time.UTC))
	test.plan()
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 0 {
		t.Errorf("a game that already started got reminders %q", got)
	}

	test.createSeries(test.dmA, oneOff("Supply Run", "2026-10-11", "02:00", false, true))
	test.plan()
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 {
		t.Errorf("announce channel got %q, want the missed reminder there, since no reminder channel is set", got)
	}
}

func TestAReminderDiscordRefusedIsRetriedAfterABackOff(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	test.createSeries(test.dmA, oneOff("Harbor Council", "2026-10-11", "15:00", false, true))
	test.client.mutex.Lock()
	test.client.sendError = errors.New("HTTP 503 Service Unavailable")
	test.client.mutex.Unlock()

	wake := test.plan()
	if attempts := len(test.client.messagesTo(channelXDesk)); attempts != 1 {
		t.Fatalf("tried %d times, want 1", attempts)
	}
	if retryAt := test.clock.now().Add(firstRetryDelay); wake == nil || !wake.Equal(retryAt) {
		t.Errorf("next wake = %v, want the retry at %s", wake, retryAt)
	}
	test.plan()
	if attempts := len(test.client.messagesTo(channelXDesk)); attempts != 1 {
		t.Errorf("tried %d times before the back-off ended, want 1", attempts)
	}

	test.client.mutex.Lock()
	test.client.sendError = nil
	test.client.mutex.Unlock()
	test.clock.advance(firstRetryDelay)
	test.plan()
	test.plan()
	if attempts := len(test.client.messagesTo(channelXDesk)); attempts != 2 {
		t.Errorf("tried %d times, want the failure and one reminder", attempts)
	}
}

func TestTwoPlannersSendOneReminderAndMakeOneEvent(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, new(channelXDesk))
	test.createSeries(test.dmA, oneOff("Harbor Council", "2026-10-11", "15:00", true, true))
	other := test.secondPlanner()

	var both sync.WaitGroup
	both.Go(func() { test.bot.planSessions(test.context) })
	both.Go(func() { other.planSessions(test.context) })
	both.Wait()
	test.plan()
	other.planSessions(test.context)

	if events := test.client.eventsIn(guildX); len(events) != 1 || test.client.eventCreates != 1 {
		t.Errorf("made %d events (%d creates), want 1", len(events), test.client.eventCreates)
	}
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 {
		t.Errorf("announce channel got %q, want one announcement", got)
	}
	if got := contents(test.client.messagesTo(channelXDesk)); len(got) != 1 {
		t.Errorf("reminder channel got %q, want one reminder", got)
	}
}

// A planner that crashed after Discord made the event, before it was stored, left its lease and the
// event behind. The next planner waits out the lease and adopts that event instead of making another.
func TestAnEventMadeBeforeACrashIsAdoptedNotDuplicated(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	plans, err := test.service.PlanSessions(test.context)
	if err != nil || len(plans) != 1 || len(plans[0].Jobs) != 1 {
		t.Fatalf("plans = %+v, %v; want one create job", plans, err)
	}
	if claimed, err := test.service.ClaimSessionDiscordEvent(test.context, plans[0].Jobs[0]); err != nil || claimed == nil {
		t.Fatalf("the crashed planner's claim: %v, %v", claimed, err)
	}
	test.client.mutex.Lock()
	someoneElses := test.client.addEventLocked(guildX, someoneElsesID, fridaysTitle, firstFridayStart).ID
	crashed := test.client.addEventLocked(guildX, benyBotUserID, fridaysTitle, firstFridayStart).ID
	test.client.mutex.Unlock()

	leaseEnds := test.clock.now().Add(5 * time.Minute)
	if wake := test.plan(); wake == nil || !wake.Equal(leaseEnds) {
		t.Errorf("next wake = %v, want the lease's end %s", wake, leaseEnds)
	}
	if got := test.client.messagesTo(channelXGeneral); len(got) != 0 {
		t.Fatalf("announced while another planner held the lease: %q", contents(got))
	}

	// One second on: core plans a create at the lease's end, but its claim needs the lease strictly older.
	test.advanceTo(leaseEnds.Add(time.Second))
	test.plan()
	test.plan()
	if events := test.client.eventsIn(guildX); len(events) != 2 || test.client.eventCreates != 0 {
		t.Errorf("events = %d (%d creates), want the two already there and no new one", len(events), test.client.eventCreates)
	}
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 || got[0] != announcement(fridaysTitle, firstFridayStart) {
		t.Errorf("announce channel got %q, want one announcement", got)
	}

	// The adopted event is Beny Bot's, so its status drives the stamps; the look-alike someone else made doesn't.
	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildX, someoneElses, discordgo.GuildScheduledEventStatusActive))
	if played := test.played(test.dmA); len(played) != 0 {
		t.Fatalf("someone else's event marked %d sessions played", len(played))
	}
	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildX, crashed, discordgo.GuildScheduledEventStatusActive))
	if played := test.played(test.dmA); len(played) != 1 {
		t.Errorf("the adopted event marked %d sessions played, want 1", len(played))
	}
}

func statusUpdate(guildID string, eventID string, status discordgo.GuildScheduledEventStatus) *discordgo.GuildScheduledEventUpdate {
	return &discordgo.GuildScheduledEventUpdate{GuildScheduledEvent: &discordgo.GuildScheduledEvent{ID: eventID, GuildID: guildID, Status: status}}
}

func (test *world) played(viewer core.Viewer) []core.Session {
	test.t.Helper()
	sessions, err := test.service.ListPlayedSessions(test.context, viewer)
	if err != nil {
		test.t.Fatalf("played sessions: %v", err)
	}
	return sessions
}

func TestDiscordEventStatusDrivesThePlayedStamps(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.tableReady(guildY, test.dmB, channelYDesk, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.plan()
	eventID := test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart).ID
	board, err := test.service.GetColonyBoard(test.context, test.dmA)
	if err != nil {
		t.Fatal(err)
	}

	// Another server naming this event, an event Beny Bot never stored and a status that isn't a
	// start or an end change nothing.
	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildY, eventID, discordgo.GuildScheduledEventStatusActive))
	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildX, "600000000000009999", discordgo.GuildScheduledEventStatusActive))
	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildX, eventID, discordgo.GuildScheduledEventStatusCanceled))
	if playedA, playedB := test.played(test.dmA), test.played(test.dmB); len(playedA) != 0 || len(playedB) != 0 {
		t.Fatalf("played sessions = %d in A and %d in B, want none", len(playedA), len(playedB))
	}

	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildX, eventID, discordgo.GuildScheduledEventStatusActive))
	played := test.played(test.dmA)
	if len(played) != 1 || played[0].ColonyDayStart == nil || *played[0].ColonyDayStart != board.Day || played[0].ColonyDayEnd != nil {
		t.Fatalf("after ACTIVE: %+v, want one session played on day %d and not ended", played, board.Day)
	}
	test.bot.handleScheduledEventUpdate(test.context, statusUpdate(guildX, eventID, discordgo.GuildScheduledEventStatusCompleted))
	played = test.played(test.dmA)
	if len(played) != 1 || played[0].ColonyDayEnd == nil || *played[0].ColonyDayEnd != board.Day {
		t.Errorf("after COMPLETED: %+v, want the end stamped on day %d", played, board.Day)
	}
	if playedB := test.played(test.dmB); len(playedB) != 0 {
		t.Errorf("campaign B got %d played sessions", len(playedB))
	}
}

func TestEachCampaignsEventsStayInItsServerAndStopWhenUnlinked(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.tableReady(guildY, test.dmB, channelYDesk, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	saturdays := weekly(saturdaysTitle, 6, "2026-10-17")
	saturdays.EveryWeeks = new(int64(2))
	test.createSeries(test.dmB, saturdays)
	firstSaturdayStart := time.Date(2026, time.October, 18, 0, 0, 0, 0, time.UTC)

	test.plan()
	test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)
	test.requireOneEventAt(guildY, saturdaysTitle, firstSaturdayStart)
	if got := contents(test.client.messagesTo(channelXGeneral)); len(got) != 1 || got[0] != announcement(fridaysTitle, firstFridayStart) {
		t.Errorf("A's announce channel got %q", got)
	}
	if got := contents(test.client.messagesTo(channelYDesk)); len(got) != 1 || got[0] != announcement(saturdaysTitle, firstSaturdayStart) {
		t.Errorf("B's announce channel got %q", got)
	}

	// Unlinking deletes Beny Bot's upcoming events in the old server, and nothing new is made there.
	if reply := test.run(serverCommand(guildX, dmUserID, "unlink", manageServer)); reply == "" {
		t.Fatal("/unlink gave no reply")
	}
	test.bot.posts.Wait()
	if events := test.client.eventsIn(guildX); len(events) != 0 || test.client.eventDeletes != 1 {
		t.Errorf("after the unlink guild X has %d events (%d deletes), want A's event deleted", len(events), test.client.eventDeletes)
	}
	test.advanceTo(firstFridayStart.Add(4*time.Hour + time.Minute))
	test.plan()
	if events := test.client.eventsIn(guildX); len(events) != 0 || test.client.eventCreates != 2 {
		t.Errorf("after the unlink guild X has %d events (%d creates in all), want nothing new", len(events), test.client.eventCreates)
	}
	test.requireOneEventAt(guildY, saturdaysTitle, firstSaturdayStart)

	// Linking A to another server gives it its own event there.
	test.link(guildZ, test.dmA)
	test.plan()
	test.requireOneEventAt(guildZ, fridaysTitle, firstFridayStart.AddDate(0, 0, 7))
}

func TestThePlannersClockHasNoMonotonicReading(t *testing.T) {
	// A sleeping Mac stops the monotonic clock, so back-off instants must compare by the wall clock.
	if reading := wallClock().String(); strings.Contains(reading, "m=") {
		t.Fatalf("wallClock() = %s carries a monotonic reading", reading)
	}
}

// The planner waits for a server the gateway hasn't delivered yet, then plans on its GuildCreate.
func TestThePlannerWaitsForTheServerToArrive(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	test.createSeries(test.dmA, weekly(fridaysTitle, 5, "2026-10-16"))
	test.state.forgetGuild(guildX)
	test.plan()
	if test.client.eventCreates != 0 {
		t.Fatalf("made %d events in a server the gateway can't see", test.client.eventCreates)
	}
	test.state.addGuild(guildX, "Colony Planner Test",
		&discordgo.Channel{ID: channelXGeneral, Name: "general", Type: discordgo.ChannelTypeGuildText})
	test.plan()
	test.requireOneEventAt(guildX, fridaysTitle, firstFridayStart)
}

func TestThePlannerRunsOnSignals(t *testing.T) {
	test := newWorld(t)
	test.tableReady(guildX, test.dmA, channelXGeneral, nil)
	input := weekly(fridaysTitle, 5, "2026-10-16")
	series := test.createSeries(test.dmA, input)

	plannerContext, cancel := context.WithCancel(test.context)
	var planner sync.WaitGroup
	planner.Go(func() { test.bot.runPlanner(plannerContext) })
	defer func() {
		cancel()
		planner.Wait()
	}()

	test.bot.requestPlan()
	test.eventually("the planner made the event after the signal", func() bool {
		return len(test.client.eventsIn(guildX)) == 1
	})
	input.StartLocal = "19:00"
	// core signals SessionsChanged after the update commits.
	if _, err := test.service.UpdateSessionSeries(test.context, test.dmA, series.ID, input); err != nil {
		t.Fatalf("update: %v", err)
	}
	test.eventually("the planner moved the event after core's signal", func() bool {
		events := test.client.eventsIn(guildX)
		return len(events) == 1 && events[0].ScheduledStartTime.Equal(firstFridayStart.Add(-time.Hour))
	})
}

func (test *world) eventually(what string, condition func() bool) {
	test.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			test.t.Fatalf("timed out waiting: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTheTimerSleepsUntilTheNextWakeWithinItsCap(t *testing.T) {
	clock := newFakeClock()
	benyBot := newBot(Options{Token: "test-token", Logger: quietLogger()}, newFakeDiscordClient(), newFakeGatewayState(), clock.now)
	for _, testCase := range []struct {
		name     string
		wake     *time.Time
		expected time.Duration
	}{
		{"nothing ahead", nil, plannerTimerCap},
		{"due in ten seconds", new(clock.now().Add(10 * time.Second)), 10 * time.Second},
		{"already due", new(clock.now().Add(-time.Hour)), 0},
		{"a day away, checked against the wall clock every minute", new(clock.now().Add(24 * time.Hour)), plannerTimerCap},
	} {
		if delay := benyBot.timerDelay(testCase.wake); delay != testCase.expected {
			t.Errorf("%s: delay = %s, want %s", testCase.name, delay, testCase.expected)
		}
	}
}
