package core

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// The planner's side of core (spec §7.3, §7.4): Beny Bot reads one plan per ACTIVE, linked campaign,
// sets one timer for the earliest NextWake, and runs each due job as claim, then Discord, then record
// (or release on a known failure). These functions take campaign IDs from the plan, not a Viewer: the
// planner acts for no member, and every write is still scoped by that campaign.

type SessionJobKind = game.SessionJobKind

const (
	SessionJobCreateEvent = game.JobCreateEvent
	SessionJobUpdateEvent = game.JobUpdateEvent
	SessionJobMoveEvent   = game.JobMoveEvent
	SessionJobRemoveEvent = game.JobRemoveEvent
	SessionJobRemind      = game.JobRemind
)

// SessionJob is one due job. SeriesID and LocalDate name the occurrence; SessionID is its row when one
// exists, and DiscordEventID is that row's event (always set for update and remove). A move takes the
// event of FromSessionID's row, and DiscordEventID is that one. GuildID is the server the plan read.
type SessionJob struct {
	Kind            SessionJobKind
	CampaignID      uuid.UUID
	GuildID         string
	SeriesID        *uuid.UUID
	FromSessionID   *uuid.UUID
	SessionID       *uuid.UUID
	LocalDate       string
	Title           string
	StartsAt        time.Time
	EndsAt          time.Time
	DurationMinutes int64
	DiscordEventID  *string
}

// CampaignSessionPlan is one campaign's due jobs and the next instant to look again (nil: nothing ahead).
// StoredEventIDs are the Discord events already stored on the campaign's rows, which a create must not
// adopt for another date.
type CampaignSessionPlan struct {
	CampaignID     uuid.UUID
	GuildID        string
	EventLocation  string
	StoredEventIDs map[string]bool
	Jobs           []SessionJob
	NextWake       *time.Time
}

// ClaimedSession is the row a claim holds, as Discord should show it.
type ClaimedSession struct {
	CampaignID      uuid.UUID
	SessionID       uuid.UUID
	Title           string
	StartsAt        time.Time
	EndsAt          time.Time
	DurationMinutes int64
	DiscordEventID  *string
}

// DiscordEventRecord is what the bot made: the event's server and ID, and the start, title, length and
// location it carries.
type DiscordEventRecord struct {
	GuildID         string
	DiscordEventID  string
	StartsAt        time.Time
	Title           string
	DurationMinutes int64
	Location        string
}

// DiscordEventStatus is a scheduled-event update Beny Bot passes on (spec §7.4).
type DiscordEventStatus string

const (
	DiscordEventActive    DiscordEventStatus = "ACTIVE"
	DiscordEventCompleted DiscordEventStatus = "COMPLETED"
)

