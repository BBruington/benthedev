-- +goose Up

-- Construction, Phase 1 (docs/design/construction.md §11): the DM's catalog of blueprints and the
-- buildings raised from them, one row through each one's life. Blueprints are archived, never deleted,
-- so a building's link to its blueprint is NO ACTION, as patrons' are (0003). A started building
-- snapshots its blueprint's cost, labor and completion Effects (K18).

CREATE TYPE building_status AS ENUM ('CONSTRUCTING', 'FINISHED', 'CANCELLED', 'DEMOLISHED');

CREATE TABLE building_blueprint (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    description text NOT NULL DEFAULT '',
    dm_notes text NOT NULL DEFAULT '',
    -- Blueprints start hidden, like patron actions (K16).
    revealed boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    -- Effects JSON applied once when a building of it finishes; core checks its keys.
    completion_effects jsonb NOT NULL DEFAULT '{}',
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id)
);

CREATE UNIQUE INDEX building_blueprint_campaign_name_idx ON building_blueprint (campaign_id, lower(name));
CREATE INDEX building_blueprint_campaign_sort_idx ON building_blueprint (campaign_id, archived, sort_order);

CREATE TABLE building_blueprint_cost (
    blueprint_id uuid NOT NULL,
    resource_key text NOT NULL,
    amount integer NOT NULL CHECK (amount > 0),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, blueprint_id, resource_key),
    CONSTRAINT building_blueprint_cost_blueprint_fkey FOREIGN KEY (campaign_id, blueprint_id)
        REFERENCES building_blueprint (campaign_id, id) ON DELETE CASCADE,
    CONSTRAINT building_blueprint_cost_resource_fkey FOREIGN KEY (campaign_id, resource_key) REFERENCES resource (campaign_id, key)
);

CREATE INDEX building_blueprint_cost_campaign_resource_idx ON building_blueprint_cost (campaign_id, resource_key);

-- At least one line per blueprint; core checks it, since a CHECK can't see other rows.
CREATE TABLE building_blueprint_labor (
    blueprint_id uuid NOT NULL,
    role_key text NOT NULL,
    effort integer NOT NULL CHECK (effort > 0),
    -- NULL for no cap (K9).
    max_crew integer CHECK (max_crew > 0),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, blueprint_id, role_key),
    CONSTRAINT building_blueprint_labor_blueprint_fkey FOREIGN KEY (campaign_id, blueprint_id)
        REFERENCES building_blueprint (campaign_id, id) ON DELETE CASCADE,
    CONSTRAINT building_blueprint_labor_role_fkey FOREIGN KEY (campaign_id, role_key) REFERENCES role (campaign_id, key)
);

CREATE INDEX building_blueprint_labor_campaign_role_idx ON building_blueprint_labor (campaign_id, role_key);

CREATE TABLE building (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    blueprint_id uuid NOT NULL,
    status building_status NOT NULL DEFAULT 'CONSTRUCTING',
    -- The Effects Start applied (the cost, negated) and the completion Effects, both snapshots.
    cost jsonb NOT NULL DEFAULT '{}',
    completion_effects jsonb NOT NULL DEFAULT '{}',
    started_day integer NOT NULL CHECK (started_day >= 0),
    finished_day integer,
    -- Cancel or demolish: the day, and the Effects the refund applied.
    ended_day integer,
    refund jsonb,
    started_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT building_finished_whole CHECK ((status IN ('FINISHED', 'DEMOLISHED')) = (finished_day IS NOT NULL)),
    CONSTRAINT building_ended_whole CHECK (
        (status IN ('CANCELLED', 'DEMOLISHED')) = (ended_day IS NOT NULL)
        AND (ended_day IS NULL) = (refund IS NULL)),
    CONSTRAINT building_blueprint_fkey FOREIGN KEY (campaign_id, blueprint_id) REFERENCES building_blueprint (campaign_id, id),
    CONSTRAINT building_started_by_fkey FOREIGN KEY (campaign_id, started_by) REFERENCES campaign_member (campaign_id, id)
);

CREATE INDEX building_campaign_blueprint_idx ON building (campaign_id, blueprint_id);
CREATE INDEX building_campaign_constructing_idx ON building (campaign_id, created_at) WHERE status = 'CONSTRUCTING';

-- A snapshot of the blueprint's labor plus the live progress and crew (K6). A finished line's crew is 0.
CREATE TABLE building_labor (
    building_id uuid NOT NULL,
    role_key text NOT NULL,
    effort integer NOT NULL CHECK (effort > 0),
    max_crew integer CHECK (max_crew > 0),
    progress integer NOT NULL DEFAULT 0,
    crew integer NOT NULL DEFAULT 0,
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, building_id, role_key),
    CONSTRAINT building_labor_progress_range CHECK (progress BETWEEN 0 AND effort),
    CONSTRAINT building_labor_crew_range CHECK (crew >= 0 AND (max_crew IS NULL OR crew <= max_crew)),
    CONSTRAINT building_labor_done_uncrewed CHECK (progress < effort OR crew = 0),
    CONSTRAINT building_labor_building_fkey FOREIGN KEY (campaign_id, building_id)
        REFERENCES building (campaign_id, id) ON DELETE CASCADE,
    CONSTRAINT building_labor_role_fkey FOREIGN KEY (campaign_id, role_key) REFERENCES role (campaign_id, key)
);

CREATE INDEX building_labor_campaign_role_idx ON building_labor (campaign_id, role_key);

-- The role a new blueprint's first labor line uses; NULL for none.
ALTER TABLE settings
    ADD COLUMN construction_role_key text,
    ADD CONSTRAINT settings_construction_role_fkey FOREIGN KEY (campaign_id, construction_role_key) REFERENCES role (campaign_id, key);

UPDATE settings SET construction_role_key = 'BUILDER'
WHERE EXISTS (SELECT 1 FROM role WHERE role.campaign_id = settings.campaign_id AND role.key = 'BUILDER' AND NOT role.archived);

-- Undoing a Start deletes its building, which clears only the link, as session_id does (0005).
ALTER TABLE timeline_event
    ADD COLUMN building_id uuid,
    ADD CONSTRAINT timeline_event_building_fkey FOREIGN KEY (campaign_id, building_id)
        REFERENCES building (campaign_id, id) ON DELETE SET NULL (building_id);

CREATE INDEX timeline_event_campaign_building_idx ON timeline_event (campaign_id, building_id) WHERE building_id IS NOT NULL;

-- +goose Down

ALTER TABLE timeline_event DROP COLUMN building_id;
ALTER TABLE settings DROP COLUMN construction_role_key;
DROP TABLE building_labor;
DROP TABLE building;
DROP TABLE building_blueprint_labor;
DROP TABLE building_blueprint_cost;
DROP TABLE building_blueprint;
DROP TYPE building_status;
