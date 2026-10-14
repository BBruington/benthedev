-- Session series and their dates (spec §4.5, §5.11). Sessions have no secrets: every active member reads
-- them, so no query here has a player projection.

-- name: ListSessionSeries :many
SELECT * FROM session_series WHERE campaign_id = @campaign_id ORDER BY first_date, start_local, title, id;

-- name: GetSessionSeries :one
SELECT * FROM session_series WHERE campaign_id = @campaign_id AND id = @id;

-- name: CountSessionSeries :one
SELECT count(*) FROM session_series WHERE campaign_id = @campaign_id;

-- name: CreateSessionSeries :one
INSERT INTO session_series (campaign_id, title, description, weekdays, every_weeks, first_date, last_date, start_local,
    duration_minutes, announce, remind)
VALUES (@campaign_id, @title, @description, @weekdays, sqlc.narg(every_weeks), @first_date, sqlc.narg(last_date), @start_local,
    @duration_minutes, @announce, @remind)
RETURNING *;

-- name: UpdateSessionSeries :one
UPDATE session_series
SET title = @title, description = @description, weekdays = @weekdays, every_weeks = sqlc.narg(every_weeks),
    first_date = @first_date, last_date = sqlc.narg(last_date), start_local = @start_local,
    duration_minutes = @duration_minutes, announce = @announce, remind = @remind, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: DeleteSessionSeries :execrows
DELETE FROM session_series WHERE campaign_id = @campaign_id AND id = @id;

-- A deleted series' dates that hold nothing worth keeping: not played, no recap, no Discord event (the
-- planner removes that event first), and no NPC met there.
-- name: DeleteIdleSeriesSessions :exec
DELETE FROM session
WHERE session.campaign_id = @campaign_id AND series_id = @series_id AND status <> 'PLAYED' AND recap = '' AND discord_event_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM npc WHERE npc.campaign_id = @campaign_id AND npc.first_session_id = session.id);

-- name: ListSessionsByDate :many
SELECT * FROM session
WHERE campaign_id = @campaign_id AND local_date BETWEEN @from_date AND @to_date
ORDER BY local_date, starts_at, id;

