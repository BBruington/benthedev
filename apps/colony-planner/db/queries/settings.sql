-- name: GetSettings :one
SELECT * FROM settings WHERE campaign_id = @campaign_id;

-- name: UpdateSettings :one
UPDATE settings
SET timezone = @timezone,
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
