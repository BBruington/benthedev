package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// The planner (spec §7.3, docs/design/multi-campaign.md §9.3): one goroutine across every ACTIVE,
// linked campaign. It asks core for each campaign's due jobs, runs them as claim, then Discord, then
// record, and sleeps on one timer until the earliest instant anything falls due. It plans at Ready,
// after core's SessionsChanged signal, when a server's GuildCreate arrives, and after every fire.

const (
	// plannerTimerCap bounds one sleep. A sleeping Mac stops Go's monotonic clock, so a timer set days
	// ahead could fire late; a capped wake compares the wall clock and touches no database.
	plannerTimerCap = time.Minute
	// plannerWorkTimeout bounds one plan and its Discord calls.
	plannerWorkTimeout  = time.Minute
	claimReleaseTimeout = 5 * time.Second
	// A job Discord refused is retried after a minute, then twice as long each time, up to an hour, so
	// a failing Discord can't make the planner spin.
	firstRetryDelay   = time.Minute
	longestRetryDelay = time.Hour
	// discordEventNameMaxLength and discordEventLocationMaxLength are Discord's limits; core's are lower.
	discordEventNameMaxLength     = 100
	discordEventLocationMaxLength = 100
)

// plannedJobKey names a job across plans: by series and date while the date may still lack a row, by
// row for removals (whose series may be gone).
type plannedJobKey struct {
	campaignID uuid.UUID
	kind       core.SessionJobKind
	seriesID   uuid.UUID
	localDate  string
	sessionID  uuid.UUID
}

func keyOf(job core.SessionJob) plannedJobKey {
	key := plannedJobKey{campaignID: job.CampaignID, kind: job.Kind}
	if job.Kind == core.SessionJobRemoveEvent && job.SessionID != nil {
		key.sessionID = *job.SessionID
		return key
	}
	if job.SeriesID != nil {
		key.seriesID = *job.SeriesID
	}
	key.localDate = job.LocalDate
	return key
}

type jobRetry struct {
	failures  int
	notBefore time.Time
}

// jobOutcome is how a job ended: done, failed (retry with back-off) or deferred to an instant.
type jobOutcome struct {
	failed     bool
	deferUntil *time.Time
}

var (
	jobDone   = jobOutcome{}
	jobFailed = jobOutcome{failed: true}
)

// SessionsChanged implements core.SessionPlanner: core calls it after commit, so it only asks the
// planner goroutine to plan again and never blocks the request.
func (benyBot *Bot) SessionsChanged(uuid.UUID) {
	benyBot.requestPlan()
}

func (benyBot *Bot) requestPlan() {
	select {
	case benyBot.replan <- struct{}{}:
	default:
	}
}

// DiscordEventsAbandoned implements core.SessionPlanner: after an unlink, Beny Bot deletes the events it
// kept for upcoming dates in the old server. Core has forgotten them already, so this is best effort: a
// bot that was kicked can't, and a 404 means someone deleted the event first.
func (benyBot *Bot) DiscordEventsAbandoned(requestContext context.Context, abandoned core.AbandonedDiscordEvents) {
	benyBot.inBackground(requestContext, "abandoned events", func(postContext context.Context, _ colonyService) {
		for _, eventID := range abandoned.DiscordEventIDs {
			err := benyBot.client.GuildScheduledEventDelete(abandoned.GuildID, eventID, discordgo.WithContext(postContext))
			if err != nil && !isUnknownEvent(err) {
				benyBot.options.Logger.Info("Beny Bot couldn't delete an event in a server it was unlinked from",
					slog.String("campaign_id", abandoned.CampaignID.String()), slog.String("guild_id", abandoned.GuildID),
					slog.String("discord_event_id", eventID), slog.Any("error", err))
			}
		}
	})
}

