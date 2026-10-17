-- +goose Up

-- Quests, patrons, patron actions and requests (spec §4.3, §5.6, §5.7; docs/design/multi-campaign.md
-- §2.6). Every table is campaign-owned and every link between campaign rows is a composite foreign key.
-- Patrons and actions are archived, never deleted: their foreign keys are NO ACTION, which refuses a
-- stray delete like RESTRICT does but checks at the end of the statement, so deleting a campaign can
-- still cascade through a patron and the quests and requests that name it.

CREATE TYPE quest_kind AS ENUM ('MAINLAND', 'COLONY');
CREATE TYPE quest_status AS ENUM ('ACTIVE', 'COMPLETED', 'FAILED');
CREATE TYPE request_status AS ENUM ('PENDING', 'APPROVED', 'DECLINED', 'WITHDRAWN');

CREATE TABLE patron (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    description text NOT NULL DEFAULT '',
    -- Players have never seen patrons, so they start hidden (Open Question 9).
    revealed boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    -- DM adjustments and failed quests may take favor below 0; spending never does (Open Question 12).
    favor integer NOT NULL DEFAULT 0,
    player_notes text NOT NULL DEFAULT '',
    dm_notes text NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id)
);

CREATE INDEX patron_campaign_sort_idx ON patron (campaign_id, sort_order, lower(name));

CREATE TABLE patron_action (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    patron_id uuid NOT NULL,
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    description text NOT NULL DEFAULT '',
    favor_cost integer NOT NULL DEFAULT 0 CHECK (favor_cost >= 0),
    -- Effects JSON (spec §4.3); its resource keys are checked by core against this campaign's resources.
    effects jsonb NOT NULL DEFAULT '{}',
    revealed boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT patron_action_patron_fkey FOREIGN KEY (campaign_id, patron_id) REFERENCES patron (campaign_id, id)
);

CREATE INDEX patron_action_campaign_patron_idx ON patron_action (campaign_id, patron_id, sort_order);

CREATE TABLE quest (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    title text NOT NULL CHECK (title <> '' AND length(title) <= 120),
    description text NOT NULL DEFAULT '',
    kind quest_kind NOT NULL DEFAULT 'COLONY',
    status quest_status NOT NULL DEFAULT 'ACTIVE',
    -- Players saw every quest before, so quests start visible (Open Question 9).
    revealed boolean NOT NULL DEFAULT true,
    giver_npc_id uuid,
    patron_id uuid,
    reward_effects jsonb NOT NULL DEFAULT '{}',
    reward_favor integer NOT NULL DEFAULT 0 CHECK (reward_favor >= 0),
    -- The optional failure cost (Open Question 14): DM-only until the quest fails.
    failure_effects jsonb NOT NULL DEFAULT '{}',
    failure_favor integer NOT NULL DEFAULT 0 CHECK (failure_favor >= 0),
    -- [{name, quantity, description}], display only.
    loot jsonb NOT NULL DEFAULT '[]',
    dm_notes text NOT NULL DEFAULT '',
    resolved_day integer,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT quest_reward_favor_needs_patron CHECK (reward_favor = 0 OR patron_id IS NOT NULL),
    CONSTRAINT quest_failure_favor_needs_patron CHECK (failure_favor = 0 OR patron_id IS NOT NULL),
    CONSTRAINT quest_resolved_whole CHECK ((status = 'ACTIVE') = (resolved_day IS NULL) AND (resolved_day IS NULL) = (resolved_at IS NULL)),
    CONSTRAINT quest_giver_npc_fkey FOREIGN KEY (campaign_id, giver_npc_id)
        REFERENCES npc (campaign_id, id) ON DELETE SET NULL (giver_npc_id),
    CONSTRAINT quest_patron_fkey FOREIGN KEY (campaign_id, patron_id) REFERENCES patron (campaign_id, id)
);

CREATE INDEX quest_campaign_status_idx ON quest (campaign_id, status, created_at);
CREATE INDEX quest_campaign_giver_npc_idx ON quest (campaign_id, giver_npc_id) WHERE giver_npc_id IS NOT NULL;
CREATE INDEX quest_campaign_patron_idx ON quest (campaign_id, patron_id) WHERE patron_id IS NOT NULL;

