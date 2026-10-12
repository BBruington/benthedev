-- Patron and action reads take dm_projection (viewer.SeesDmFields()): players see only revealed,
-- unarchived patrons, and only revealed, unarchived actions of those (spec §5.7, §5.9).

-- name: ListPatrons :many
SELECT * FROM patron
WHERE campaign_id = @campaign_id
  AND (@dm_projection::boolean OR (revealed AND NOT archived))
ORDER BY archived, sort_order, lower(name), id;

-- An archived patron leaves the players' list but keeps its page, so history that links to it still works.
-- name: GetPatronView :one
SELECT * FROM patron
WHERE campaign_id = @campaign_id AND id = @id
  AND (@dm_projection::boolean OR revealed);

-- DM reads: an ID from a request body is loaded with the viewer's campaign.
-- name: GetPatron :one
SELECT * FROM patron WHERE campaign_id = @campaign_id AND id = @id;

-- Every favor change locks the patron after the colony (spec §5.7: colony first, then the patron).
-- NO KEY: favor is not a key, so quest edits, request inserts and other rows whose foreign keys name
-- this patron (KEY SHARE) neither wait for it nor deadlock with undo, which locks patrons before quests.
-- name: GetPatronForUpdate :one
SELECT * FROM patron WHERE campaign_id = @campaign_id AND id = @id FOR NO KEY UPDATE;

-- Patrons are never deleted, so archiving is the only way under PatronLimit; the total still has a ceiling.
-- name: CountPatrons :one
SELECT count(*) FILTER (WHERE NOT archived) AS unarchived, count(*) AS total
FROM patron WHERE campaign_id = @campaign_id;

-- name: CreatePatron :one
INSERT INTO patron (campaign_id, name, description, revealed, player_notes, dm_notes, sort_order)
VALUES (@campaign_id, @name, @description, @revealed, @player_notes, @dm_notes, @sort_order)
RETURNING *;

-- PUT semantics. Favor changes only through colony mutations, revealed only through SetPatronRevealed
-- and archived only through SetPatronArchived.
-- name: UpdatePatron :one
UPDATE patron
SET name = @name, description = @description, player_notes = @player_notes, dm_notes = @dm_notes,
    sort_order = @sort_order, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetPatronRevealed :one
UPDATE patron SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- name: SetPatronArchived :one
UPDATE patron SET archived = @archived, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- A colony mutation (complete, fail, approve, adjust) or its undo.
-- name: SetPatronFavor :exec
UPDATE patron SET favor = @favor, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id;

-- name: ListPatronActions :many
SELECT patron_action.* FROM patron_action
JOIN patron ON patron.campaign_id = patron_action.campaign_id AND patron.id = patron_action.patron_id
WHERE patron_action.campaign_id = @campaign_id
  AND (sqlc.narg(patron_id)::uuid IS NULL OR patron_action.patron_id = sqlc.narg(patron_id))
  AND (@dm_projection::boolean OR (patron_action.revealed AND NOT patron_action.archived AND patron.revealed AND NOT patron.archived))
ORDER BY patron_action.archived, patron_action.sort_order, lower(patron_action.name), patron_action.id;

-- name: GetPatronAction :one
SELECT * FROM patron_action WHERE campaign_id = @campaign_id AND id = @id;

-- The same two counts, for the same reason.
-- name: CountPatronActions :one
SELECT count(*) FILTER (WHERE NOT archived) AS unarchived, count(*) AS total
FROM patron_action WHERE campaign_id = @campaign_id;

-- name: CreatePatronAction :one
INSERT INTO patron_action (campaign_id, patron_id, name, description, favor_cost, effects, revealed, sort_order)
VALUES (@campaign_id, @patron_id, @name, @description, @favor_cost, @effects, @revealed, @sort_order)
RETURNING *;

-- name: UpdatePatronAction :one
UPDATE patron_action
SET name = @name, description = @description, favor_cost = @favor_cost, effects = @effects,
    sort_order = @sort_order, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetPatronActionRevealed :one
UPDATE patron_action SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- name: SetPatronActionArchived :one
UPDATE patron_action SET archived = @archived, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;