// SessionCancelled implements core.SessionPlanner: the old cancellation notice, posted to the reminder
// channel (or the announce channel while that is unset). The planner removes the date's event itself.
// A date already past cancels nothing anyone could still plan for, so it is not announced.
func (benyBot *Bot) SessionCancelled(requestContext context.Context, cancelled core.CancelledSession) {
	if !cancelled.StartsAt.After(benyBot.now()) {
		return
	}
	content := fmt.Sprintf("❌ **%s** on %s has been cancelled.", escapeMarkdown(cancelled.Title), discordTimestamp(cancelled.StartsAt, "F"))
	benyBot.inBackground(requestContext, "cancellation notice", func(postContext context.Context, service colonyService) {
		benyBot.postToCampaign(postContext, service, cancelled.CampaignID, sessionNoticeChannel, &discordgo.MessageSend{Content: content}, nil)
	})
}

// runPlanner is the planner goroutine: session jobs and poll deadlines. Before the first plan (Ready)
// only deadlines that poll changes reported are timed.
func (benyBot *Bot) runPlanner(lifetimeContext context.Context) {
	var sessionWake *time.Time
	timer := time.NewTimer(plannerTimerCap)
	defer timer.Stop()
	for {
		select {
		case <-lifetimeContext.Done():
			return
		case <-benyBot.replan:
			sessionWake = benyBot.planSessions(lifetimeContext)
		case <-benyBot.pollsChanged:
		case <-timer.C:
			benyBot.retryDuePollPosts()
			benyBot.closeDuePolls(lifetimeContext)
			if sessionWake != nil && !benyBot.now().Before(*sessionWake) {
				sessionWake = benyBot.planSessions(lifetimeContext)
			}
		}
		wake := sessionWake
		if pollWake := benyBot.nextPollWake(); pollWake != nil && (wake == nil || pollWake.Before(*wake)) {
			wake = pollWake
		}
		timer.Reset(benyBot.timerDelay(wake))
	}
}

func (benyBot *Bot) timerDelay(wake *time.Time) time.Duration {
	if wake == nil {
		return plannerTimerCap
	}
	return min(max(wake.Sub(benyBot.now()), 0), plannerTimerCap)
}

// planSessions plans, runs every due job, and plans again until a plan has nothing new to run, so it
// fires each job at most once per wake. It returns the next instant to wake, nil for none.
func (benyBot *Bot) planSessions(lifetimeContext context.Context) *time.Time {
	service := benyBot.currentService()
	if service == nil {
		return nil
	}
	workContext, cancel := context.WithTimeout(lifetimeContext, plannerWorkTimeout)
	defer cancel()
	logger := benyBot.options.Logger
	attempted := map[plannedJobKey]bool{}
	for {
		plans, err := service.PlanSessions(workContext)
		if err != nil {
			logger.Error("Beny Bot couldn't plan sessions; it tries again in a minute", slog.Any("error", err))
			return new(benyBot.now().Add(firstRetryDelay))
		}
		if len(attempted) == 0 {
			benyBot.reloadPollDeadlines(workContext, service, plans)
		}
		var wake *time.Time
		wakeAt := func(instant *time.Time) {
			if instant != nil && (wake == nil || instant.Before(*wake)) {
				wake = new(*instant)
			}
		}
		planned := map[plannedJobKey]bool{}
		ranAny := false
		for _, plan := range plans {
			wakeAt(plan.NextWake)
			if len(plan.Jobs) == 0 {
				continue
			}
			if !benyBot.guildVisible(plan.GuildID) {
				logger.Info("the campaign's server isn't in the gateway state yet; its sessions wait for it", slog.String("campaign_id", plan.CampaignID.String()), slog.String("guild_id", plan.GuildID))
				continue
			}
			for _, job := range plan.Jobs {
				key := keyOf(job)
				planned[key] = true
				if retry, waiting := benyBot.retries[key]; waiting && benyBot.now().Before(retry.notBefore) {
					wakeAt(&retry.notBefore)
					continue
				}
				if attempted[key] {
					// Still due right after it ran: wait rather than run it again at once.
					wakeAt(benyBot.backOff(key))
					continue
				}
				attempted[key] = true
				ranAny = true
				outcome := benyBot.runJob(workContext, service, plan, job)
				switch {
				case outcome.deferUntil != nil:
					benyBot.retries[key] = jobRetry{notBefore: *outcome.deferUntil}
				case outcome.failed:
					benyBot.backOff(key)
				default:
					delete(benyBot.retries, key)
				}
			}
		}
		if !ranAny {
			for key := range benyBot.retries {
				if !planned[key] {
					delete(benyBot.retries, key)
				}
			}
			return wake
		}
	}
}

