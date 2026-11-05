-- +goose Up

-- Polls (spec §3.5): the DM asks the party a question, and each active player answers once. Where they
-- vote is fixed at creation: on the site, or in a native Discord poll Beny Bot posts when the campaign
-- has a linked server and an announcements channel. Polls have no secrets and no hidden rows. A poll is
-- closed once closed_at is set or closes_at has passed; closing writes a POLL_CLOSED entry, which stays
-- when the poll is deleted, so timeline_event gets no link column.
CREATE TYPE poll_voting AS ENUM ('SITE', 'DISCORD');

CREATE TABLE poll (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    question text NOT NULL CHECK (question <> '' AND length(question) <= 300),
    allow_multiple boolean NOT NULL DEFAULT false,
    voting poll_voting NOT NULL,
    closes_at timestamptz NOT NULL,
    closed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- Beny Bot's Discord poll; all NULL until it is posted, and always for a SITE poll.
    discord_guild_id text CHECK (discord_guild_id ~ '^[0-9]{17,20}$'),
    discord_channel_id text CHECK (discord_channel_id ~ '^[0-9]{17,20}$'),
    discord_message_id text CHECK (discord_message_id ~ '^[0-9]{17,20}$'),
    -- Distinct Discord voters who aren't the campaign's active players: only their count is kept.
    discord_other_voters integer NOT NULL DEFAULT 0 CHECK (discord_other_voters >= 0),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT poll_discord_message_whole CHECK (
        (discord_guild_id IS NULL) = (discord_message_id IS NULL)
        AND (discord_channel_id IS NULL) = (discord_message_id IS NULL)
    ),
    CONSTRAINT poll_discord_message_only_discord CHECK (voting = 'DISCORD' OR discord_message_id IS NULL)
);

CREATE INDEX poll_campaign_created_idx ON poll (campaign_id, created_at);
CREATE INDEX poll_campaign_open_idx ON poll (campaign_id, closes_at) WHERE closed_at IS NULL;
CREATE UNIQUE INDEX poll_campaign_discord_message_idx ON poll (campaign_id, discord_message_id)
    WHERE discord_message_id IS NOT NULL;

CREATE TABLE poll_option (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    poll_id uuid NOT NULL,
    position integer NOT NULL CHECK (position BETWEEN 0 AND 9),
    label text NOT NULL CHECK (label <> '' AND length(label) <= 55),
    -- The Discord poll's answer for this option; NULL until the poll is posted.
    discord_answer_id integer,
    -- Discord votes for this option from anyone who isn't one of the campaign's active players.
    discord_other_votes integer NOT NULL DEFAULT 0 CHECK (discord_other_votes >= 0),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    UNIQUE (campaign_id, poll_id, position),
    UNIQUE (campaign_id, poll_id, discord_answer_id),
    -- The target of poll_vote's option key, so a vote's option belongs to the vote's poll.
    UNIQUE (campaign_id, poll_id, id),
    CONSTRAINT poll_option_poll_fkey FOREIGN KEY (campaign_id, poll_id) REFERENCES poll (campaign_id, id) ON DELETE CASCADE
);

-- One row per option a player picked. Members are never deleted, so a removed member's votes stay.
CREATE TABLE poll_vote (
    poll_id uuid NOT NULL,
    option_id uuid NOT NULL,
    voter uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, poll_id, voter, option_id),
    CONSTRAINT poll_vote_poll_fkey FOREIGN KEY (campaign_id, poll_id) REFERENCES poll (campaign_id, id) ON DELETE CASCADE,
    CONSTRAINT poll_vote_option_fkey FOREIGN KEY (campaign_id, poll_id, option_id)
        REFERENCES poll_option (campaign_id, poll_id, id) ON DELETE CASCADE,
    CONSTRAINT poll_vote_voter_fkey FOREIGN KEY (campaign_id, voter) REFERENCES campaign_member (campaign_id, id)
);

CREATE INDEX poll_vote_campaign_voter_idx ON poll_vote (campaign_id, voter);

-- +goose Down

DROP TABLE poll_vote;
DROP TABLE poll_option;
DROP TABLE poll;
DROP TYPE poll_voting;
