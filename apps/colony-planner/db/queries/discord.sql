-- discord_guild_link is a directory table: Beny Bot resolves a server to its campaign.

-- unscoped: Beny Bot resolves the server to its campaign, then the caller to a membership there
-- name: ViewerForGuild :one
SELECT discord_guild_link.campaign_id AS linked_campaign_id,
       campaign.status AS campaign_status,
       campaign_member.id AS member_id,
       campaign_member.account_id AS member_account_id,
       campaign_member.role AS member_role,
       campaign_member.status AS member_status
FROM discord_guild_link
JOIN campaign ON campaign.id = discord_guild_link.campaign_id
LEFT JOIN account ON account.discord_user_id = @discord_user_id
LEFT JOIN campaign_member ON campaign_member.campaign_id = discord_guild_link.campaign_id AND campaign_member.account_id = account.id
WHERE discord_guild_link.guild_id = @guild_id;

-- name: GetGuildLink :one
SELECT * FROM discord_guild_link WHERE campaign_id = @campaign_id;

-- name: GetGuildLinkByGuild :one
SELECT * FROM discord_guild_link WHERE guild_id = @guild_id;

-- name: ListGuildLinks :many
SELECT * FROM discord_guild_link ORDER BY linked_at, guild_id;

-- name: CreateGuildLink :one
INSERT INTO discord_guild_link (campaign_id, guild_id, guild_name, linked_by_discord_user_id)
VALUES (@campaign_id, @guild_id, @guild_name, @linked_by_discord_user_id)
RETURNING *;

-- name: DeleteGuildLink :execrows
DELETE FROM discord_guild_link WHERE campaign_id = @campaign_id;

-- name: DeleteGuildLinkByGuild :one
DELETE FROM discord_guild_link WHERE guild_id = @guild_id RETURNING campaign_id;
