-- Every resource, archived ones included, in board order. The board shows the unarchived ones;
-- rules load them all so history stays correct.
-- name: ListResources :many
SELECT * FROM resource WHERE campaign_id = @campaign_id ORDER BY sort_order, key;

-- A colony mutation saves each changed amount; the CHECK keeps it within the bounds.
-- name: SetResourceAmount :exec
UPDATE resource SET amount = @amount WHERE campaign_id = @campaign_id AND key = @key;

-- name: NextResourceSortOrder :one
SELECT (COALESCE(max(sort_order), 0) + 10)::integer FROM resource WHERE campaign_id = @campaign_id;

-- name: CreateResource :one
INSERT INTO resource (campaign_id, key, label, icon, amount, min_amount, max_amount, sort_order)
VALUES (@campaign_id, @key, @label, @icon, @amount, @min_amount, sqlc.narg(max_amount), @sort_order)
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
WHERE campaign_id = @campaign_id AND key = @key
RETURNING *;
