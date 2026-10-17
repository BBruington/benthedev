-- name: GetSettings :one
SELECT * FROM settings WHERE campaign_id = @campaign_id;

-- name: UpdateSettings :one
UPDATE settings
SET timezone = @timezone,
    discord_event_location = @discord_event_location,
    discord_announce_channel_id = sqlc.narg(discord_announce_channel_id),
    discord_reminder_channel_id = sqlc.narg(discord_reminder_channel_id),
    discord_dm_channel_id = sqlc.narg(discord_dm_channel_id)
WHERE campaign_id = @campaign_id
RETURNING *;

-- Linking or unlinking a Discord server clears the channels: they belonged to the old server.
-- name: ClearSettingsChannels :exec
UPDATE settings
SET discord_announce_channel_id = NULL,
    discord_reminder_channel_id = NULL,
    discord_dm_channel_id = NULL
WHERE campaign_id = @campaign_id;

-- The board's header: every member reads the campaign's name and the table's time zone, which has no
-- secret, so players see "8:00 PM New York time" rather than a UTC offset.
-- name: GetBoardHeader :one
SELECT campaign.name AS campaign_name, settings.timezone
FROM campaign
JOIN settings ON settings.campaign_id = campaign.id
WHERE campaign.id = @campaign_id AND settings.campaign_id = @campaign_id;