-- Rows whose times follow their series: every row of it that is not played and has no Discord event
-- (the planner moves an event's time itself, then records it).
-- name: ListFollowingSessions :many
SELECT * FROM session
WHERE campaign_id = @campaign_id AND series_id IS NOT NULL AND status <> 'PLAYED' AND discord_event_id IS NULL;

-- name: SetSessionSchedule :exec
UPDATE session SET starts_at = @starts_at, duration_minutes = @duration_minutes, title = @title, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id;

-- name: ListPlayedSessions :many
SELECT * FROM session WHERE campaign_id = @campaign_id AND status = 'PLAYED' ORDER BY starts_at DESC, id LIMIT @row_limit;

-- name: LatestRecap :one
SELECT * FROM session WHERE campaign_id = @campaign_id AND status = 'PLAYED' AND recap <> ''
ORDER BY starts_at DESC, id LIMIT 1;

-- Played sessions whose colony days meet a timeline range: the timeline's session bands. A session
-- still in play (no last day yet) runs on to today, so it meets every range from its first day on.
-- name: ListSessionBands :many
SELECT * FROM session
WHERE campaign_id = @campaign_id AND status = 'PLAYED' AND colony_day_start IS NOT NULL
  AND colony_day_start <= @to_day AND (colony_day_end IS NULL OR colony_day_end >= @from_day)
ORDER BY colony_day_start, starts_at, id;

-- name: GetSession :one
SELECT * FROM session WHERE campaign_id = @campaign_id AND id = @id;

-- name: GetSessionForUpdate :one
SELECT * FROM session WHERE campaign_id = @campaign_id AND id = @id FOR UPDATE;

-- name: GetSessionByDiscordEvent :one
SELECT * FROM session WHERE campaign_id = @campaign_id AND discord_event_id = @discord_event_id FOR UPDATE;

-- Creates an occurrence's row on demand, or returns it. A row that is not played and has no Discord
-- event follows its series' current time and title.
-- name: EnsureSession :one
INSERT INTO session (campaign_id, series_id, local_date, starts_at, duration_minutes, title)
VALUES (@campaign_id, @series_id, @local_date, @starts_at, @duration_minutes, @title)
ON CONFLICT (campaign_id, series_id, local_date) DO UPDATE
SET starts_at = CASE WHEN session.status = 'PLAYED' OR session.discord_event_id IS NOT NULL THEN session.starts_at ELSE excluded.starts_at END,
    duration_minutes = CASE WHEN session.status = 'PLAYED' OR session.discord_event_id IS NOT NULL THEN session.duration_minutes ELSE excluded.duration_minutes END,
    title = CASE WHEN session.status = 'PLAYED' OR session.discord_event_id IS NOT NULL THEN session.title ELSE excluded.title END
RETURNING *;

-- name: CancelSession :one
UPDATE session SET status = 'CANCELLED', updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'SCHEDULED'
RETURNING *;

-- name: RestoreSession :one
UPDATE session SET status = 'SCHEDULED', updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'CANCELLED'
RETURNING *;

-- name: MarkSessionPlayed :one
UPDATE session SET status = 'PLAYED', colony_day_start = @colony_day_start, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'SCHEDULED'
RETURNING *;

-- name: EndSession :one
UPDATE session SET colony_day_end = @colony_day_end, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'PLAYED' AND colony_day_end IS NULL
RETURNING *;

-- name: SetSessionDays :one
UPDATE session SET colony_day_start = @colony_day_start, colony_day_end = sqlc.narg(colony_day_end), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'PLAYED'
RETURNING *;

-- A played session's entry sits on its first day, so it moves when the DM edits that day.
-- name: MoveSessionPlayedEntries :exec
UPDATE timeline_event SET day = @day
WHERE campaign_id = @campaign_id AND session_id = @session_id AND kind = 'SESSION_PLAYED';

-- name: SetSessionRecap :one
UPDATE session SET recap = @recap, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetSessionTitle :one
UPDATE session SET title = @title, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'PLAYED'
RETURNING *;

-- Beny Bot's claims (spec §7.3): claim, then send, so a restart or an overlap can't post twice.
-- name: ClaimSessionReminder :one
UPDATE session SET reminded_at = @now
WHERE campaign_id = @campaign_id AND id = @id AND reminded_at IS NULL AND status = 'SCHEDULED' AND starts_at > @now
RETURNING *;

-- name: ReleaseSessionReminder :exec
UPDATE session SET reminded_at = NULL WHERE campaign_id = @campaign_id AND id = @id;

-- lease_expired_before is now − 5 minutes (game.DiscordEventLease); a lease is over at exactly that age,
-- the instant game.PlanSessions calls the date due again. The campaign must still be ACTIVE and linked
-- to the server the plan read, so a campaign paused or moved meanwhile gets no event there.
-- name: ClaimSessionDiscordEvent :one
UPDATE session SET discord_claimed_at = @now
WHERE session.campaign_id = @campaign_id AND session.id = @id AND session.discord_event_id IS NULL AND session.status = 'SCHEDULED'
  AND (session.discord_claimed_at IS NULL OR session.discord_claimed_at <= @lease_expired_before)
  AND EXISTS (
    SELECT 1 FROM discord_guild_link JOIN campaign ON campaign.id = discord_guild_link.campaign_id
    WHERE discord_guild_link.campaign_id = @campaign_id AND discord_guild_link.guild_id = @guild_id AND campaign.status = 'ACTIVE'
  )
RETURNING *;

-- name: ReleaseSessionDiscordClaim :exec
UPDATE session SET discord_claimed_at = NULL WHERE campaign_id = @campaign_id AND id = @id AND discord_event_id IS NULL;

-- The event's start, title, length and location are recorded on the row, so the planner sees when an
-- edit left the event behind. Like the claim, it holds only while the campaign is ACTIVE and linked to
-- the event's server.
-- name: RecordSessionDiscordEvent :one
UPDATE session
SET discord_event_id = @discord_event_id, discord_claimed_at = NULL, starts_at = @starts_at,
    duration_minutes = @duration_minutes, title = @title, discord_event_location = @discord_event_location, updated_at = now()
WHERE session.campaign_id = @campaign_id AND session.id = @id AND (session.discord_event_id IS NULL OR session.discord_event_id = @discord_event_id)
  AND EXISTS (
    SELECT 1 FROM discord_guild_link JOIN campaign ON campaign.id = discord_guild_link.campaign_id
    WHERE discord_guild_link.campaign_id = @campaign_id AND discord_guild_link.guild_id = @guild_id AND campaign.status = 'ACTIVE'
  )
RETURNING *;

-- name: ForgetSessionDiscordEvent :one
UPDATE session SET discord_event_id = NULL, discord_claimed_at = NULL, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND discord_event_id = @discord_event_id
RETURNING *;

-- Unlinking a server: the events Beny Bot keeps there for dates still ahead, which it then deletes.
-- name: ListUpcomingSessionDiscordEvents :many
SELECT discord_event_id::text FROM session
WHERE campaign_id = @campaign_id AND status <> 'PLAYED' AND discord_event_id IS NOT NULL AND starts_at > @now
ORDER BY starts_at, id;

-- Unlinking forgets every unplayed date's event and lease: they belong to the old server, and the server
-- linked next gets its own.
-- name: ClearSessionDiscordEvents :exec
UPDATE session SET discord_event_id = NULL, discord_claimed_at = NULL, updated_at = now()
WHERE campaign_id = @campaign_id AND status <> 'PLAYED' AND (discord_event_id IS NOT NULL OR discord_claimed_at IS NOT NULL);

-- The calendar feed keeps played dates, a deleted series' ones included.
-- name: ListPlayedSessionsBetween :many
SELECT * FROM session
WHERE campaign_id = @campaign_id AND status = 'PLAYED' AND starts_at >= @from_instant AND starts_at < @to_instant
ORDER BY starts_at, id;

-- A row the planner listed for removal once its event is gone: one whose series was deleted and that
-- holds nothing worth keeping.
-- name: DeleteOrphanSession :exec
DELETE FROM session
WHERE session.campaign_id = @campaign_id AND session.id = @id AND series_id IS NULL AND status <> 'PLAYED' AND recap = '' AND discord_event_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM npc WHERE npc.campaign_id = @campaign_id AND npc.first_session_id = session.id);

