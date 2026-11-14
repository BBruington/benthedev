-- +goose Up

-- The Foundry bridge (docs/design/foundry-bridge.md): one campaign's link to one Foundry world, the
-- one-time codes that make a link, the outbox Foundry reads and the events Foundry reports. Only
-- SHA-256 hashes of tokens and codes are stored.
CREATE TYPE bridge_outbox_kind AS ENUM ('COLONY_SUMMARY', 'DAYS_ADVANCED');
CREATE TYPE bridge_event_kind AS ENUM ('STATUS', 'TIME_ADVANCED');
CREATE TYPE bridge_event_status AS ENUM ('PENDING', 'ACCEPTED', 'DISMISSED');

-- A revoked link is kept for the record and for outbox_seq, which the next link carries on, so a
-- campaign's outbox sequence never goes back.
CREATE TABLE bridge_link (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    token_hash bytea NOT NULL CHECK (length(token_hash) = 32),
    origin text NOT NULL CHECK (origin <> '' AND length(origin) <= 100),
    linked_at timestamptz NOT NULL DEFAULT now(),
    last_contact_on date,
    revoked_at timestamptz,
    outbox_seq bigint NOT NULL DEFAULT 0 CHECK (outbox_seq >= 0),
    -- The module's newest STATUS event; '' until the first one.
    foundry_version text NOT NULL DEFAULT '' CHECK (length(foundry_version) <= 100),
    system_id text NOT NULL DEFAULT '' CHECK (length(system_id) <= 100),
    system_version text NOT NULL DEFAULT '' CHECK (length(system_version) <= 100),
    module_version text NOT NULL DEFAULT '' CHECK (length(module_version) <= 100),
    world_title text NOT NULL DEFAULT '' CHECK (length(world_title) <= 100),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id)
);

-- The bridge token names its campaign (the one unscoped token lookup), as the ICS token does.
CREATE UNIQUE INDEX bridge_link_token_hash_idx ON bridge_link (token_hash);
CREATE UNIQUE INDEX bridge_link_campaign_active_idx ON bridge_link (campaign_id) WHERE revoked_at IS NULL;
-- A browser's preflight carries no token, only its origin.
CREATE INDEX bridge_link_active_origin_idx ON bridge_link (origin) WHERE revoked_at IS NULL;

-- A code is closed once it is used, replaced or revoked; closed codes stay a day for the hourly limit.
CREATE TABLE bridge_pairing_code (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    code_hash bytea NOT NULL CHECK (length(code_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    closed_at timestamptz,
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id)
);

CREATE UNIQUE INDEX bridge_pairing_code_hash_idx ON bridge_pairing_code (code_hash);
CREATE UNIQUE INDEX bridge_pairing_code_campaign_open_idx ON bridge_pairing_code (campaign_id) WHERE closed_at IS NULL;
CREATE INDEX bridge_pairing_code_campaign_created_idx ON bridge_pairing_code (campaign_id, created_at);

-- Rows belong to the campaign, not the link, so a GM who reconnects from another browser still gets
-- what was waiting. A row is unapplied until applied_at or failed_at is set.
CREATE TABLE bridge_outbox (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    seq bigint NOT NULL CHECK (seq > 0),
    kind bridge_outbox_kind NOT NULL,
    target_key text NOT NULL CHECK (target_key <> '' AND length(target_key) <= 100),
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    applied_at timestamptz,
    failed_at timestamptz,
    failure_reason text NOT NULL DEFAULT '' CHECK (length(failure_reason) <= 300),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    UNIQUE (campaign_id, seq),
    CONSTRAINT bridge_outbox_applied_or_failed CHECK (applied_at IS NULL OR failed_at IS NULL)
);

CREATE UNIQUE INDEX bridge_outbox_campaign_unapplied_target_idx ON bridge_outbox (campaign_id, target_key)
    WHERE applied_at IS NULL AND failed_at IS NULL;

-- module_event_id is the module's own UUID, kept to ignore a repeat. days is a TIME_ADVANCED event's.
CREATE TABLE bridge_event (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    module_event_id uuid NOT NULL,
    kind bridge_event_kind NOT NULL,
    status bridge_event_status NOT NULL,
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    days integer CHECK (days BETWEEN 1 AND 3650),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    UNIQUE (campaign_id, module_event_id),
    CONSTRAINT bridge_event_days_only_time CHECK ((kind = 'TIME_ADVANCED') = (days IS NOT NULL)),
    CONSTRAINT bridge_event_status_never_pending CHECK (kind = 'TIME_ADVANCED' OR status = 'ACCEPTED')
);

CREATE INDEX bridge_event_campaign_status_idx ON bridge_event (campaign_id, status, received_at);

-- +goose Down

DROP TABLE bridge_event;
DROP TABLE bridge_outbox;
DROP TABLE bridge_pairing_code;
DROP TABLE bridge_link;
DROP TYPE bridge_event_status;
DROP TYPE bridge_event_kind;
DROP TYPE bridge_outbox_kind;
