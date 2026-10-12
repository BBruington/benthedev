-- Every role, archived ones included, in display order, with its named NPC colonists counted (all of
-- them, and the revealed ones for the player projection). Colonists = the sum of unnamed and named
-- headcounts (spec §4.2, §5.5).
-- name: ListRoles :many
SELECT sqlc.embed(role),
       (SELECT count(*) FROM npc
        WHERE npc.campaign_id = role.campaign_id AND npc.colony_role_key = role.key)::bigint AS named_count,
       (SELECT count(*) FROM npc
        WHERE npc.campaign_id = role.campaign_id AND npc.colony_role_key = role.key AND npc.revealed)::bigint AS revealed_named_count
FROM role
WHERE role.campaign_id = @campaign_id
ORDER BY role.sort_order, role.key;

-- A colony mutation saves each changed headcount (new colonists land in UNASSIGNED; undo restores).
-- name: SetRoleUnnamedCount :exec
UPDATE role SET unnamed_count = @unnamed_count WHERE campaign_id = @campaign_id AND key = @key;

-- The named colonists the given projection may list, for /people; hidden ones still count (spec §5.5).
-- name: ListNamedColonists :many
SELECT * FROM npc
WHERE campaign_id = @campaign_id
  AND colony_role_key IS NOT NULL
  AND (@dm_projection::boolean OR revealed)
ORDER BY lower(display_name), created_at, id;

-- name: NextRoleSortOrder :one
SELECT (COALESCE(max(sort_order), 0) + 10)::integer FROM role WHERE campaign_id = @campaign_id;

-- name: CreateRole :one
INSERT INTO role (campaign_id, key, label, icon, sort_order)
VALUES (@campaign_id, @key, @label, @icon, @sort_order)
RETURNING *;

-- Replaces a role's settings (PUT semantics). Headcounts change only through colony mutations; the
-- schema refuses archiving a role anyone is in.
-- name: UpdateRole :one
UPDATE role
SET label = @label,
    icon = @icon,
    sort_order = @sort_order,
    archived = @archived
WHERE campaign_id = @campaign_id AND key = @key
RETURNING *;
