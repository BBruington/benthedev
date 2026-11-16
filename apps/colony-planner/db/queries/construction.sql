-- Construction (docs/design/construction.md). Blueprint and building reads take dm_projection
-- (viewer.SeesDmFields()): players see only revealed, unarchived blueprints, and the buildings of
-- revealed ones in every status (K16).

-- name: ListBlueprints :many
SELECT * FROM building_blueprint
WHERE campaign_id = @campaign_id
  AND (@dm_projection::boolean OR (revealed AND NOT archived))
ORDER BY archived, sort_order, lower(name), id;

-- Every blueprint's cost and labor lines; core keeps the ones of the blueprints it lists.
-- name: ListBlueprintCosts :many
SELECT * FROM building_blueprint_cost WHERE campaign_id = @campaign_id ORDER BY blueprint_id, resource_key;

-- name: ListBlueprintLabor :many
SELECT * FROM building_blueprint_labor WHERE campaign_id = @campaign_id ORDER BY blueprint_id, role_key;

-- An ID from a path or a request body is loaded with the viewer's campaign; core hides a hidden one
-- from players.
-- name: GetBlueprint :one
SELECT * FROM building_blueprint WHERE campaign_id = @campaign_id AND id = @id;

-- Blueprints are never deleted, so the count is a lifetime ceiling (BlueprintLimit).
-- name: CountBlueprints :one
SELECT count(*) FROM building_blueprint WHERE campaign_id = @campaign_id;

-- name: BlueprintNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM building_blueprint
    WHERE campaign_id = @campaign_id AND lower(name) = lower(@name::text) AND id <> @except_id::uuid
) AS taken;

-- name: CreateBlueprint :one
INSERT INTO building_blueprint (campaign_id, name, description, dm_notes, revealed, completion_effects, sort_order)
VALUES (@campaign_id, @name, @description, @dm_notes, @revealed, @completion_effects, @sort_order)
RETURNING *;

-- PUT semantics. revealed changes only through SetBlueprintRevealed and archived only through
-- SetBlueprintArchived; the cost and labor rows are replaced beside it.
-- name: UpdateBlueprint :one
UPDATE building_blueprint
SET name = @name, description = @description, dm_notes = @dm_notes, completion_effects = @completion_effects,
    sort_order = @sort_order, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetBlueprintRevealed :one
UPDATE building_blueprint SET revealed = @revealed, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetBlueprintArchived :one
UPDATE building_blueprint SET archived = @archived, updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: DeleteBlueprintCosts :exec
DELETE FROM building_blueprint_cost WHERE campaign_id = @campaign_id AND blueprint_id = @blueprint_id;

-- name: CreateBlueprintCost :exec
INSERT INTO building_blueprint_cost (campaign_id, blueprint_id, resource_key, amount)
VALUES (@campaign_id, @blueprint_id, @resource_key, @amount);

-- name: DeleteBlueprintLabor :exec
DELETE FROM building_blueprint_labor WHERE campaign_id = @campaign_id AND blueprint_id = @blueprint_id;

-- name: CreateBlueprintLabor :exec
INSERT INTO building_blueprint_labor (campaign_id, blueprint_id, role_key, effort, max_crew)
VALUES (@campaign_id, @blueprint_id, @role_key, @effort, sqlc.narg(max_crew));

-- Buildings with their blueprint's live name and visibility, oldest first.
-- name: ListBuildings :many
SELECT sqlc.embed(building), building_blueprint.name AS blueprint_name, building_blueprint.revealed AS blueprint_revealed
FROM building
JOIN building_blueprint ON building_blueprint.campaign_id = building.campaign_id AND building_blueprint.id = building.blueprint_id
WHERE building.campaign_id = @campaign_id
  AND building_blueprint.campaign_id = @campaign_id
  AND (@dm_projection::boolean OR building_blueprint.revealed)
ORDER BY building.created_at, building.id;