-- The planner's rows: what it needs to place events and reminders from `since` on.
-- name: ListPlannerSessions :many
SELECT * FROM session
WHERE campaign_id = @campaign_id AND (starts_at >= @since OR (discord_event_id IS NOT NULL AND status <> 'PLAYED'))
ORDER BY starts_at, id;

-- The planner's one cross-campaign read (docs/design/multi-campaign.md §9.3): only ACTIVE campaigns
-- linked to a Discord server; everything else it reads per campaign.
-- unscoped: planner
-- name: ListPlannerCampaigns :many
SELECT campaign.id AS campaign_id, discord_guild_link.guild_id, settings.timezone, settings.discord_event_location
FROM campaign
JOIN discord_guild_link ON discord_guild_link.campaign_id = campaign.id
JOIN settings ON settings.campaign_id = campaign.id
WHERE campaign.status = 'ACTIVE'
ORDER BY campaign.id;

-- /calendar/{token}.ics names its campaign only by the token (design §2.6, token level); a paused
-- campaign's feed is gone like a wrong token.
-- unscoped: an ICS token names its campaign
-- name: CampaignByIcsToken :one
SELECT settings.campaign_id, settings.discord_event_location, campaign.name AS campaign_name
FROM settings
JOIN campaign ON campaign.id = settings.campaign_id
WHERE settings.ics_token = @ics_token AND campaign.status = 'ACTIVE';

-- name: SetIcsToken :one
UPDATE settings SET ics_token = @ics_token WHERE campaign_id = @campaign_id RETURNING ics_token;

-- name: SetNpcFirstSession :execrows
UPDATE npc SET first_session_id = sqlc.narg(first_session_id), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id;