// backOff records one more failure of the job and returns when it may run again.
func (benyBot *Bot) backOff(key plannedJobKey) *time.Time {
	retry := benyBot.retries[key]
	delay := firstRetryDelay
	for range retry.failures {
		delay = min(delay*2, longestRetryDelay)
	}
	retry.failures++
	retry.notBefore = benyBot.now().Add(delay)
	benyBot.retries[key] = retry
	return &retry.notBefore
}

// guildVisible reports whether the gateway state has the server, so its channels can be checked and
// its events listed. A server that hasn't arrived yet is planned again on its GuildCreate.
func (benyBot *Bot) guildVisible(guildID string) bool {
	if benyBot.state == nil || guildID == "" {
		return false
	}
	guild, err := benyBot.state.Guild(guildID)
	if err != nil || guild == nil {
		return false
	}
	benyBot.state.RLock()
	defer benyBot.state.RUnlock()
	return !guild.Unavailable
}

func (benyBot *Bot) runJob(workContext context.Context, service colonyService, plan core.CampaignSessionPlan, job core.SessionJob) jobOutcome {
	logger := benyBot.options.Logger.With(
		slog.String("campaign_id", job.CampaignID.String()),
		slog.String("job", string(job.Kind)),
		slog.String("local_date", job.LocalDate),
	)
	switch job.Kind {
	case core.SessionJobRemind:
		return benyBot.remind(workContext, logger, service, job)
	case core.SessionJobCreateEvent:
		return benyBot.createEvent(workContext, logger, service, plan, job)
	case core.SessionJobUpdateEvent:
		return benyBot.updateEvent(workContext, logger, service, plan, job)
	case core.SessionJobMoveEvent:
		return benyBot.moveEvent(workContext, logger, service, plan, job)
	case core.SessionJobRemoveEvent:
		return benyBot.removeEvent(workContext, logger, service, plan, job)
	}
	logger.Warn("Beny Bot doesn't know this planner job")
	return jobDone
}

// remind claims the date's reminder, then posts it. Only a post that failed (Discord refused it, or
// the campaign's server couldn't be read) gives the claim back for a retry; a campaign with no channel
// to post to has nothing to retry.
func (benyBot *Bot) remind(workContext context.Context, logger *slog.Logger, service colonyService, job core.SessionJob) jobOutcome {
	claimed, err := service.ClaimSessionReminder(workContext, job)
	if err != nil {
		logger.Error("Beny Bot couldn't claim a reminder", slog.Any("error", err))
		return jobFailed
	}
	if claimed == nil {
		return jobDone
	}
	content := fmt.Sprintf("Reminder: **%s** is coming up %s (%s). Don't forget!",
		escapeMarkdown(claimed.Title), discordTimestamp(claimed.StartsAt, "R"), discordTimestamp(claimed.StartsAt, "F"))
	_, failed := benyBot.postToCampaign(workContext, service, claimed.CampaignID, sessionNoticeChannel, &discordgo.MessageSend{Content: content}, nil)
	if !failed {
		return jobDone
	}
	// The post may have failed because the plan's shared deadline ran out, so the release gets its own.
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(workContext), claimReleaseTimeout)
	defer cancel()
	if err := service.ReleaseSessionReminder(releaseContext, claimed.CampaignID, claimed.SessionID); err != nil {
		logger.Error("Beny Bot couldn't give back a reminder it failed to send; that reminder is lost", slog.Any("error", err))
	}
	return jobFailed
}

