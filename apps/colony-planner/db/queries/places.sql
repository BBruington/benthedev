-- Place reads take dm_projection (viewer.SeesDmFields()): players see only revealed places.

-- name: ListPlaces :many
SELECT * FROM place
WHERE campaign_id = @campaign_id
  AND (@dm_projection::boolean OR revealed)
  AND (sqlc.narg(search)::text IS NULL OR name ILIKE sqlc.narg(search))
ORDER BY lower(name), created_at, id;

-- name: GetPlaceView :one
SELECT * FROM place WHERE campaign_id = @campaign_id AND id = @id AND (@dm_projection::boolean OR revealed);

-- DM reads. A place ID from a request body (an NPC's current or origin place) is loaded here with the
-- viewer's campaign, so another campaign's place is simply not found.
-- name: GetPlace :one
SELECT * FROM place WHERE campaign_id = @campaign_id AND id = @id;

-- name: CountPlaces :one
SELECT count(*) FROM place WHERE campaign_id = @campaign_id;

-- name: CreatePlace :one
INSERT INTO place (campaign_id, name, description, player_notes, dm_notes, revealed)
VALUES (@campaign_id, @name, @description, @player_notes, @dm_notes, @revealed)
RETURNING *;

-- Replaces a place's record (PUT semantics).
-- name: UpdatePlace :one
UPDATE place
SET name = @name,
    description = @description,
    player_notes = @player_notes,
    dm_notes = @dm_notes,
    revealed = @revealed,
    updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetPlaceRevealed :one
UPDATE place SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- NPCs here or from here keep their record; the foreign keys clear only the place column.
-- name: DeletePlace :execrows
DELETE FROM place WHERE campaign_id = @campaign_id AND id = @id;
