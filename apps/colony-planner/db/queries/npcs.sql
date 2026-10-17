-- NPC reads take dm_projection, which core sets from viewer.SeesDmFields() (never IsDM): the player
-- projection sees only revealed NPCs, and only revealed places beside them (spec §5.9). The joined
-- place names are NULL for a place this projection can't see, so core never names it.

-- The roster, with search and filters (each NULL filter is off). search is an ILIKE pattern core
-- builds and escapes; players match display_name and aliases, the DM also the true and known names.
-- display_name_pattern is a LIKE pattern over lower(display_name), for Beny Bot's prefix and exact
-- lookups.
-- name: ListNpcs :many
SELECT sqlc.embed(npc),
       current_place.name AS current_place_name,
       origin_place.name AS origin_place_name,
       role.label AS colony_role_label,
       role.icon AS colony_role_icon,
       first_session.title AS first_session_title,
       first_session.local_date AS first_session_local_date,
       first_session.starts_at AS first_session_starts_at
FROM npc
LEFT JOIN session AS first_session ON first_session.campaign_id = npc.campaign_id AND first_session.id = npc.first_session_id
LEFT JOIN place AS current_place ON current_place.campaign_id = npc.campaign_id
    AND current_place.id = npc.current_place_id AND (@dm_projection::boolean OR current_place.revealed)
LEFT JOIN place AS origin_place ON origin_place.campaign_id = npc.campaign_id
    AND origin_place.id = npc.origin_place_id AND (@dm_projection::boolean OR origin_place.revealed)
LEFT JOIN role ON role.campaign_id = npc.campaign_id AND role.key = npc.colony_role_key
WHERE npc.campaign_id = @campaign_id
  AND (@dm_projection::boolean OR npc.revealed)
  AND (sqlc.narg(search)::text IS NULL
       OR npc.display_name ILIKE sqlc.narg(search)
       OR EXISTS (SELECT 1 FROM unnest(npc.aliases) AS alias WHERE alias ILIKE sqlc.narg(search))
       OR (@dm_projection::boolean AND (npc.name ILIKE sqlc.narg(search) OR npc.known_name ILIKE sqlc.narg(search))))
  AND (sqlc.narg(display_name_pattern)::text IS NULL OR lower(npc.display_name) LIKE sqlc.narg(display_name_pattern))
  AND (sqlc.narg(status)::npc_status IS NULL OR npc.status = sqlc.narg(status))
  AND (sqlc.narg(health)::npc_health IS NULL OR npc.health = sqlc.narg(health))
  AND (sqlc.narg(relation)::npc_relation IS NULL OR npc.relation = sqlc.narg(relation))
  AND (sqlc.narg(label)::text IS NULL OR sqlc.narg(label) = ANY (npc.labels))
  AND (sqlc.narg(colony_role_key)::text IS NULL OR npc.colony_role_key = sqlc.narg(colony_role_key))
  AND (NOT @colonists_only::boolean OR npc.colony_role_key IS NOT NULL)
  AND (sqlc.narg(place_id)::uuid IS NULL OR current_place.id = sqlc.narg(place_id))
  AND (sqlc.narg(revealed)::boolean IS NULL OR npc.revealed = sqlc.narg(revealed))
ORDER BY lower(npc.display_name), npc.created_at, npc.id
LIMIT @result_limit;

-- One NPC as the given projection sees it.
-- name: GetNpcView :one
SELECT sqlc.embed(npc),
       current_place.name AS current_place_name,
       origin_place.name AS origin_place_name,
       role.label AS colony_role_label,
       role.icon AS colony_role_icon,
       first_session.title AS first_session_title,
       first_session.local_date AS first_session_local_date,
       first_session.starts_at AS first_session_starts_at
FROM npc
LEFT JOIN session AS first_session ON first_session.campaign_id = npc.campaign_id AND first_session.id = npc.first_session_id
LEFT JOIN place AS current_place ON current_place.campaign_id = npc.campaign_id
    AND current_place.id = npc.current_place_id AND (@dm_projection::boolean OR current_place.revealed)
LEFT JOIN place AS origin_place ON origin_place.campaign_id = npc.campaign_id
    AND origin_place.id = npc.origin_place_id AND (@dm_projection::boolean OR origin_place.revealed)
LEFT JOIN role ON role.campaign_id = npc.campaign_id AND role.key = npc.colony_role_key
WHERE npc.campaign_id = @campaign_id
  AND npc.id = @id
  AND (@dm_projection::boolean OR npc.revealed);

-- DM reads and colony mutations. An ID from a request body or the undo slot is loaded here with the
-- viewer's campaign, so another campaign's NPC is simply not found.
-- name: GetNpc :one
SELECT * FROM npc WHERE campaign_id = @campaign_id AND id = @id;

-- name: CountNpcs :one
SELECT count(*) FROM npc WHERE campaign_id = @campaign_id;

-- name: CreateNpc :one
INSERT INTO npc (
    campaign_id, name, known_name, revealed, aliases, labels, race, character_class, level, age,
    appearance, personality, backstory, player_notes, dm_notes, status, health, relation,
    current_place_id, origin_place_id
) VALUES (
    @campaign_id, @name, @known_name, @revealed, @aliases, @labels, @race, @character_class, sqlc.narg(level), @age,
    @appearance, @personality, @backstory, @player_notes, @dm_notes, @status, @health, @relation,
    sqlc.narg(current_place_id), sqlc.narg(origin_place_id)
)
RETURNING *;

-- Replaces an NPC's record (PUT semantics). colony_role_key changes only through colony mutations (spec
-- §5.5) and revealed only through SetNpcRevealed, so a stale edit form can't reveal a hidden NPC.
-- name: UpdateNpc :one
UPDATE npc
SET name = @name,
    known_name = @known_name,
    aliases = @aliases,
    labels = @labels,
    race = @race,
    character_class = @character_class,
    level = sqlc.narg(level),
    age = @age,
    appearance = @appearance,
    personality = @personality,
    backstory = @backstory,
    player_notes = @player_notes,
    dm_notes = @dm_notes,
    status = @status,
    health = @health,
    relation = @relation,
    current_place_id = sqlc.narg(current_place_id),
    origin_place_id = sqlc.narg(origin_place_id),
    updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id
RETURNING *;

-- name: SetNpcRevealed :one
UPDATE npc SET revealed = @revealed, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- A colony mutation (join, change role, leave) or its undo; NULL means not a colonist.
-- name: SetNpcColonyRole :exec
UPDATE npc
SET colony_role_key = sqlc.narg(colony_role_key), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id;

-- A colonist can't be deleted until they leave the colony (spec §5.5); core checks first and says so.
-- name: DeleteNpc :execrows
DELETE FROM npc WHERE campaign_id = @campaign_id AND id = @id AND colony_role_key IS NULL;

-- A portrait is one of this campaign's images; NULL clears it. Returns the portrait it replaced, so core
-- can delete that image once nothing uses it.
-- name: SetNpcPortrait :one
UPDATE npc AS updated
SET portrait_image_id = sqlc.narg(portrait_image_id), updated_at = now()
FROM npc AS previous
WHERE updated.campaign_id = @campaign_id AND updated.id = @id
  AND previous.campaign_id = updated.campaign_id AND previous.id = updated.id
RETURNING previous.portrait_image_id AS previous_portrait_image_id;
