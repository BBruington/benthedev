-- Place reads take dm_projection (viewer.SeesDmFields()): players see only revealed places.

-- pin_map_name is NULL when the place has no pin or its map is hidden from this projection, so core
-- never shows a player a pin on a hidden map.
-- name: ListPlaces :many
SELECT sqlc.embed(place), pin_map.name AS pin_map_name
FROM place
LEFT JOIN map AS pin_map ON pin_map.campaign_id = place.campaign_id
    AND pin_map.id = place.map_id AND (@dm_projection::boolean OR pin_map.revealed)
WHERE place.campaign_id = @campaign_id
  AND (@dm_projection::boolean OR place.revealed)
  AND (sqlc.narg(search)::text IS NULL OR place.name ILIKE sqlc.narg(search))
ORDER BY lower(place.name), place.created_at, place.id;

-- name: GetPlaceView :one
SELECT sqlc.embed(place), pin_map.name AS pin_map_name
FROM place
LEFT JOIN map AS pin_map ON pin_map.campaign_id = place.campaign_id
    AND pin_map.id = place.map_id AND (@dm_projection::boolean OR pin_map.revealed)
WHERE place.campaign_id = @campaign_id AND place.id = @id AND (@dm_projection::boolean OR place.revealed);

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

-- Replaces a place's record (PUT semantics); revealed changes only through SetPlaceRevealed.
-- name: UpdatePlace :one
UPDATE place
SET name = @name,
    description = @description,
    player_notes = @player_notes,
    dm_notes = @dm_notes,
    updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetPlaceRevealed :one
UPDATE place SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- NPCs here or from here keep their record; the foreign keys clear only the place column.
-- name: DeletePlace :execrows
DELETE FROM place WHERE campaign_id = @campaign_id AND id = @id;

-- A pin is a fraction of its map's art (0..1, y from the top), set with its map in one write.
-- name: SetPlacePin :one
UPDATE place
SET map_id = @map_id, pin_x = @pin_x, pin_y = @pin_y, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: ClearPlacePin :one
UPDATE place
SET map_id = NULL, pin_x = NULL, pin_y = NULL, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- Before a map is deleted: the foreign key would clear only map_id and leave the fractions behind.
-- name: ClearMapPins :exec
UPDATE place
SET map_id = NULL, pin_x = NULL, pin_y = NULL, updated_at = now()
WHERE campaign_id = @campaign_id AND map_id = @map_id;
