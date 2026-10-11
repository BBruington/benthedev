-- Every resource, archived ones included, in board order. The board shows the unarchived ones;
-- rules load them all so history stays correct.
-- name: ListResources :many
SELECT * FROM resource ORDER BY sort_order, key;

-- name: GetResource :one
SELECT * FROM resource WHERE key = $1;

-- name: GetResourceForUpdate :one
SELECT * FROM resource WHERE key = $1 FOR UPDATE;

-- A colony mutation saves each changed amount; the CHECK keeps it within the bounds.
-- name: SetResourceAmount :exec
UPDATE resource SET amount = @amount WHERE key = @key;

-- name: NextResourceSortOrder :one
SELECT (COALESCE(max(sort_order), 0) + 10)::integer FROM resource;

-- name: CreateResource :one
INSERT INTO resource (key, label, icon, amount, min_amount, max_amount, sort_order)
VALUES (@key, @label, @icon, @amount, @min_amount, sqlc.narg(max_amount), @sort_order)
RETURNING *;

-- Replaces a resource's settings (PUT semantics). The amount changes only through colony mutations.
-- name: UpdateResource :one
UPDATE resource
SET label = @label,
    icon = @icon,
    min_amount = @min_amount,
    max_amount = sqlc.narg(max_amount),
    sort_order = @sort_order,
    archived = @archived
WHERE key = @key
RETURNING *;