// PlanSessions reads every ACTIVE campaign linked to a Discord server (the one `-- unscoped: planner`
// query), then each one's series and rows by campaign, and runs game.PlanSessions on the clock.
func (service *Service) PlanSessions(requestContext context.Context) ([]CampaignSessionPlan, error) {
	campaigns, err := service.queries.ListPlannerCampaigns(requestContext)
	if err != nil {
		return nil, err
	}
	now := service.now()
	plans := make([]CampaignSessionPlan, 0, len(campaigns))
	for _, campaign := range campaigns {
		plan, err := service.planCampaign(requestContext, campaign, now)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (service *Service) planCampaign(requestContext context.Context, campaign store.ListPlannerCampaignsRow, now time.Time) (CampaignSessionPlan, error) {
	location, err := tableLocation(campaign.Timezone)
	if err != nil {
		return CampaignSessionPlan{}, err
	}
	seriesRows, err := service.queries.ListSessionSeries(requestContext, campaign.CampaignID)
	if err != nil {
		return CampaignSessionPlan{}, err
	}
	sessionRows, err := service.queries.ListPlannerSessions(requestContext, store.ListPlannerSessionsParams{CampaignID: campaign.CampaignID, Since: now.Add(-plannerLookBehind)})
	if err != nil {
		return CampaignSessionPlan{}, err
	}
	plannerSeries := make([]game.PlannerSeries, 0, len(seriesRows))
	for _, row := range seriesRows {
		plannerSeries = append(plannerSeries, game.PlannerSeries{ID: row.ID, Title: row.Title, Series: gameSeries(row), Announce: row.Announce, Remind: row.Remind})
	}
	plannerSessions := make([]game.PlannerSession, 0, len(sessionRows))
	eventIDs := map[uuid.UUID]*string{}
	storedEventIDs := map[string]bool{}
	for _, row := range sessionRows {
		eventIDs[row.ID] = row.DiscordEventID
		if row.DiscordEventID != nil {
			storedEventIDs[*row.DiscordEventID] = true
		}
		eventLocation := ""
		if row.DiscordEventLocation != nil {
			eventLocation = *row.DiscordEventLocation
		}
		plannerSessions = append(plannerSessions, game.PlannerSession{
			ID:                   row.ID,
			SeriesID:             row.SeriesID,
			LocalDate:            fromPgDate(row.LocalDate),
			Status:               game.SessionState(row.Status),
			HasDiscordEvent:      row.DiscordEventID != nil,
			EventStartsAt:        row.StartsAt,
			EventTitle:           row.Title,
			EventDurationMinutes: int(row.DurationMinutes),
			EventLocation:        eventLocation,
			DiscordClaimedAt:     row.DiscordClaimedAt,
			RemindedAt:           row.RemindedAt,
		})
	}
	plan := game.PlanSessions(now, location, campaign.DiscordEventLocation, plannerSeries, plannerSessions)
	jobs := make([]SessionJob, 0, len(plan.Due))
	for _, due := range plan.Due {
		job := SessionJob{
			Kind:            due.Kind,
			CampaignID:      campaign.CampaignID,
			GuildID:         campaign.GuildID,
			SeriesID:        due.SeriesID,
			SessionID:       due.SessionID,
			FromSessionID:   due.FromSessionID,
			LocalDate:       due.Occurrence.LocalDate.String(),
			Title:           due.Title,
			StartsAt:        due.Occurrence.StartsAt,
			EndsAt:          due.Occurrence.EndsAt,
			DurationMinutes: int64(due.Occurrence.EndsAt.Sub(due.Occurrence.StartsAt) / time.Minute),
		}
		switch {
		case due.FromSessionID != nil:
			job.DiscordEventID = eventIDs[*due.FromSessionID]
		case due.SessionID != nil:
			job.DiscordEventID = eventIDs[*due.SessionID]
		}
		jobs = append(jobs, job)
	}
	return CampaignSessionPlan{
		CampaignID:     campaign.CampaignID,
		GuildID:        campaign.GuildID,
		EventLocation:  campaign.DiscordEventLocation,
		StoredEventIDs: storedEventIDs,
		Jobs:           jobs,
		NextWake:       plan.NextWake,
	}, nil
}

// claimOccurrence re-reads the job's series (it may have changed since the plan), creates the date's row
// on demand and runs claim on it. It returns nil when the job no longer applies or another planner
// holds it.
func (service *Service) claimOccurrence(requestContext context.Context, job SessionJob, wanted func(store.SessionSeries) bool,
	claim func(queries *store.Queries, row store.Session, now time.Time) (store.Session, error),
) (*ClaimedSession, error) {
	if job.SeriesID == nil {
		return nil, nil
	}
	var claimed *ClaimedSession
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		series, occurrence, err := loadOccurrence(requestContext, queries, job.CampaignID, SessionDateInput{SeriesID: *job.SeriesID, LocalDate: job.LocalDate})
		var invalid *Error
		if errors.As(err, &invalid) {
			return nil
		}
		if err != nil {
			return err
		}
		if !wanted(series) {
			return nil
		}
		row, err := ensureSession(requestContext, queries, job.CampaignID, series, occurrence)
		if err != nil {
			return err
		}
		held, err := claim(queries, row, service.now())
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		claimed = &ClaimedSession{
			CampaignID:      job.CampaignID,
			SessionID:       held.ID,
			Title:           series.Title,
			StartsAt:        occurrence.StartsAt,
			EndsAt:          occurrence.EndsAt,
			DurationMinutes: int64(series.DurationMinutes),
			DiscordEventID:  held.DiscordEventID,
		}
		return nil
	})
	return claimed, err
}