CREATE TABLE request (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    -- Validated in Go; v1 has only PATRON_ACTION (spec §4.3).
    kind text NOT NULL CHECK (kind <> ''),
    -- Snapshots of what was asked for: later edits to the action or patron never change them.
    title text NOT NULL CHECK (title <> ''),
    patron_id uuid,
    patron_name text,
    patron_action_id uuid,
    cost jsonb NOT NULL DEFAULT '{}',
    effects jsonb NOT NULL DEFAULT '{}',
    -- The DM's per-approval tweak; NULL when the snapshot was applied as asked.
    applied_effects jsonb,
    requested_by uuid NOT NULL,
    on_behalf_of uuid,
    requested_day integer NOT NULL CHECK (requested_day >= 0),
    note text NOT NULL DEFAULT '',
    -- Whether players could see the patron and the action when it was made. One made while either was
    -- hidden (Use action now) never reaches players, even after both are revealed under new names.
    made_while_revealed boolean NOT NULL,
    status request_status NOT NULL DEFAULT 'PENDING',
    decided_by uuid,
    decided_at timestamptz,
    -- Only the requester and the DM read it (Open Question 12).
    decision_note text NOT NULL DEFAULT '',
    -- The timeline group the approval wrote, so undo can find the request again.
    event_group_id uuid,
    discord_message_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT request_patron_action_names_patron CHECK (kind <> 'PATRON_ACTION' OR patron_name IS NOT NULL),
    CONSTRAINT request_patron_fkey FOREIGN KEY (campaign_id, patron_id) REFERENCES patron (campaign_id, id),
    CONSTRAINT request_patron_action_fkey FOREIGN KEY (campaign_id, patron_action_id) REFERENCES patron_action (campaign_id, id),
    CONSTRAINT request_requested_by_fkey FOREIGN KEY (campaign_id, requested_by) REFERENCES campaign_member (campaign_id, id),
    CONSTRAINT request_on_behalf_of_fkey FOREIGN KEY (campaign_id, on_behalf_of)
        REFERENCES campaign_member (campaign_id, id) ON DELETE SET NULL (on_behalf_of),
    CONSTRAINT request_decided_by_fkey FOREIGN KEY (campaign_id, decided_by)
        REFERENCES campaign_member (campaign_id, id) ON DELETE SET NULL (decided_by)
);

CREATE INDEX request_campaign_status_idx ON request (campaign_id, status, created_at);
CREATE INDEX request_campaign_requested_by_idx ON request (campaign_id, requested_by, created_at);
CREATE INDEX request_campaign_patron_idx ON request (campaign_id, patron_id, created_at) WHERE patron_id IS NOT NULL;

-- The rest of timeline_event's link columns (spec §4.3). Deleting a quest, or the request an undone Use
-- action now made, clears only the link; patrons and actions are never deleted.
ALTER TABLE timeline_event
    ADD COLUMN quest_id uuid,
    ADD COLUMN patron_id uuid,
    ADD COLUMN request_id uuid,
    ADD CONSTRAINT timeline_event_quest_fkey FOREIGN KEY (campaign_id, quest_id)
        REFERENCES quest (campaign_id, id) ON DELETE SET NULL (quest_id),
    ADD CONSTRAINT timeline_event_patron_fkey FOREIGN KEY (campaign_id, patron_id)
        REFERENCES patron (campaign_id, id) ON DELETE SET NULL (patron_id),
    ADD CONSTRAINT timeline_event_request_fkey FOREIGN KEY (campaign_id, request_id)
        REFERENCES request (campaign_id, id) ON DELETE SET NULL (request_id);

CREATE INDEX timeline_event_campaign_quest_idx ON timeline_event (campaign_id, quest_id) WHERE quest_id IS NOT NULL;
CREATE INDEX timeline_event_campaign_patron_idx ON timeline_event (campaign_id, patron_id) WHERE patron_id IS NOT NULL;
CREATE INDEX timeline_event_campaign_request_idx ON timeline_event (campaign_id, request_id) WHERE request_id IS NOT NULL;

-- +goose Down

ALTER TABLE timeline_event DROP COLUMN request_id, DROP COLUMN patron_id, DROP COLUMN quest_id;
DROP TABLE request;
DROP TABLE quest;
DROP TABLE patron_action;
DROP TABLE patron;
DROP TYPE request_status;
DROP TYPE quest_status;
DROP TYPE quest_kind;