// createEvent takes the date's 5-minute lease, adopts the event a crashed run already made (same
// name and start, made by Beny Bot) or creates an external one, records it, then announces it.
func (benyBot *Bot) createEvent(workContext context.Context, logger *slog.Logger, service colonyService, plan core.CampaignSessionPlan, job core.SessionJob) jobOutcome {
	// Discord refuses an event that starts in the past; once this date ends the series rolls on.
	if !job.StartsAt.After(benyBot.now()) {
		return jobOutcome{deferUntil: new(job.EndsAt)}
	}
	claimed, err := service.ClaimSessionDiscordEvent(workContext, job)
	if err != nil {
		logger.Error("Beny Bot couldn't claim a Discord event", slog.Any("error", err))
		return jobFailed
	}
	if claimed == nil {
		return jobDone
	}
	release := func() jobOutcome {
		if err := service.ReleaseSessionDiscordClaim(workContext, claimed.CampaignID, claimed.SessionID); err != nil {
			logger.Error("Beny Bot couldn't release a Discord event claim; it expires in 5 minutes", slog.Any("error", err))
		}
		return jobFailed
	}
	name := eventName(claimed.Title)
	eventID, found, err := benyBot.findOwnEvent(workContext, plan.GuildID, name, claimed.StartsAt, plan.StoredEventIDs)
	if err != nil {
		logger.Warn("Beny Bot couldn't list the server's events", slog.String("guild_id", plan.GuildID), slog.Any("error", err))
		return release()
	}
	if !found {
		created, err := benyBot.client.GuildScheduledEventCreate(plan.GuildID, eventParams(name, claimed.StartsAt, claimed.EndsAt, plan.EventLocation), discordgo.WithContext(workContext))
		if err != nil || created == nil {
			logger.Warn("Beny Bot couldn't create a Discord event", slog.String("guild_id", plan.GuildID), slog.Any("error", err))
			return release()
		}
		eventID = created.ID
	}
	stillScheduled, err := service.RecordSessionDiscordEvent(workContext, claimed.CampaignID, claimed.SessionID, core.DiscordEventRecord{
		GuildID: plan.GuildID, DiscordEventID: eventID, StartsAt: claimed.StartsAt, Title: claimed.Title,
		DurationMinutes: claimed.DurationMinutes, Location: plan.EventLocation,
	})
	if isNotFound(err) {
		// The DM deleted the series, or unlinked or the owner paused the campaign, while Discord made the
		// event, so no later plan knows this event to remove it.
		benyBot.deleteUnrecordedEvent(workContext, logger, plan.GuildID, eventID)
		return jobDone
	}
	if err != nil {
		// The lease runs out and the next try adopts this event instead of making a second one.
		logger.Error("Beny Bot couldn't store a Discord event it made", slog.String("discord_event_id", eventID), slog.Any("error", err))
		return jobFailed
	}
	if !stillScheduled {
		return jobDone
	}
	content := fmt.Sprintf("**%s** is scheduled for %s", escapeMarkdown(claimed.Title), sessionWhen(claimed.StartsAt))
	benyBot.postToCampaign(workContext, service, claimed.CampaignID, announceChannel, &discordgo.MessageSend{Content: content}, nil)
	return jobDone
}

func (benyBot *Bot) deleteUnrecordedEvent(workContext context.Context, logger *slog.Logger, guildID string, eventID string) {
	if err := benyBot.client.GuildScheduledEventDelete(guildID, eventID, discordgo.WithContext(workContext)); err != nil && !isUnknownEvent(err) {
		logger.Warn("Beny Bot couldn't delete an event no session holds", slog.String("discord_event_id", eventID), slog.Any("error", err))
	}
}

// findOwnEvent looks for an event Beny Bot made in the server with this name and start that no date of
// the campaign holds yet: another date with the same title and start keeps its own event.
func (benyBot *Bot) findOwnEvent(workContext context.Context, guildID string, name string, startsAt time.Time, storedEventIDs map[string]bool) (string, bool, error) {
	benyBot.mutex.Lock()
	botUserID := benyBot.botUserID
	benyBot.mutex.Unlock()
	if botUserID == "" {
		return "", false, errors.New("the bot's own user ID isn't known before Ready")
	}
	events, err := benyBot.client.GuildScheduledEvents(guildID, false, discordgo.WithContext(workContext))
	if err != nil {
		return "", false, err
	}
	for _, event := range events {
		if event == nil || storedEventIDs[event.ID] || event.CreatorID != botUserID || event.Name != name || !event.ScheduledStartTime.Equal(startsAt) {
			continue
		}
		if event.Status == discordgo.GuildScheduledEventStatusScheduled || event.Status == discordgo.GuildScheduledEventStatusActive {
			return event.ID, true, nil
		}
	}
	return "", false, nil
}

