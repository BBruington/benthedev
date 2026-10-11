-- +goose Up

-- M1: the colony clock and books (spec §4.2) and the timeline (spec §4.3, §5.8). Link columns from
-- timeline_event to quest, patron, request, npc and session arrive with those tables.

-- The curated lucide icons a resource or role may show; the web maps each name to its component.
CREATE TYPE colony_icon AS ENUM (
    'Anchor', 'Apple', 'Axe', 'Beef', 'Beer', 'Bone', 'BookOpen', 'Boxes', 'Coins', 'Crown',
    'Droplets', 'Feather', 'Fish', 'Flame', 'FlaskConical', 'Gem', 'Hammer', 'HardHat', 'Heart',
    'HeartHandshake', 'HeartPulse', 'Leaf', 'Mountain', 'Package', 'Pickaxe', 'Scroll', 'Shield',
    'Ship', 'Shirt', 'Sparkles', 'Star', 'Sword', 'Tent', 'Trees', 'Users', 'WandSparkles', 'Wheat',
    'Wrench'
);

CREATE TYPE timeline_visibility AS ENUM ('PUBLIC', 'DM');

CREATE TABLE colony (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    name text NOT NULL DEFAULT 'The Colony',
    day integer NOT NULL DEFAULT 0 CHECK (day >= 0),
    -- Incremented by every colony mutation; forms send it back as expectedVersion (spec §5.1).
    version bigint NOT NULL DEFAULT 0,
    shipment_interval_days integer CHECK (shipment_interval_days >= 1),
    next_shipment_day integer,
    -- The undo slot: {groupId, label, day, versionAfter, changedAt, before}; NULL when nothing can be
    -- undone. Its shape belongs to internal/core.
    last_change jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT colony_shipment_schedule_whole CHECK ((shipment_interval_days IS NULL) = (next_shipment_day IS NULL)),
    -- The next arrival is always in the future, so the countdown can never stall at 0 (spec §5.3).
    CONSTRAINT colony_next_shipment_after_day CHECK (next_shipment_day > day)
);

INSERT INTO colony (id) VALUES (1);

CREATE TABLE resource (
    -- Stable once created: Effects JSON (rewards, actions, timeline deltas) names resources by key.
    key text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]{0,39}$'),
    label text NOT NULL CHECK (label <> ''),
    icon colony_icon NOT NULL,
    amount integer NOT NULL DEFAULT 0,
    min_amount integer NOT NULL DEFAULT 0,
    max_amount integer,
    sort_order integer NOT NULL DEFAULT 0,
    -- A system resource is read by a rule (food, by upkeep), so it can't be archived.
    is_system boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    CONSTRAINT resource_amount_within_bounds CHECK (amount >= min_amount AND (max_amount IS NULL OR amount <= max_amount)),
    CONSTRAINT resource_system_not_archived CHECK (NOT (is_system AND archived))
);

INSERT INTO resource (key, label, icon, amount, min_amount, max_amount, sort_order, is_system) VALUES
    ('gold', 'Gold', 'Coins', 0, 0, NULL, 10, false),
    ('food', 'Food', 'Wheat', 0, 0, NULL, 20, true),
    ('materials', 'Materials', 'Boxes', 0, 0, NULL, 30, false),
    ('morale', 'Morale', 'HeartHandshake', 100, 0, 100, 40, false);

CREATE TABLE role (
    key text PRIMARY KEY CHECK (key ~ '^[A-Z][A-Z0-9_]{0,39}$'),
    label text NOT NULL CHECK (label <> ''),
    icon colony_icon NOT NULL,
    sort_order integer NOT NULL DEFAULT 0,
    -- Colonists without a name in this role; named NPC colonists (M2) count on top of it.
    unnamed_count integer NOT NULL DEFAULT 0 CHECK (unnamed_count >= 0),
    -- UNASSIGNED is the system role where every new colonist lands (spec §5.5).
    is_system boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    -- A role is archived only when nobody is in it; core also checks its named colonists (M2).
    CONSTRAINT role_archived_empty CHECK (NOT (archived AND unnamed_count > 0)),
    CONSTRAINT role_system_not_archived CHECK (NOT (is_system AND archived))
);

INSERT INTO role (key, label, icon, sort_order, is_system) VALUES
    ('FARMER', 'Farmer', 'Wheat', 10, false),
    ('HEALER', 'Healer', 'HeartPulse', 20, false),
    ('BLACKSMITH', 'Blacksmith', 'Hammer', 30, false),
    ('SOLDIER', 'Soldier', 'Sword', 40, false),
    ('MINER', 'Miner', 'Pickaxe', 50, false),
    ('BUILDER', 'Builder', 'HardHat', 60, false),
    ('SCHOLAR', 'Scholar', 'BookOpen', 70, false),
    ('MAGE', 'Mage', 'WandSparkles', 80, false),
    ('UNASSIGNED', 'Unassigned', 'Users', 90, true);

-- Named timeline_event so it isn't confused with the future Events mechanic. Text is rendered when
-- the entry is written, in player-facing words (spec §2.2, item 5).
CREATE TABLE timeline_event (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Shared by every entry one colony mutation writes; undo marks a whole group. NULL for CUSTOM.
    group_id uuid,
    -- Write order; it breaks ties between equal created_at values.
    sequence bigint NOT NULL GENERATED ALWAYS AS IDENTITY UNIQUE,
    day integer NOT NULL CHECK (day >= 0),
    -- Validated against core's TimelineKind list; a new kind needs no migration (spec §4).
    kind text NOT NULL CHECK (kind <> ''),
    title text NOT NULL CHECK (title <> ''),
    body text NOT NULL DEFAULT '',
    visibility timeline_visibility NOT NULL,
    -- The applied Effects only (spec §5.4), e.g. {"resources":{"food":-100}}.
    deltas jsonb NOT NULL DEFAULT '{}',
    -- Kind-specific: SHIPMENT_ARRIVED holds {first, every, count}; CARGO_RECORDED holds {arrivalEntryId}.
    data jsonb NOT NULL DEFAULT '{}',
    undone_at timestamptz,
    created_by uuid REFERENCES member (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at timestamptz
);

CREATE INDEX timeline_event_day_created_at_idx ON timeline_event (day, created_at);
CREATE INDEX timeline_event_group_id_idx ON timeline_event (group_id) WHERE group_id IS NOT NULL;

-- +goose Down

DROP TABLE timeline_event;
DROP TABLE role;
DROP TABLE resource;
DROP TABLE colony;
DROP TYPE timeline_visibility;
DROP TYPE colony_icon;
