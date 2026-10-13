-- Map reads take dm_projection (viewer.SeesDmFields()): players see only revealed maps, and on a map
-- only the pins of revealed places.

-- name: ListMaps :many
SELECT * FROM map
WHERE campaign_id = @campaign_id AND (@dm_projection::boolean OR revealed)
ORDER BY sort_order, lower(name), created_at, id;

-- name: GetMapView :one
SELECT * FROM map WHERE campaign_id = @campaign_id AND id = @id AND (@dm_projection::boolean OR revealed);

-- DM reads and writes. A map ID from a request body (a pin's map) is loaded here with the viewer's
-- campaign, so another campaign's map is simply not found.
-- name: GetMap :one
SELECT * FROM map WHERE campaign_id = @campaign_id AND id = @id;

-- name: ListMapIDs :many
SELECT id FROM map WHERE campaign_id = @campaign_id ORDER BY id;

-- name: CountMaps :one
SELECT count(*) FROM map WHERE campaign_id = @campaign_id;

-- A new map goes to the end of the list.
-- name: CreateMap :one
INSERT INTO map (campaign_id, name, image_id, external_url, width, height, revealed, sort_order)
VALUES (
    @campaign_id, @name, sqlc.narg(image_id), sqlc.narg(external_url), @width, @height, @revealed,
    (SELECT coalesce(max(existing.sort_order) + 1, 0) FROM map AS existing WHERE existing.campaign_id = @campaign_id)
)
RETURNING *;

-- Edits only the name: visibility, the Mainland flag, the order and the art each have their own switch.
-- name: RenameMap :one
UPDATE map SET name = @name, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- Replaces the art; pins are fractions of it, so they stay where they were.
-- name: SetMapArt :one
UPDATE map
SET image_id = sqlc.narg(image_id), external_url = sqlc.narg(external_url), width = @width, height = @height, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetMapRevealed :one
UPDATE map SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- At most one Mainland per campaign (map_one_mainland): core clears the old one first.
-- name: ClearMainland :exec
UPDATE map SET is_mainland = false, updated_at = now() WHERE campaign_id = @campaign_id AND is_mainland;

-- name: SetMapMainland :one
UPDATE map SET is_mainland = @is_mainland, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- name: SetMapSortOrder :exec
UPDATE map SET sort_order = @sort_order WHERE campaign_id = @campaign_id AND id = @id;

-- name: DeleteMap :one
DELETE FROM map WHERE campaign_id = @campaign_id AND id = @id RETURNING image_id;

-- The pins on one map, as the projection sees them.
-- name: ListMapPins :many
SELECT id, name, pin_x::real AS pin_x, pin_y::real AS pin_y FROM place
WHERE campaign_id = @campaign_id
  AND map_id = @map_id
  AND pin_x IS NOT NULL
  AND (@dm_projection::boolean OR revealed)
ORDER BY lower(name), id;