// updateEvent moves the date's event to its series' current title, start and length. An event that
// was deleted in Discord is forgotten, so the next plan creates a new one.
func (benyBot *Bot) updateEvent(workContext context.Context, logger *slog.Logger, service colonyService, plan core.CampaignSessionPlan, job core.SessionJob) jobOutcome {
	if job.SessionID == nil || job.DiscordEventID == nil {
		return jobDone
	}
	if !job.StartsAt.After(benyBot.now()) {
		return jobOutcome{deferUntil: new(job.EndsAt)}
	}
	eventID := *job.DiscordEventID
	_, err := benyBot.client.GuildScheduledEventEdit(plan.GuildID, eventID, eventParams(eventName(job.Title), job.StartsAt, job.EndsAt, plan.EventLocation), discordgo.WithContext(workContext))
	if isUnknownEvent(err) {
		return benyBot.forgetEvent(workContext, logger, service, job)
	}
	if err != nil {
		logger.Warn("Beny Bot couldn't update a Discord event", slog.String("discord_event_id", eventID), slog.Any("error", err))
		return jobFailed
	}
	_, err = service.RecordSessionDiscordEvent(workContext, job.CampaignID, *job.SessionID, core.DiscordEventRecord{
		GuildID: plan.GuildID, DiscordEventID: eventID, StartsAt: job.StartsAt, Title: job.Title,
		DurationMinutes: job.DurationMinutes, Location: plan.EventLocation,
	})
	if isNotFound(err) {
		// Unlinked or paused meanwhile: the unlink deletes the campaign's events itself.
		return jobDone
	}
	if err != nil {
		logger.Error("Beny Bot couldn't store a Discord event it updated", slog.String("discord_event_id", eventID), slog.Any("error", err))
		return jobFailed
	}
	return jobDone
}

// moveEvent moves the event of a date the series no longer meets on to its next date (spec §7.4: "Edit a
// series. Update the existing event to the new next occurrence."), so the event keeps its "interested"
// list, then hands it to that date's row.
func (benyBot *Bot) moveEvent(workContext context.Context, logger *slog.Logger, service colonyService, plan core.CampaignSessionPlan, job core.SessionJob) jobOutcome {
	if job.FromSessionID == nil || job.DiscordEventID == nil {
		return jobDone
	}
	eventID := *job.DiscordEventID
	_, err := benyBot.client.GuildScheduledEventEdit(plan.GuildID, eventID, eventParams(eventName(job.Title), job.StartsAt, job.EndsAt, plan.EventLocation), discordgo.WithContext(workContext))
	if isUnknownEvent(err) {
		return benyBot.forgetEvent(workContext, logger, service, core.SessionJob{CampaignID: job.CampaignID, SessionID: job.FromSessionID, DiscordEventID: job.DiscordEventID})
	}
	if err != nil {
		logger.Warn("Beny Bot couldn't move a Discord event", slog.String("discord_event_id", eventID), slog.Any("error", err))
		return jobFailed
	}
	if _, err := service.MoveSessionDiscordEvent(workContext, job, core.DiscordEventRecord{
		GuildID: plan.GuildID, DiscordEventID: eventID, StartsAt: job.StartsAt, Title: job.Title,
		DurationMinutes: job.DurationMinutes, Location: plan.EventLocation,
	}); err != nil && !isNotFound(err) {
		logger.Error("Beny Bot couldn't store a Discord event it moved", slog.String("discord_event_id", eventID), slog.Any("error", err))
		return jobFailed
	}
	return jobDone
}

