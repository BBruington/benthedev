-- +goose Up

-- M1.5: many campaigns, each sealed (docs/design/multi-campaign.md §2). An account is a Discord
-- identity; a campaign_member is an account's place in one campaign; the site owner is never stored
-- (SITE_OWNER_DISCORD_ID, compared on every request). Every campaign-owned table carries campaign_id,
-- text keys are unique per campaign and links between campaign rows are composite foreign keys.

CREATE TYPE campaign_role AS ENUM ('DM', 'PLAYER');
CREATE TYPE campaign_status AS ENUM ('ACTIVE', 'SUSPENDED');
CREATE TYPE campaign_request_status AS ENUM ('PENDING', 'APPROVED', 'DENIED', 'WITHDRAWN');

-- Today's member, minus everything that is per campaign (status, character name, approval): those
-- columns are dropped below, once the author's rows have been copied into campaign_member.
ALTER TABLE member RENAME TO account;
ALTER TABLE account RENAME CONSTRAINT member_pkey TO account_pkey;
ALTER TABLE account RENAME CONSTRAINT member_discord_user_id_key TO account_discord_user_id_key;

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

-- Sessions belong to the account, so one sign-in serves every campaign.
ALTER TABLE web_session RENAME COLUMN member_id TO account_id;
ALTER TABLE web_session RENAME CONSTRAINT web_session_member_id_fkey TO web_session_account_id_fkey;
ALTER INDEX web_session_member_id_idx RENAME TO web_session_account_id_idx;

-- Nullable until the author's rows are attributed below.
ALTER TABLE settings ADD COLUMN campaign_id uuid;
ALTER TABLE colony ADD COLUMN campaign_id uuid;
ALTER TABLE resource ADD COLUMN campaign_id uuid;
ALTER TABLE role ADD COLUMN campaign_id uuid;
ALTER TABLE timeline_event ADD COLUMN campaign_id uuid;

-- Only this statement substitutes environment variables: the DO block below uses $$, which ENVSUB
-- would expand. There is no default here; the DO block decides what an empty value means.
-- +goose ENVSUB ON
SELECT set_config('colony.site_owner_discord_id', '${SITE_OWNER_DISCORD_ID:-}', true),
       set_config('colony.legacy_dm_discord_id', '${DM_DISCORD_ID:-}', true);
-- +goose ENVSUB OFF

-- +goose StatementBegin
DO $$
DECLARE
    owner_discord_id text := coalesce(current_setting('colony.site_owner_discord_id', true), '');
    legacy_discord_id text := coalesce(current_setting('colony.legacy_dm_discord_id', true), '');
    owner_account_id uuid;
    first_campaign_id uuid;
    first_campaign_name text;
    first_campaign_slug text;
BEGIN
    -- A fresh database (every test, mise run reset): no campaign until someone is approved, so the
    -- singleton and default rows 0001 and 0002 inserted go. The variables are ignored, so a stale
    -- shell environment can't break a test database.
    IF NOT EXISTS (SELECT 1 FROM account) THEN
        DELETE FROM timeline_event;
        DELETE FROM role;
        DELETE FROM resource;
        DELETE FROM colony;
        DELETE FROM settings;
        RETURN;
    END IF;

    IF legacy_discord_id <> '' AND owner_discord_id = '' THEN
        RAISE EXCEPTION '0003: rename DM_DISCORD_ID to SITE_OWNER_DISCORD_ID in .env (keep the value).';
    END IF;
    IF legacy_discord_id <> '' AND legacy_discord_id <> owner_discord_id THEN
        RAISE EXCEPTION '0003: DM_DISCORD_ID and SITE_OWNER_DISCORD_ID differ; delete DM_DISCORD_ID.';
    END IF;
    IF owner_discord_id = '' THEN
        owner_discord_id := 'seed:dm';
    END IF;
    IF owner_discord_id !~ '^([0-9]{17,20}|seed:dm)$' THEN
        RAISE EXCEPTION '0003: SITE_OWNER_DISCORD_ID must be a Discord user ID (17 to 20 digits).';
    END IF;

    SELECT id INTO owner_account_id FROM account WHERE discord_user_id = owner_discord_id;
    IF owner_account_id IS NULL THEN
        RAISE EXCEPTION '0003: no account has Discord ID %. Put the Discord ID you used as DM into SITE_OWNER_DISCORD_ID, or run mise run fresh.', owner_discord_id;
    END IF;

    SELECT coalesce(nullif(left(btrim(campaign_name), 80), ''), 'Colony Campaign')
    INTO first_campaign_name FROM settings LIMIT 1;
    first_campaign_name := coalesce(first_campaign_name, 'Colony Campaign');
    -- Cut to 40 before trimming '-', so the cut can't leave a trailing '-'.
    first_campaign_slug := btrim(left(regexp_replace(lower(first_campaign_name), '[^a-z0-9]+', '-', 'g'), 40), '-');
    IF length(first_campaign_slug) < 3 THEN
        first_campaign_slug := 'colony';
    END IF;

    INSERT INTO campaign (name, slug, invite_code)
    VALUES (first_campaign_name, first_campaign_slug, replace(gen_random_uuid()::text, '-', ''))
    RETURNING id INTO first_campaign_id;

    -- Each membership reuses its account's id, so timeline_event.created_by stays valid unrewritten.
    INSERT INTO campaign_member (id, campaign_id, account_id, role, status, character_name, created_at, approved_at)
    SELECT account.id,
           first_campaign_id,
           account.id,
           CASE WHEN account.id = owner_account_id THEN 'DM'::campaign_role ELSE 'PLAYER'::campaign_role END,
           CASE WHEN account.id = owner_account_id THEN 'ACTIVE'::member_status ELSE account.status END,
           account.character_name,
           account.created_at,
           CASE WHEN account.id = owner_account_id THEN coalesce(account.approved_at, account.created_at) ELSE account.approved_at END
    FROM account;

    -- The stored channels belonged to the legacy DISCORD_GUILD_ID server, which has no link row, and
    -- a channel must belong to the linked server: kept, they would block every save of Settings until a
    -- server is linked. Linking clears them anyway; the DM picks them again from the linked server.
    UPDATE settings SET campaign_id = first_campaign_id,
                        discord_announce_channel_id = NULL,
                        discord_reminder_channel_id = NULL,
                        discord_dm_channel_id = NULL;
    UPDATE colony SET campaign_id = first_campaign_id;
    UPDATE resource SET campaign_id = first_campaign_id;
    UPDATE role SET campaign_id = first_campaign_id;
    UPDATE timeline_event SET campaign_id = first_campaign_id;
