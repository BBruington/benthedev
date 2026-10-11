-- +goose Up

-- The DM is never stored: it is derived from DM_DISCORD_ID on every request.
CREATE TYPE member_status AS ENUM ('PENDING', 'ACTIVE', 'REMOVED');

CREATE TABLE member (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    discord_user_id text NOT NULL UNIQUE,
    discord_username text NOT NULL,
    display_name text NOT NULL,
    avatar_url text,
    character_name text,
    status member_status NOT NULL DEFAULT 'PENDING',
    created_at timestamptz NOT NULL DEFAULT now(),
    approved_at timestamptz,
    -- Written at most once a day, so ordinary requests never write.
    last_seen_at timestamptz
);

CREATE TABLE web_session (
    -- SHA-256 of the 32-byte random cookie token; the token itself is never stored.
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    member_id uuid NOT NULL REFERENCES member (id) ON DELETE CASCADE,
    view_as_player boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    -- Written at most once a day, together with the sliding expiry.
    last_seen_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX web_session_member_id_idx ON web_session (member_id);

CREATE TABLE settings (
    id smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    campaign_name text NOT NULL DEFAULT 'Colony Campaign',
    -- IANA name; session series are interpreted in this zone.
    timezone text NOT NULL DEFAULT 'UTC',
    discord_announce_channel_id text,
    discord_reminder_channel_id text,
    discord_dm_channel_id text
);

INSERT INTO settings (id) VALUES (1);

-- +goose Down

DROP TABLE settings;
DROP TABLE web_session;
DROP TABLE member;
DROP TYPE member_status;
