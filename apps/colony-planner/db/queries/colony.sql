-- Every colony mutation starts here: the campaign's colony row is always locked first, so there is
-- one lock order (spec §5.1), and campaigns never wait on each other.
-- name: GetColonyForUpdate :one
SELECT * FROM colony WHERE campaign_id = @campaign_id FOR UPDATE;

-- name: GetColony :one
SELECT * FROM colony WHERE campaign_id = @campaign_id;

-- Saves a mutation's clock, bumps the version and records the undo slot (NULL clears it, as undo
-- does). Resource amounts and role headcounts are saved by their own queries in the same transaction.
-- name: SaveColonyChange :one
UPDATE colony
SET day = @day,
    shipment_interval_days = sqlc.narg(shipment_interval_days),
    next_shipment_day = sqlc.narg(next_shipment_day),
    last_change = sqlc.narg(last_change),
    version = version + 1,
    updated_at = now()
WHERE campaign_id = @campaign_id
RETURNING *;

-- The colony's name is a setting, not a colony mutation: it leaves the version and the undo slot alone.
-- name: SetColonyName :one
UPDATE colony SET name = @name, updated_at = now() WHERE campaign_id = @campaign_id RETURNING *;
