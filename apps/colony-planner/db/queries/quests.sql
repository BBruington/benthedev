-- Quest reads take dm_projection (viewer.SeesDmFields()): players see only revealed quests, and the
-- giver NPC's and patron's names beside them only while those are revealed too (spec §5.9). The quest
-- row still carries giver_npc_id and patron_id, which core drops for a hidden giver or patron. An
-- archived patron still names its quests: archiving keeps history intact.

-- name: ListQuests :many
SELECT sqlc.embed(quest),
       giver.display_name AS giver_npc_name,
       patron.name AS patron_name
FROM quest
LEFT JOIN npc AS giver ON giver.campaign_id = quest.campaign_id
    AND giver.id = quest.giver_npc_id AND (@dm_projection::boolean OR giver.revealed)
LEFT JOIN patron ON patron.campaign_id = quest.campaign_id
    AND patron.id = quest.patron_id AND (@dm_projection::boolean OR patron.revealed)
WHERE quest.campaign_id = @campaign_id
  AND (@dm_projection::boolean OR quest.revealed)
ORDER BY quest.created_at DESC, quest.id;

-- name: GetQuestView :one
SELECT sqlc.embed(quest),
       giver.display_name AS giver_npc_name,
       patron.name AS patron_name
FROM quest
LEFT JOIN npc AS giver ON giver.campaign_id = quest.campaign_id
    AND giver.id = quest.giver_npc_id AND (@dm_projection::boolean OR giver.revealed)
LEFT JOIN patron ON patron.campaign_id = quest.campaign_id
    AND patron.id = quest.patron_id AND (@dm_projection::boolean OR patron.revealed)
WHERE quest.campaign_id = @campaign_id
  AND quest.id = @id
  AND (@dm_projection::boolean OR quest.revealed);

-- DM reads and colony mutations: an ID from a request body is loaded with the viewer's campaign.
-- name: GetQuest :one
SELECT * FROM quest WHERE campaign_id = @campaign_id AND id = @id;

-- name: CountQuests :one
SELECT count(*) FROM quest WHERE campaign_id = @campaign_id;

-- name: CreateQuest :one
INSERT INTO quest (
    campaign_id, title, description, kind, revealed, giver_npc_id, patron_id, reward_effects, reward_favor,
    failure_effects, failure_favor, loot, dm_notes
) VALUES (
    @campaign_id, @title, @description, @kind, @revealed, sqlc.narg(giver_npc_id), sqlc.narg(patron_id), @reward_effects, @reward_favor,
    @failure_effects, @failure_favor, @loot, @dm_notes
)
RETURNING *;

-- Replaces a quest's record (PUT semantics). Status changes only through Complete, Fail and Reopen,
-- and revealed only through SetQuestRevealed, so a stale edit form can't reveal a hidden quest.
-- name: UpdateQuest :one
UPDATE quest
SET title = @title,
    description = @description,
    kind = @kind,
    giver_npc_id = sqlc.narg(giver_npc_id),
    patron_id = sqlc.narg(patron_id),
    reward_effects = @reward_effects,
    reward_favor = @reward_favor,
    failure_effects = @failure_effects,
    failure_favor = @failure_favor,
    loot = @loot,
    dm_notes = @dm_notes,
    updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetQuestRevealed :one
UPDATE quest SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- The status guard of Complete and Fail (spec §5.6): no row means it was already resolved, so a
-- second click pays nothing.
-- name: ResolveQuest :one
UPDATE quest
SET status = @status, resolved_day = @resolved_day, resolved_at = now(), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'ACTIVE'
RETURNING *;

-- name: ReopenFailedQuest :one
UPDATE quest
SET status = 'ACTIVE', resolved_day = NULL, resolved_at = NULL, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND status = 'FAILED'
RETURNING *;

-- Undo puts a quest back exactly as it was before the change it rewinds.
-- name: RestoreQuestStatus :exec
UPDATE quest
SET status = @status, resolved_day = sqlc.narg(resolved_day), resolved_at = sqlc.narg(resolved_at), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id;

-- name: DeleteQuest :execrows
DELETE FROM quest WHERE campaign_id = @campaign_id AND id = @id;
