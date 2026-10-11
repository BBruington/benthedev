-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- name: UpdateSettings :one
UPDATE settings
SET campaign_name = @campaign_name,
    timezone = @timezone,
    discord_announce_channel_id = sqlc.narg(discord_announce_channel_id),
    discord_reminder_channel_id = sqlc.narg(discord_reminder_channel_id),
    discord_dm_channel_id = sqlc.narg(discord_dm_channel_id)
WHERE id = 1
RETURNING *;
