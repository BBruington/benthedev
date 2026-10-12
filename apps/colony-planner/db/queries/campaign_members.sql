-- A membership is one account in one campaign. Every query here takes the campaign, except the list
-- of an account's own memberships.

-- The one read EnterCampaign makes: the membership and its campaign. No row means no such campaign.
-- name: GetMembershipForAccount :one
SELECT sqlc.embed(campaign), sqlc.embed(campaign_member)
FROM campaign_member
JOIN campaign ON campaign.id = campaign_member.campaign_id
WHERE campaign_member.campaign_id = @campaign_id AND campaign_member.account_id = @account_id;

-- unscoped: an account's own memberships across campaigns, for the picker
-- name: ListMembershipsForAccount :many
SELECT sqlc.embed(campaign), sqlc.embed(campaign_member)
FROM campaign_member
JOIN campaign ON campaign.id = campaign_member.campaign_id
WHERE campaign_member.account_id = @account_id
ORDER BY campaign.name, campaign.id;

-- name: ListCampaignMembers :many
SELECT sqlc.embed(campaign_member), sqlc.embed(account)
FROM campaign_member
JOIN account ON account.id = campaign_member.account_id
WHERE campaign_member.campaign_id = @campaign_id
ORDER BY campaign_member.created_at, campaign_member.id;

-- name: GetCampaignMember :one
SELECT sqlc.embed(campaign_member), sqlc.embed(account)
FROM campaign_member
JOIN account ON account.id = campaign_member.account_id
WHERE campaign_member.campaign_id = @campaign_id AND campaign_member.id = @id;

-- name: GetCampaignMemberForUpdate :one
SELECT sqlc.embed(campaign_member), sqlc.embed(account)
FROM campaign_member
JOIN account ON account.id = campaign_member.account_id
WHERE campaign_member.campaign_id = @campaign_id AND campaign_member.id = @id
FOR UPDATE OF campaign_member;

-- name: GetCampaignMemberByAccountForUpdate :one
SELECT * FROM campaign_member
WHERE campaign_id = @campaign_id AND account_id = @account_id
FOR UPDATE;

-- The campaign's one DM, for the invite page ("run by Sample DM").
-- name: GetCampaignDm :one
SELECT sqlc.embed(account)
FROM campaign_member
JOIN account ON account.id = campaign_member.account_id
WHERE campaign_member.campaign_id = @campaign_id AND campaign_member.role = 'DM';

-- A campaign with its one DM's name, for the caller's own view of it ("Waiting for Sample DM").
-- name: GetCampaignWithDm :one
SELECT sqlc.embed(campaign), account.display_name AS dm_display_name
FROM campaign
JOIN campaign_member ON campaign_member.campaign_id = @campaign_id AND campaign_member.role = 'DM'
JOIN account ON account.id = campaign_member.account_id
WHERE campaign.id = @campaign_id;

-- name: CountPendingCampaignMembers :one
SELECT count(*) FROM campaign_member WHERE campaign_id = @campaign_id AND status = 'PENDING';

-- Serializes joins to one campaign for the rest of the transaction, so two people asking at once
-- can't both slip under the pending cap.
-- name: LockCampaignJoins :exec
SELECT pg_advisory_xact_lock(hashtext('colony-planner:campaign-join'), hashtext(CAST(sqlc.arg(campaign_id)::uuid AS text)));

-- name: CreateCampaignMember :one
INSERT INTO campaign_member (campaign_id, account_id, role, status, character_name, created_at, approved_at)
VALUES (
    @campaign_id, @account_id, @role, @status, sqlc.narg(character_name),
    COALESCE(sqlc.narg(created_at)::timestamptz, now()), sqlc.narg(approved_at)
)
RETURNING *;

-- name: SetCampaignMemberStatus :one
UPDATE campaign_member
SET status = @status,
    left_on_own = @left_on_own,
    approved_at = CASE WHEN @status = 'ACTIVE'::member_status THEN now() ELSE approved_at END
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetCampaignMemberCharacterName :one
UPDATE campaign_member SET character_name = sqlc.narg(character_name)
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;