// ClaimSessionReminder claims a reminder before sending it: reminded_at is set only if it was empty,
// the date is still scheduled and it hasn't started. A crash between claim and send loses that one
// reminder rather than risking a duplicate.
func (service *Service) ClaimSessionReminder(requestContext context.Context, job SessionJob) (*ClaimedSession, error) {
	return service.claimOccurrence(requestContext, job, func(series store.SessionSeries) bool { return series.Remind },
		func(queries *store.Queries, row store.Session, now time.Time) (store.Session, error) {
			return queries.ClaimSessionReminder(requestContext, store.ClaimSessionReminderParams{CampaignID: job.CampaignID, ID: row.ID, Now: &now})
		})
}

// ReleaseSessionReminder clears a claim after a known send failure, so the next plan retries. It sends
// no re-plan signal: the planner decides when to retry, so a failing Discord can't spin it.
func (service *Service) ReleaseSessionReminder(requestContext context.Context, campaignID uuid.UUID, sessionID uuid.UUID) error {
	return service.queries.ReleaseSessionReminder(requestContext, store.ReleaseSessionReminderParams{CampaignID: campaignID, ID: sessionID})
}

// ClaimSessionDiscordEvent takes the 5-minute lease to create the date's event, only while the campaign
// is ACTIVE and still linked to the job's server. Before creating, the bot lists the server's scheduled
// events and adopts one it made with the same name and start.
func (service *Service) ClaimSessionDiscordEvent(requestContext context.Context, job SessionJob) (*ClaimedSession, error) {
	return service.claimOccurrence(requestContext, job, func(series store.SessionSeries) bool { return series.Announce },
		func(queries *store.Queries, row store.Session, now time.Time) (store.Session, error) {
			return queries.ClaimSessionDiscordEvent(requestContext, store.ClaimSessionDiscordEventParams{
				CampaignID: job.CampaignID, ID: row.ID, Now: &now, LeaseExpiredBefore: new(now.Add(-game.DiscordEventLease)), GuildID: job.GuildID,
			})
		})
}

// ReleaseSessionDiscordClaim clears the lease after a known failure; like ReleaseSessionReminder, it
// sends no re-plan signal.
func (service *Service) ReleaseSessionDiscordClaim(requestContext context.Context, campaignID uuid.UUID, sessionID uuid.UUID) error {
	return service.queries.ReleaseSessionDiscordClaim(requestContext, store.ReleaseSessionDiscordClaimParams{CampaignID: campaignID, ID: sessionID})
}

// RecordSessionDiscordEvent stores the event the bot created or updated on its row, with the start,
// title, length and location it carries, and ends the lease. stillScheduled is false once the DM
// cancelled the date or deleted its series while Discord made the event, so the bot doesn't announce it;
// the next plan removes that event. A row that is gone, or a campaign paused or no longer linked to the
// event's server, is errSessionNotFound: no plan will ever remove that event, so the bot deletes it.
func (service *Service) RecordSessionDiscordEvent(requestContext context.Context, campaignID uuid.UUID, sessionID uuid.UUID, record DiscordEventRecord) (stillScheduled bool, err error) {
	row, err := recordDiscordEvent(requestContext, service.queries, campaignID, sessionID, record)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, errSessionNotFound
	}
	if err != nil {
		return false, err
	}
	return row.Status == SessionStatusScheduled && row.SeriesID != nil, nil
}

func recordDiscordEvent(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, sessionID uuid.UUID, record DiscordEventRecord) (store.Session, error) {
	return queries.RecordSessionDiscordEvent(requestContext, store.RecordSessionDiscordEventParams{
		CampaignID:           campaignID,
		ID:                   sessionID,
		GuildID:              record.GuildID,
		DiscordEventID:       &record.DiscordEventID,
		StartsAt:             record.StartsAt.UTC(),
		DurationMinutes:      int16(record.DurationMinutes),
		Title:                record.Title,
		DiscordEventLocation: &record.Location,
	})
}

