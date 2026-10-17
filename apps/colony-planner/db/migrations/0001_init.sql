-- +goose Up

-- Many campaigns, each sealed (docs/design/multi-campaign.md §2). An account is a Discord identity; a
-- campaign_member is an account's place in one campaign; the site owner is never stored
-- (SITE_OWNER_DISCORD_ID, compared on every request). Every campaign-owned table carries campaign_id,
-- text keys are unique per campaign and links between campaign rows are composite foreign keys. No row
-- is inserted here: a campaign's settings, colony, resources and roles are created when it is approved.

CREATE TYPE member_status AS ENUM ('PENDING', 'ACTIVE', 'REMOVED');
CREATE TYPE campaign_role AS ENUM ('DM', 'PLAYER');
CREATE TYPE campaign_status AS ENUM ('ACTIVE', 'SUSPENDED');
CREATE TYPE campaign_request_status AS ENUM ('PENDING', 'APPROVED', 'DENIED', 'WITHDRAWN');

-- The curated lucide icons a resource or role may show; the web maps each name to its component.
CREATE TYPE colony_icon AS ENUM (
    'Anchor', 'Apple', 'Axe', 'Beef', 'Beer', 'Bone', 'BookOpen', 'Boxes', 'Coins', 'Crown',
    'Droplets', 'Feather', 'Fish', 'Flame', 'FlaskConical', 'Gem', 'Hammer', 'HardHat', 'Heart',
    'HeartHandshake', 'HeartPulse', 'Leaf', 'Mountain', 'Package', 'Pickaxe', 'Scroll', 'Shield',
    'Ship', 'Shirt', 'Sparkles', 'Star', 'Sword', 'Tent', 'Trees', 'Users', 'WandSparkles', 'Wheat',
    'Wrench'
);

CREATE TYPE timeline_visibility AS ENUM ('PUBLIC', 'DM');

-- Holds nothing per campaign: status, character name and approval live on campaign_member.
CREATE TABLE account (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    discord_user_id text NOT NULL UNIQUE,
    discord_username text NOT NULL,
    display_name text NOT NULL,
    avatar_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- Written at most once a day, so ordinary requests never write.
    last_seen_at timestamptz
);

