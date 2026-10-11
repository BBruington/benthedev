-- Every role, archived ones included, in display order. Colonists = the sum of the headcounts (spec
-- §4.2); from M2 the named NPC colonists of each role are added by their own query.
-- name: ListRoles :many
SELECT * FROM role WHERE campaign_id = @campaign_id ORDER BY sort_order, key;

-- A colony mutation saves each changed headcount (new colonists land in UNASSIGNED; undo restores).
-- name: SetRoleUnnamedCount :exec
UPDATE role SET unnamed_count = @unnamed_count WHERE campaign_id = @campaign_id AND key = @key;
