-- campaign is a directory table: the owner lists it, and the invite page reads it by code.

-- name: CreateCampaign :one
INSERT INTO campaign (slug, name, invite_code) VALUES (@slug, @name, @invite_code) RETURNING *;

-- name: GetCampaign :one
SELECT * FROM campaign WHERE id = @campaign_id;

-- name: GetCampaignForUpdate :one
SELECT * FROM campaign WHERE id = @campaign_id FOR UPDATE;

-- unscoped: the invite page resolves a campaign from its code
-- name: CampaignByInviteCode :one
SELECT * FROM campaign WHERE invite_code = @invite_code;

-- A slug is taken by a campaign or by another open request.
-- name: SlugTaken :one
SELECT EXISTS (SELECT 1 FROM campaign WHERE campaign.slug = @slug::text)
    OR EXISTS (
        SELECT 1 FROM campaign_request
        WHERE campaign_request.slug = @slug::text AND campaign_request.status = 'PENDING' AND campaign_request.id <> @except_request_id::uuid
    ) AS taken;

-- name: SetCampaignName :one
UPDATE campaign SET name = @name WHERE id = @campaign_id RETURNING *;

-- name: SetCampaignInviteCode :one
UPDATE campaign SET invite_code = @invite_code WHERE id = @campaign_id RETURNING *;

-- name: SetCampaignStatus :one
UPDATE campaign
SET status = @status,
    suspended_at = CASE WHEN @status = 'SUSPENDED'::campaign_status THEN now() ELSE NULL END
WHERE id = @campaign_id
RETURNING *;

-- Everything the campaign owns cascades; accounts and their other campaigns stay.
-- name: DeleteCampaign :execrows
DELETE FROM campaign WHERE id = @campaign_id;

-- unscoped: the site owner's directory counts every campaign, and reads no campaign content
-- name: ListCampaignsForOwner :many
SELECT campaign.*,
       dm_account.display_name AS dm_display_name,
       dm_account.discord_username AS dm_discord_username,
       (SELECT count(*) FROM campaign_member AS players
        WHERE players.campaign_id = campaign.id AND players.role = 'PLAYER' AND players.status = 'ACTIVE')::bigint AS player_count,
       (SELECT count(*) FROM campaign_member AS waiting
        WHERE waiting.campaign_id = campaign.id AND waiting.status = 'PENDING')::bigint AS pending_count,
       EXISTS (SELECT 1 FROM discord_guild_link WHERE discord_guild_link.campaign_id = campaign.id) AS discord_linked
FROM campaign
JOIN campaign_member AS dm_member ON dm_member.campaign_id = campaign.id AND dm_member.role = 'DM'
JOIN account AS dm_account ON dm_account.id = dm_member.account_id
ORDER BY campaign.created_at, campaign.id;

-- A new campaign's rows: the column defaults (The Colony, Day 0, UTC, no channels), and the resources
-- and roles in CreateDefaultResources and CreateDefaultRoles, the ones M1's migrations inserted. They
-- are data, not rules.
-- name: CreateSettings :exec
INSERT INTO settings (campaign_id) VALUES (@campaign_id);

-- name: CreateColony :exec
INSERT INTO colony (campaign_id) VALUES (@campaign_id);

-- name: CreateDefaultResources :exec
INSERT INTO resource (campaign_id, key, label, icon, amount, min_amount, max_amount, sort_order, is_system)
SELECT @campaign_id, defaults.key, defaults.label, defaults.icon::colony_icon, defaults.amount, 0, defaults.max_amount, defaults.sort_order, defaults.is_system
FROM (VALUES
    ('gold', 'Gold', 'Coins', 0, NULL::integer, 10, false),
    ('food', 'Food', 'Wheat', 0, NULL::integer, 20, true),
    ('materials', 'Materials', 'Boxes', 0, NULL::integer, 30, false),
    ('morale', 'Morale', 'HeartHandshake', 100, 100, 40, false)
) AS defaults (key, label, icon, amount, max_amount, sort_order, is_system);

-- name: CreateDefaultRoles :exec
INSERT INTO role (campaign_id, key, label, icon, sort_order, is_system)
SELECT @campaign_id, defaults.key, defaults.label, defaults.icon::colony_icon, defaults.sort_order, defaults.is_system
FROM (VALUES
    ('FARMER', 'Farmer', 'Wheat', 10, false),
    ('HEALER', 'Healer', 'HeartPulse', 20, false),
    ('BLACKSMITH', 'Blacksmith', 'Hammer', 30, false),
    ('SOLDIER', 'Soldier', 'Sword', 40, false),
    ('MINER', 'Miner', 'Pickaxe', 50, false),
    ('BUILDER', 'Builder', 'HardHat', 60, false),
    ('SCHOLAR', 'Scholar', 'BookOpen', 70, false),
    ('MAGE', 'Mage', 'WandSparkles', 80, false),
    ('UNASSIGNED', 'Unassigned', 'Users', 90, true)
) AS defaults (key, label, icon, sort_order, is_system);

-- campaign_request is a directory table: an account asks, the owner decides.

-- name: CreateCampaignRequest :one
INSERT INTO campaign_request (account_id, campaign_name, slug, note)
VALUES (@account_id, @campaign_name, @slug, @note)
RETURNING *;

-- The account's newest request, whatever its status, for /request.
-- name: LatestCampaignRequestForAccount :one
SELECT * FROM campaign_request
WHERE account_id = @account_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountOpenCampaignRequestsForAccount :one
SELECT count(*) FROM campaign_request WHERE account_id = @account_id AND status = 'PENDING';

-- name: GetCampaignRequestForUpdate :one
SELECT * FROM campaign_request WHERE id = @id FOR UPDATE;

-- name: ListOpenCampaignRequests :many
SELECT sqlc.embed(campaign_request), sqlc.embed(account)
FROM campaign_request
JOIN account ON account.id = campaign_request.account_id
WHERE campaign_request.status = 'PENDING'
ORDER BY campaign_request.created_at, campaign_request.id;

-- name: DecideCampaignRequest :one
UPDATE campaign_request
SET status = @status,
    decided_at = now(),
    slug = @slug,
    campaign_id = sqlc.narg(campaign_id)
WHERE id = @id
RETURNING *;
