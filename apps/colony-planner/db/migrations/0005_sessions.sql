-- +goose Up

-- Sessions and time (spec §4.5, §5.11, §7.3; docs/design/multi-campaign.md §2.6). Both tables are
-- campaign-owned and every link between campaign rows is a composite foreign key.

CREATE TYPE session_status AS ENUM ('SCHEDULED', 'CANCELLED', 'PLAYED');

-- A slot in local wall time, read in the campaign's settings.timezone by game.Occurrences. A one-off is a
-- series too (every_weeks NULL, no weekdays, on first_date), so one code path serves events, reminders,
-- the ICS feed and /nextsession.
CREATE TABLE session_series (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    title text NOT NULL CHECK (title <> '' AND length(title) <= 80),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    -- 0 = Sunday … 6 = Saturday.
    weekdays smallint[] NOT NULL DEFAULT '{}' CHECK (weekdays <@ '{0,1,2,3,4,5,6}'::smallint[]),
    every_weeks smallint CHECK (every_weeks IN (1, 2)),
    first_date date NOT NULL,
    last_date date,
    start_local time NOT NULL,
    duration_minutes smallint NOT NULL CHECK (duration_minutes BETWEEN 15 AND 480),
    announce boolean NOT NULL DEFAULT true,
    remind boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT session_series_repeats_on_weekdays CHECK ((every_weeks IS NULL) = (cardinality(weekdays) = 0)),
    CONSTRAINT session_series_one_off_has_no_end CHECK (every_weeks IS NOT NULL OR last_date IS NULL),
    CONSTRAINT session_series_dates_in_order CHECK (last_date IS NULL OR last_date >= first_date)
);

-- One occurrence's row, created on demand (cancel, event, reminder, played, recap) and keyed by its local
-- date, so a cancellation survives a change to the series' start time. Deleting a series keeps its played
-- history: only series_id clears, and those rows stand alone.
CREATE TABLE session (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    series_id uuid,
    local_date date NOT NULL,
    starts_at timestamptz NOT NULL,
    duration_minutes smallint NOT NULL CHECK (duration_minutes BETWEEN 15 AND 480),
    title text NOT NULL CHECK (title <> '' AND length(title) <= 80),
    status session_status NOT NULL DEFAULT 'SCHEDULED',
    discord_event_id text CHECK (discord_event_id ~ '^[0-9]{17,20}$'),
    -- The location the event was made or last edited with, so a changed settings location reaches it.
    discord_event_location text,
    -- A lease, not a flag: a planner that crashed mid-create is retried once it is 5 minutes old (§7.3).
    discord_claimed_at timestamptz,
    reminded_at timestamptz,
    recap text NOT NULL DEFAULT '' CHECK (length(recap) <= 20000),
    colony_day_start integer CHECK (colony_day_start >= 0),
    colony_day_end integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT session_series_fkey FOREIGN KEY (campaign_id, series_id)
        REFERENCES session_series (campaign_id, id) ON DELETE SET NULL (series_id),
    CONSTRAINT session_series_local_date_key UNIQUE (campaign_id, series_id, local_date),
    CONSTRAINT session_stamps_when_played CHECK (status = 'PLAYED' OR (colony_day_start IS NULL AND colony_day_end IS NULL)),
    CONSTRAINT session_end_after_start CHECK (colony_day_end IS NULL OR (colony_day_start IS NOT NULL AND colony_day_end >= colony_day_start))
);

CREATE INDEX session_campaign_starts_at_idx ON session (campaign_id, starts_at);
CREATE UNIQUE INDEX session_campaign_discord_event_key ON session (campaign_id, discord_event_id) WHERE discord_event_id IS NOT NULL;

-- The Discord event's text location (an external event, Open Question 15) and the calendar feed's secret.
-- The token is global because /calendar/{token}.ics resolves its campaign from it; two random UUIDs give
-- 244 random bits, and core regenerates it from crypto/rand.
ALTER TABLE settings
    ADD COLUMN discord_event_location text NOT NULL DEFAULT 'Online'
        CHECK (discord_event_location <> '' AND length(discord_event_location) <= 100),
    ADD COLUMN ics_token text NOT NULL DEFAULT replace(gen_random_uuid()::text || gen_random_uuid()::text, '-', '')
        CHECK (ics_token ~ '^[A-Za-z0-9_-]{43,64}$'),
    ADD CONSTRAINT settings_ics_token_key UNIQUE (ics_token);

-- Where the party first met an NPC (Open Question 10), which M2 left out.
ALTER TABLE npc
    ADD COLUMN first_session_id uuid,
    ADD CONSTRAINT npc_first_session_fkey FOREIGN KEY (campaign_id, first_session_id)
        REFERENCES session (campaign_id, id) ON DELETE SET NULL (first_session_id);

CREATE INDEX npc_campaign_first_session_idx ON npc (campaign_id, first_session_id) WHERE first_session_id IS NOT NULL;

ALTER TABLE timeline_event
    ADD COLUMN session_id uuid,
    ADD CONSTRAINT timeline_event_session_fkey FOREIGN KEY (campaign_id, session_id)
        REFERENCES session (campaign_id, id) ON DELETE SET NULL (session_id);

CREATE INDEX timeline_event_campaign_session_idx ON timeline_event (campaign_id, session_id) WHERE session_id IS NOT NULL;

-- +goose Down

ALTER TABLE timeline_event DROP COLUMN session_id;
ALTER TABLE npc DROP COLUMN first_session_id;
ALTER TABLE settings DROP CONSTRAINT settings_ics_token_key, DROP COLUMN ics_token, DROP COLUMN discord_event_location;
DROP TABLE session;
DROP TABLE session_series;
DROP TYPE session_status;