-- Sessions belong to the account, so one sign-in serves every campaign.
CREATE TABLE web_session (
    -- SHA-256 of the 32-byte random cookie token; the token itself is never stored.
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    account_id uuid NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    view_as_player boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    -- Written at most once a day, together with the sliding expiry.
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX web_session_account_id_idx ON web_session (account_id);

CREATE TABLE campaign (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- In the SPA's URLs; chosen in the request, adjustable by the owner at approval, then fixed.
    slug text NOT NULL UNIQUE CHECK (length(slug) BETWEEN 3 AND 40 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    -- Directory data: the owner sees it. The DM edits it on Settings.
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    status campaign_status NOT NULL DEFAULT 'ACTIVE',
    suspended_at timestamptz,
    -- Random and stored plainly so the DM can copy it again; it only lets someone ask to join. Served
    -- only by the DM's invite read, never in any other DTO.
    invite_code text NOT NULL UNIQUE CHECK (invite_code ~ '^[A-Za-z0-9_-]{22,64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_suspended_at_matches CHECK ((status = 'SUSPENDED') = (suspended_at IS NOT NULL))
);

-- Holds directory data only: no colony name, timezone or other campaign content.
CREATE TABLE campaign_request (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES account (id),
    campaign_name text NOT NULL CHECK (campaign_name <> '' AND length(campaign_name) <= 80),
    slug text NOT NULL,
    -- To the site owner.
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    status campaign_request_status NOT NULL DEFAULT 'PENDING',
    decided_at timestamptz,
    -- Set on approval.
    campaign_id uuid REFERENCES campaign (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX campaign_request_one_open ON campaign_request (account_id) WHERE status = 'PENDING';
CREATE UNIQUE INDEX campaign_request_open_slug ON campaign_request (slug) WHERE status = 'PENDING';

CREATE TABLE campaign_member (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES account (id),
    -- No API operation writes role: only campaign approval (and the seed) inserts a DM.
    role campaign_role NOT NULL DEFAULT 'PLAYER',
    status member_status NOT NULL DEFAULT 'PENDING',
    character_name text,
    -- The player removed themselves ("Left" on /players rather than "Removed"). It is still REMOVED:
    -- the invite link can't bring them back and only the DM re-admits them.
    left_on_own boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    approved_at timestamptz,
    UNIQUE (campaign_id, account_id),
    UNIQUE (campaign_id, id),
    CONSTRAINT campaign_member_dm_active CHECK (role <> 'DM' OR status = 'ACTIVE'),
    CONSTRAINT campaign_member_left_is_removed CHECK (NOT left_on_own OR status = 'REMOVED')
);
CREATE UNIQUE INDEX campaign_member_one_dm ON campaign_member (campaign_id) WHERE role = 'DM';
CREATE INDEX campaign_member_account_idx ON campaign_member (account_id);

CREATE TABLE discord_guild_link (
    -- One server per campaign, and one campaign per server.
    campaign_id uuid PRIMARY KEY REFERENCES campaign (id) ON DELETE CASCADE,
    guild_id text NOT NULL UNIQUE CHECK (guild_id ~ '^[0-9]{17,20}$'),
    -- A snapshot for the DM's Settings card.
    guild_name text NOT NULL,
    linked_by_discord_user_id text NOT NULL,
    linked_at timestamptz NOT NULL DEFAULT now()
);

-- The campaign's name lives on campaign.
CREATE TABLE settings (
    -- IANA name; session series are interpreted in this zone.
    timezone text NOT NULL DEFAULT 'UTC',
    discord_announce_channel_id text,
    discord_reminder_channel_id text,
    discord_dm_channel_id text,
    campaign_id uuid PRIMARY KEY REFERENCES campaign (id) ON DELETE CASCADE
);

-- The colony clock and books (spec §4.2) and the timeline (spec §4.3, §5.8). Link columns from
-- timeline_event to quest, patron, request, npc and session arrive with those tables.

CREATE TABLE colony (
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
    campaign_id uuid PRIMARY KEY REFERENCES campaign (id) ON DELETE CASCADE,
    CONSTRAINT colony_shipment_schedule_whole CHECK ((shipment_interval_days IS NULL) = (next_shipment_day IS NULL)),
    -- The next arrival is always in the future, so the countdown can never stall at 0 (spec §5.3).
    CONSTRAINT colony_next_shipment_after_day CHECK (next_shipment_day > day)
);

CREATE TABLE resource (
    -- Stable once created: Effects JSON (rewards, actions, timeline deltas) names resources by key.
    key text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,39}$'),
    label text NOT NULL CHECK (label <> ''),
    icon colony_icon NOT NULL,
    amount integer NOT NULL DEFAULT 0,
    min_amount integer NOT NULL DEFAULT 0,
    max_amount integer,
    sort_order integer NOT NULL DEFAULT 0,
    -- A system resource is read by a rule (food, by upkeep), so it can't be archived.
    is_system boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    -- Every campaign has its own gold and food.
    PRIMARY KEY (campaign_id, key),
    CONSTRAINT resource_amount_within_bounds CHECK (amount >= min_amount AND (max_amount IS NULL OR amount <= max_amount)),
    CONSTRAINT resource_system_not_archived CHECK (NOT (is_system AND archived))
);

CREATE TABLE role (
    key text NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9_]{0,39}$'),
    label text NOT NULL CHECK (label <> ''),
    icon colony_icon NOT NULL,
    sort_order integer NOT NULL DEFAULT 0,
    -- Colonists without a name in this role; named NPC colonists (M2) count on top of it.
    unnamed_count integer NOT NULL DEFAULT 0 CHECK (unnamed_count >= 0),
    -- UNASSIGNED is the system role where every new colonist lands (spec §5.5).
    is_system boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, key),
    -- A role is archived only when nobody is in it; core also checks its named colonists (M2).
    CONSTRAINT role_archived_empty CHECK (NOT (archived AND unnamed_count > 0)),
    CONSTRAINT role_system_not_archived CHECK (NOT (is_system AND archived))
);

-- Named timeline_event so it isn't confused with the future Events mechanic. Text is rendered when
-- the entry is written, in player-facing words (spec §2.2, item 5).
CREATE TABLE timeline_event (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Shared by every entry one colony mutation writes; undo marks a whole group. NULL for CUSTOM.
    group_id uuid,
    -- Write order; it breaks ties between equal created_at values. One identity for every campaign:
    -- it only orders rows within a query that already filters by campaign.
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
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    edited_at timestamptz,
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    UNIQUE (campaign_id, id),
    -- Composite, so an entry can only name a member of its own campaign.
    CONSTRAINT timeline_event_created_by_fkey FOREIGN KEY (campaign_id, created_by)
        REFERENCES campaign_member (campaign_id, id) ON DELETE SET NULL (created_by)
);

CREATE INDEX timeline_event_campaign_day_created_at_idx ON timeline_event (campaign_id, day, created_at);
CREATE INDEX timeline_event_campaign_group_id_idx ON timeline_event (campaign_id, group_id) WHERE group_id IS NOT NULL;

-- +goose Down

DROP TABLE timeline_event;
DROP TABLE role;
DROP TABLE resource;
DROP TABLE colony;
DROP TABLE settings;
DROP TABLE discord_guild_link;
DROP TABLE campaign_member;
DROP TABLE campaign_request;
DROP TABLE campaign;
DROP TABLE web_session;
DROP TABLE account;
DROP TYPE timeline_visibility;
DROP TYPE colony_icon;
DROP TYPE campaign_request_status;
DROP TYPE campaign_status;
DROP TYPE campaign_role;
DROP TYPE member_status;
