-- Player projection: PUBLIC entries that have not been undone. core passes viewer.SeesDmFields() as
-- dm_projection (never IsDM), so the DM viewing as player gets the player projection (spec §2.2).

-- name: ListTimelineEvents :many
SELECT * FROM timeline_event
WHERE campaign_id = @campaign_id
  AND day BETWEEN @from_day AND @to_day
  AND (@dm_projection::boolean OR (visibility = 'PUBLIC' AND undone_at IS NULL))
ORDER BY day, created_at, sequence;

-- The newest entries in the order they were written, for the board and the live toasts.
-- name: ListRecentTimelineEvents :many
SELECT * FROM timeline_event
WHERE campaign_id = @campaign_id
  AND (@dm_projection::boolean OR (visibility = 'PUBLIC' AND undone_at IS NULL))
ORDER BY sequence DESC
LIMIT @entry_limit;

-- The nearest day before a page that has an entry this viewer may see, so paging skips empty weeks.
-- name: PreviousTimelineDay :one
SELECT day FROM timeline_event
WHERE campaign_id = @campaign_id
  AND day < @before_day
  AND (@dm_projection::boolean OR (visibility = 'PUBLIC' AND undone_at IS NULL))
ORDER BY day DESC
LIMIT 1;

-- name: NextTimelineDay :one
SELECT day FROM timeline_event
WHERE campaign_id = @campaign_id
  AND day > @after_day
  AND (@dm_projection::boolean OR (visibility = 'PUBLIC' AND undone_at IS NULL))
ORDER BY day
LIMIT 1;

-- DM reads only (edits, record cargo): no projection. An ID from a request body or from jsonb is
-- loaded here with the viewer's campaign, so another campaign's entry is simply not found.
-- name: GetTimelineEvent :one
SELECT * FROM timeline_event WHERE campaign_id = @campaign_id AND id = @id;

-- created_at is read when the row is written, not when its transaction began (now()). Every insert
-- runs under the colony lock, so entries then carry their commit order, and the live toasts can
-- tell news from older entries that slid into the recent list (web/src/lib/colony.ts).
-- name: CreateTimelineEvent :one
INSERT INTO timeline_event (campaign_id, group_id, day, kind, title, body, visibility, deltas, data, created_by, created_at)
VALUES (@campaign_id, sqlc.narg(group_id), @day, @kind, @title, @body, @visibility, @deltas, @data, sqlc.narg(created_by), clock_timestamp())
RETURNING *;

-- The DM edits any entry's text and visibility; only a CUSTOM entry may move to another day (core
-- checks). Editing never reverses effects (spec §5.8).
-- name: UpdateTimelineEvent :one
UPDATE timeline_event
SET day = @day,
    title = @title,
    body = @body,
    visibility = @visibility,
    edited_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- Auto-logged entries are never deleted; only CUSTOM ones (spec §5.8).
-- name: DeleteCustomTimelineEvent :execrows
DELETE FROM timeline_event WHERE campaign_id = @campaign_id AND id = @id AND kind = 'CUSTOM';

-- Undo keeps the entries and marks them (spec §5.1). The group comes from colony.last_change.
-- name: MarkTimelineGroupUndone :many
UPDATE timeline_event
SET undone_at = now()
WHERE campaign_id = @campaign_id AND group_id = @group_id::uuid AND undone_at IS NULL
RETURNING *;
