-- The players' hex map (spec §5.10). Any active member reads and writes it; nothing is hidden on it.

-- name: ListHexes :many
SELECT q, r, terrain, label FROM hex WHERE campaign_id = @campaign_id ORDER BY q, r;

-- name: CountHexes :one
SELECT count(*) FROM hex WHERE campaign_id = @campaign_id;

-- How many of these cells are not charted yet, for the hex limit.
-- name: CountUnchartedHexes :one
SELECT count(*) FROM (SELECT unnest(@qs::integer[]) AS q, unnest(@rs::integer[]) AS r) AS cell
WHERE NOT EXISTS (SELECT 1 FROM hex WHERE hex.campaign_id = @campaign_id AND hex.q = cell.q AND hex.r = cell.r);

-- Painting keeps a hex's label (spec §5.10). terrains is text[] because pgx has no codec for an array
-- of a custom enum.
-- name: PaintHexes :exec
INSERT INTO hex (campaign_id, q, r, terrain)
SELECT @campaign_id, cell.q, cell.r, cell.terrain::terrain
FROM (SELECT unnest(@qs::integer[]) AS q, unnest(@rs::integer[]) AS r, unnest(@terrains::text[]) AS terrain) AS cell
ON CONFLICT (campaign_id, q, r) DO UPDATE SET terrain = excluded.terrain, updated_at = now();

-- name: SetHexLabel :execrows
UPDATE hex SET label = sqlc.narg(label), updated_at = now() WHERE campaign_id = @campaign_id AND q = @q AND r = @r;

-- name: EraseHexes :exec
DELETE FROM hex
WHERE campaign_id = @campaign_id
  AND (q, r) IN (SELECT unnest(@qs::integer[]), unnest(@rs::integer[]));