END
$$;
-- +goose StatementEnd

-- The campaign's name lives on campaign; the singleton ids give way to campaign_id.
ALTER TABLE settings DROP COLUMN id;
ALTER TABLE settings DROP COLUMN campaign_name;
ALTER TABLE settings ALTER COLUMN campaign_id SET NOT NULL;
ALTER TABLE settings ADD CONSTRAINT settings_pkey PRIMARY KEY (campaign_id);
ALTER TABLE settings ADD CONSTRAINT settings_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES campaign (id) ON DELETE CASCADE;

ALTER TABLE colony DROP COLUMN id;
ALTER TABLE colony ALTER COLUMN campaign_id SET NOT NULL;
ALTER TABLE colony ADD CONSTRAINT colony_pkey PRIMARY KEY (campaign_id);
ALTER TABLE colony ADD CONSTRAINT colony_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES campaign (id) ON DELETE CASCADE;

-- Every campaign has its own gold and food.
ALTER TABLE resource DROP CONSTRAINT resource_pkey;
ALTER TABLE resource ALTER COLUMN campaign_id SET NOT NULL;
ALTER TABLE resource ADD CONSTRAINT resource_pkey PRIMARY KEY (campaign_id, key);
ALTER TABLE resource ADD CONSTRAINT resource_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES campaign (id) ON DELETE CASCADE;

ALTER TABLE role DROP CONSTRAINT role_pkey;
ALTER TABLE role ALTER COLUMN campaign_id SET NOT NULL;
ALTER TABLE role ADD CONSTRAINT role_pkey PRIMARY KEY (campaign_id, key);
ALTER TABLE role ADD CONSTRAINT role_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES campaign (id) ON DELETE CASCADE;

-- sequence stays a global identity: it only orders within a query that already filters by campaign.
ALTER TABLE timeline_event DROP CONSTRAINT timeline_event_created_by_fkey;
ALTER TABLE timeline_event ALTER COLUMN campaign_id SET NOT NULL;
ALTER TABLE timeline_event ADD CONSTRAINT timeline_event_campaign_id_fkey FOREIGN KEY (campaign_id) REFERENCES campaign (id) ON DELETE CASCADE;
ALTER TABLE timeline_event ADD CONSTRAINT timeline_event_campaign_id_id_key UNIQUE (campaign_id, id);
ALTER TABLE timeline_event ADD CONSTRAINT timeline_event_created_by_fkey
    FOREIGN KEY (campaign_id, created_by) REFERENCES campaign_member (campaign_id, id) ON DELETE SET NULL (created_by);
DROP INDEX timeline_event_day_created_at_idx;
DROP INDEX timeline_event_group_id_idx;
CREATE INDEX timeline_event_campaign_day_created_at_idx ON timeline_event (campaign_id, day, created_at);
CREATE INDEX timeline_event_campaign_group_id_idx ON timeline_event (campaign_id, group_id) WHERE group_id IS NOT NULL;

ALTER TABLE account DROP COLUMN status;
ALTER TABLE account DROP COLUMN character_name;
ALTER TABLE account DROP COLUMN approved_at;

-- +goose Down

-- +goose StatementBegin
DO $$
BEGIN
    RAISE EXCEPTION '0003 cannot be undone; run mise run reset';
END
$$;
-- +goose StatementEnd
