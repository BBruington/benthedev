-- +goose Up

-- People and places (spec §4.3, §4.4; docs/design/multi-campaign.md §2.6). Both tables are campaign-owned,
-- and every link between campaign rows is a composite foreign key, so it can never point into another
-- campaign. Portraits (npc.avatar_url), pins (place.map_id, pin_x, pin_y) and npc.first_session_id arrive
-- with the tables they point at (M4, M5).

-- The old app's NPC enums, kept as they were.
CREATE TYPE npc_status AS ENUM ('ALIVE', 'DEAD', 'MISSING', 'UNKNOWN');
CREATE TYPE npc_health AS ENUM ('HEALTHY', 'INJURED', 'SICK', 'UNKNOWN', 'DEAD');
CREATE TYPE npc_relation AS ENUM ('ALLY', 'NEUTRAL', 'SUSPICIOUS', 'ENEMY', 'UNKNOWN');

CREATE TABLE place (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    description text NOT NULL DEFAULT '',
    player_notes text NOT NULL DEFAULT '',
    dm_notes text NOT NULL DEFAULT '',
    -- Players saw every location before, so places start visible (spec §5.9, Open Question 9).
    revealed boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id)
);

CREATE INDEX place_campaign_name_idx ON place (campaign_id, lower(name));

CREATE TABLE npc (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    -- The true name. Players never see it, nor known_name, revealed or dm_notes (Open Question 8).
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    -- What the party calls them; blank when they know the true name.
    known_name text NOT NULL DEFAULT '' CHECK (length(known_name) <= 80),
    -- The one name players get, defined once so every query and index agrees on it.
    display_name text NOT NULL GENERATED ALWAYS AS (CASE WHEN known_name <> '' THEN known_name ELSE name END) STORED,
    -- New NPCs start hidden, as the old app's is_known_to_party did (Open Question 9).
    revealed boolean NOT NULL DEFAULT false,
    aliases text[] NOT NULL DEFAULT '{}',
    labels text[] NOT NULL DEFAULT '{}',
    race text NOT NULL DEFAULT '',
    character_class text NOT NULL DEFAULT '',
    level integer CHECK (level BETWEEN 0 AND 100),
    -- Free text, as before: "mid-40s", "ancient".
    age text NOT NULL DEFAULT '',
    appearance text NOT NULL DEFAULT '',
    personality text NOT NULL DEFAULT '',
    backstory text NOT NULL DEFAULT '',
    player_notes text NOT NULL DEFAULT '',
    dm_notes text NOT NULL DEFAULT '',
    status npc_status NOT NULL DEFAULT 'UNKNOWN',
    health npc_health NOT NULL DEFAULT 'HEALTHY',
    relation npc_relation NOT NULL DEFAULT 'UNKNOWN',
    current_place_id uuid,
    origin_place_id uuid,
    -- Set means the NPC is a named colonist in that role and counts toward it (spec §5.5). Only colony
    -- mutations write it, so undo can restore it.
    colony_role_key text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT npc_current_place_fkey FOREIGN KEY (campaign_id, current_place_id)
        REFERENCES place (campaign_id, id) ON DELETE SET NULL (current_place_id),
    CONSTRAINT npc_origin_place_fkey FOREIGN KEY (campaign_id, origin_place_id)
        REFERENCES place (campaign_id, id) ON DELETE SET NULL (origin_place_id),
    -- Roles are archived, never deleted, so nothing cascades from a role.
    CONSTRAINT npc_colony_role_fkey FOREIGN KEY (campaign_id, colony_role_key) REFERENCES role (campaign_id, key)
);

-- Prefix search for Beny Bot's autocomplete and the roster's order.
CREATE INDEX npc_campaign_display_name_idx ON npc (campaign_id, lower(display_name) text_pattern_ops);
CREATE INDEX npc_campaign_colony_role_idx ON npc (campaign_id, colony_role_key) WHERE colony_role_key IS NOT NULL;
CREATE INDEX npc_campaign_current_place_idx ON npc (campaign_id, current_place_id) WHERE current_place_id IS NOT NULL;
CREATE INDEX npc_campaign_origin_place_idx ON npc (campaign_id, origin_place_id) WHERE origin_place_id IS NOT NULL;

-- A role is archived only when its headcount is 0 (spec §4.2): role_archived_empty covers the unnamed
-- count, and these two triggers cover named colonists from both sides.
-- +goose StatementBegin
CREATE FUNCTION role_archive_keeps_no_named_colonists() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM npc WHERE campaign_id = NEW.campaign_id AND colony_role_key = NEW.key) THEN
        RAISE EXCEPTION 'role % still has named colonists', NEW.key
            USING ERRCODE = 'check_violation', CONSTRAINT = 'role_archived_no_named_colonists';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER role_archived_no_named_colonists BEFORE UPDATE OF archived ON role
    FOR EACH ROW WHEN (NEW.archived) EXECUTE FUNCTION role_archive_keeps_no_named_colonists();

-- +goose StatementBegin
CREATE FUNCTION npc_joins_only_unarchived_roles() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM role WHERE campaign_id = NEW.campaign_id AND key = NEW.colony_role_key AND archived) THEN
        RAISE EXCEPTION 'role % is archived', NEW.colony_role_key
            USING ERRCODE = 'check_violation', CONSTRAINT = 'npc_colony_role_not_archived';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER npc_colony_role_not_archived BEFORE INSERT OR UPDATE OF colony_role_key ON npc
    FOR EACH ROW WHEN (NEW.colony_role_key IS NOT NULL) EXECUTE FUNCTION npc_joins_only_unarchived_roles();

-- The first of timeline_event's link columns (spec §4.3): COLONIST_JOINED, COLONIST_LEFT and
-- WORKFORCE_CHANGED entries name their NPC. Deleting the NPC clears only the link. npc_name is the
-- display name the entry was written with, so core can tell when its text names the NPC by a name the
-- party doesn't know (Invariant 15), even after a rename or the NPC's deletion; '' means none.
ALTER TABLE timeline_event
    ADD COLUMN npc_id uuid,
    ADD COLUMN npc_name text NOT NULL DEFAULT '',
    ADD CONSTRAINT timeline_event_npc_fkey FOREIGN KEY (campaign_id, npc_id)
        REFERENCES npc (campaign_id, id) ON DELETE SET NULL (npc_id);

CREATE INDEX timeline_event_campaign_npc_idx ON timeline_event (campaign_id, npc_id) WHERE npc_id IS NOT NULL;

-- +goose Down

ALTER TABLE timeline_event DROP COLUMN npc_name, DROP COLUMN npc_id;
DROP TRIGGER npc_colony_role_not_archived ON npc;
DROP FUNCTION npc_joins_only_unarchived_roles();
DROP TRIGGER role_archived_no_named_colonists ON role;
DROP FUNCTION role_archive_keeps_no_named_colonists();
DROP TABLE npc;
DROP TABLE place;
DROP TYPE npc_relation;
DROP TYPE npc_status;
DROP TYPE npc_health;