// removeEvent deletes the event of a cancelled date, a deleted series or a date that is no longer next.
func (benyBot *Bot) removeEvent(workContext context.Context, logger *slog.Logger, service colonyService, plan core.CampaignSessionPlan, job core.SessionJob) jobOutcome {
	if job.SessionID == nil || job.DiscordEventID == nil {
		return jobDone
	}
	err := benyBot.client.GuildScheduledEventDelete(plan.GuildID, *job.DiscordEventID, discordgo.WithContext(workContext))
	if err != nil && !isUnknownEvent(err) {
		logger.Warn("Beny Bot couldn't delete a Discord event", slog.String("discord_event_id", *job.DiscordEventID), slog.Any("error", err))
		return jobFailed
	}
	return benyBot.forgetEvent(workContext, logger, service, job)
}

func (benyBot *Bot) forgetEvent(workContext context.Context, logger *slog.Logger, service colonyService, job core.SessionJob) jobOutcome {
	if err := service.ForgetSessionDiscordEvent(workContext, job.CampaignID, *job.SessionID, *job.DiscordEventID); err != nil {
		logger.Error("Beny Bot couldn't forget a removed Discord event", slog.String("discord_event_id", *job.DiscordEventID), slog.Any("error", err))
		return jobFailed
	}
	return jobDone
}

// eventParams is an external event in the server, at the campaign's event location (spec §7.4).
func eventParams(name string, startsAt time.Time, endsAt time.Time, location string) *discordgo.GuildScheduledEventParams {
	return &discordgo.GuildScheduledEventParams{
		Name:               name,
		ScheduledStartTime: new(startsAt.UTC()),
		ScheduledEndTime:   new(endsAt.UTC()),
		PrivacyLevel:       discordgo.GuildScheduledEventPrivacyLevelGuildOnly,
		EntityType:         discordgo.GuildScheduledEventEntityTypeExternal,
		EntityMetadata:     &discordgo.GuildScheduledEventEntityMetadata{Location: fitText(location, discordEventLocationMaxLength, keepText)},
	}
}

// eventName is the title as Discord shows it: plain text, so nothing is escaped.
func eventName(title string) string {
	return fitText(title, discordEventNameMaxLength, keepText)
}

func isNotFound(err error) bool {
	var operationError *core.Error
	return errors.As(err, &operationError) && operationError.Kind == core.KindNotFound
}

// isUnknownEvent reports Discord's "Unknown Guild Scheduled Event": someone deleted it in Discord.
func isUnknownEvent(err error) bool {
	var restError *discordgo.RESTError
	return errors.As(err, &restError) && restError.Message != nil && restError.Message.Code == discordgo.ErrCodeUnknownGuildScheduledEvent
}

// handleScheduledEventUpdate passes an event going ACTIVE or COMPLETED to core, which matches it only
// against the events stored on that server's campaign, so another server's update never reaches
// another campaign and an event someone else made is ignored.
func (benyBot *Bot) handleScheduledEventUpdate(lifetimeContext context.Context, update *discordgo.GuildScheduledEventUpdate) {
	if update == nil || update.GuildScheduledEvent == nil || update.GuildID == "" || update.ID == "" {
		return
	}
	var status core.DiscordEventStatus
	switch update.Status {
	case discordgo.GuildScheduledEventStatusActive:
		status = core.DiscordEventActive
	case discordgo.GuildScheduledEventStatusCompleted:
		status = core.DiscordEventCompleted
	default:
		return
	}
	service := benyBot.currentService()
	if service == nil {
		return
	}
	workContext, cancel := context.WithTimeout(lifetimeContext, gatewayWorkTimeout)
	defer cancel()
	if err := service.RecordDiscordEventStatus(workContext, update.GuildID, update.ID, status); err != nil {
		benyBot.options.Logger.Error("Beny Bot couldn't record a Discord event's status",
			slog.String("guild_id", update.GuildID), slog.String("discord_event_id", update.ID), slog.String("status", string(status)), slog.Any("error", err))
	}
}