-- name: ListBuildingLabor :many
SELECT * FROM building_labor WHERE campaign_id = @campaign_id ORDER BY building_id, role_key;

-- name: ListLaborOfBuilding :many
SELECT * FROM building_labor WHERE campaign_id = @campaign_id AND building_id = @building_id ORDER BY role_key;

-- The unfinished projects' labor lines, which game.State carries (readColony).
-- name: ListUnfinishedBuildingLabor :many
SELECT sqlc.embed(building_labor), building.completion_effects
FROM building_labor
JOIN building ON building.campaign_id = building_labor.campaign_id AND building.id = building_labor.building_id
WHERE building_labor.campaign_id = @campaign_id
  AND building.campaign_id = @campaign_id
  AND building.status = 'CONSTRUCTING'
ORDER BY building_labor.building_id, building_labor.role_key;

-- An ID from a path or the undo slot is loaded with the viewer's campaign; core hides a building of a
-- hidden blueprint from players.
-- name: GetBuilding :one
SELECT sqlc.embed(building), building_blueprint.name AS blueprint_name, building_blueprint.revealed AS blueprint_revealed
FROM building
JOIN building_blueprint ON building_blueprint.campaign_id = building.campaign_id AND building_blueprint.id = building.blueprint_id
WHERE building.campaign_id = @campaign_id
  AND building_blueprint.campaign_id = @campaign_id
  AND building.id = @id;

-- name: CountUnfinishedBuildings :one
SELECT count(*) FROM building WHERE campaign_id = @campaign_id AND status = 'CONSTRUCTING';

-- The ID comes from core, which gives it to the rules before the row exists.
-- name: CreateBuilding :one
INSERT INTO building (campaign_id, id, blueprint_id, cost, completion_effects, started_day, started_by)
VALUES (@campaign_id, @id, @blueprint_id, @cost, @completion_effects, @started_day, sqlc.narg(started_by))
RETURNING *;

-- name: CreateBuildingLabor :exec
INSERT INTO building_labor (campaign_id, building_id, role_key, effort, max_crew, progress, crew)
VALUES (@campaign_id, @building_id, @role_key, @effort, sqlc.narg(max_crew), 0, @crew);

-- name: SetBuildingLabor :exec
UPDATE building_labor SET progress = @progress, crew = @crew
WHERE campaign_id = @campaign_id AND building_id = @building_id AND role_key = @role_key;

-- Finishing, cancelling, demolishing and their undo all set the status with its days and refund.
-- name: SetBuildingStatus :exec
UPDATE building
SET status = @status, finished_day = sqlc.narg(finished_day), ended_day = sqlc.narg(ended_day),
    refund = sqlc.narg(refund), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id;

-- Undoing a Start deletes the building it made; its labor cascades and timeline links clear.
-- name: DeleteBuilding :exec
DELETE FROM building WHERE campaign_id = @campaign_id AND id = @id;

-- Renaming a hidden blueprint moves its buildings' PUBLIC entries to DM, so a later reveal can't show
-- players the name they were written with.
-- name: HideBlueprintTimelineEvents :exec
UPDATE timeline_event SET visibility = 'DM'
WHERE timeline_event.campaign_id = @campaign_id
  AND timeline_event.visibility = 'PUBLIC'
  AND timeline_event.building_id IN (SELECT building.id FROM building WHERE building.campaign_id = @campaign_id AND building.blueprint_id = @blueprint_id);

-- A role can't be archived while an unfinished building's labor names it.
-- name: RoleHasUnfinishedLabor :one
SELECT EXISTS (
    SELECT 1 FROM building_labor
    JOIN building ON building.campaign_id = building_labor.campaign_id AND building.id = building_labor.building_id
    WHERE building_labor.campaign_id = @campaign_id
      AND building.campaign_id = @campaign_id
      AND building_labor.role_key = @role_key
      AND building.status = 'CONSTRUCTING'
) AS named;