// MoveSessionDiscordEvent hands the event the bot just moved in Discord from the row it was made for to
// the job's date, in one transaction. moved is false when the job no longer applies (the series stopped
// announcing or no longer meets that day, the date isn't scheduled or already has an event, or another
// planner moved it first); the next plan then removes or keeps the event as its own row says.
func (service *Service) MoveSessionDiscordEvent(requestContext context.Context, job SessionJob, record DiscordEventRecord) (moved bool, err error) {
	if job.SeriesID == nil || job.FromSessionID == nil {
		return false, nil
	}
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		series, occurrence, err := loadOccurrence(requestContext, queries, job.CampaignID, SessionDateInput{SeriesID: *job.SeriesID, LocalDate: job.LocalDate})
		var invalid *Error
		if errors.As(err, &invalid) {
			return nil
		}
		if err != nil {
			return err
		}
		if !series.Announce || !occurrence.StartsAt.Equal(record.StartsAt) {
			return nil
		}
		target, err := ensureSession(requestContext, queries, job.CampaignID, series, occurrence)
		if err != nil {
			return err
		}
		if target.Status != SessionStatusScheduled || target.DiscordEventID != nil {
			return nil
		}
		if _, err := queries.ForgetSessionDiscordEvent(requestContext, store.ForgetSessionDiscordEventParams{
			CampaignID: job.CampaignID, ID: *job.FromSessionID, DiscordEventID: &record.DiscordEventID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if _, err := recordDiscordEvent(requestContext, queries, job.CampaignID, target.ID, record); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errSessionNotFound
			}
			return err
		}
		moved = true
		return nil
	})
	return moved, err
}

// ForgetSessionDiscordEvent clears a removed event from its row; a row of a deleted series that holds
// nothing else goes with it.
func (service *Service) ForgetSessionDiscordEvent(requestContext context.Context, campaignID uuid.UUID, sessionID uuid.UUID, discordEventID string) error {
	return service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.ForgetSessionDiscordEvent(requestContext, store.ForgetSessionDiscordEventParams{
			CampaignID: campaignID, ID: sessionID, DiscordEventID: &discordEventID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		return queries.DeleteOrphanSession(requestContext, store.DeleteOrphanSessionParams{CampaignID: campaignID, ID: sessionID})
	})
}

// RecordDiscordEventStatus applies a scheduled-event update from the guild's server: ACTIVE marks the
// date played on the colony's current day (SESSION_PLAYED), COMPLETED stamps its last day. Only events
// the bot created are stored on a row, so any other event, another campaign's included, matches nothing
// and is ignored.
func (service *Service) RecordDiscordEventStatus(requestContext context.Context, guildID string, discordEventID string, status DiscordEventStatus) error {
	link, err := service.queries.GetGuildLinkByGuild(requestContext, guildID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	campaign, err := service.queries.GetCampaign(requestContext, link.CampaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if campaign.Status != CampaignStatusActive {
		return nil
	}
	planner := Viewer{campaignID: link.CampaignID}
	changed := false
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := queries.GetColonyForUpdate(requestContext, link.CampaignID)
		if err != nil {
			return campaignGone(err)
		}
		row, err := queries.GetSessionByDiscordEvent(requestContext, store.GetSessionByDiscordEventParams{CampaignID: link.CampaignID, DiscordEventID: &discordEventID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		day := int64(colony.Day)
		if row.Status == SessionStatusScheduled {
			if row, err = markPlayed(requestContext, queries, planner, row, day); err != nil {
				return err
			}
			changed = true
		}
		if status != DiscordEventCompleted || row.Status != SessionStatusPlayed || row.ColonyDayEnd != nil {
			return nil
		}
		if row.ColonyDayStart != nil && day < int64(*row.ColonyDayStart) {
			return nil
		}
		if _, err := queries.EndSession(requestContext, store.EndSessionParams{CampaignID: link.CampaignID, ID: row.ID, ColonyDayEnd: int32Pointer(day)}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil || !changed {
		return err
	}
	service.afterSessionWrite(link.CampaignID, TopicTimeline, TopicColony)
	return nil
}
