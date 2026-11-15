-- +goose Up

-- The Foundry bridge's third phase (docs/design/foundry-bridge.md §14): the GM sends coin deposits, NPCs
-- and items to the DM's inbox, and a dismissed or undone deposit tells Foundry. Postgres refuses a value
-- added in the same transaction, so every CHECK here compares kind::text.
ALTER TYPE bridge_event_kind ADD VALUE 'COIN_DEPOSIT';
ALTER TYPE bridge_event_kind ADD VALUE 'NPC_IMPORT';
ALTER TYPE bridge_event_kind ADD VALUE 'ITEM_IMPORT';
ALTER TYPE bridge_outbox_kind ADD VALUE 'DEPOSIT_REFUND';
ALTER TYPE bridge_outbox_kind ADD VALUE 'DEPOSIT_UNDONE';

-- gold is a deposit's amount; content is an inbox item's cleaned payload, re-encoded by core.
ALTER TABLE bridge_event
    ADD COLUMN gold integer CHECK (gold BETWEEN 1 AND 1000000),
    ADD COLUMN content jsonb CHECK (length(content::text) <= 32768),
    ADD CONSTRAINT bridge_event_gold_only_deposit CHECK ((kind::text = 'COIN_DEPOSIT') = (gold IS NOT NULL)),
    ADD CONSTRAINT bridge_event_content_only_imports CHECK (
        (kind::text IN ('COIN_DEPOSIT', 'NPC_IMPORT', 'ITEM_IMPORT')) = (content IS NOT NULL)),
    DROP CONSTRAINT bridge_event_status_never_pending,
    ADD CONSTRAINT bridge_event_status_never_pending CHECK (kind::text <> 'STATUS' OR status = 'ACCEPTED');

-- +goose Down

DELETE FROM bridge_event WHERE kind::text IN ('COIN_DEPOSIT', 'NPC_IMPORT', 'ITEM_IMPORT');
DELETE FROM bridge_outbox WHERE kind::text IN ('DEPOSIT_REFUND', 'DEPOSIT_UNDONE');

-- A CHECK comparing kind to an enum literal can't follow the column to a rebuilt type, so both go first.
ALTER TABLE bridge_event
    DROP COLUMN gold,
    DROP COLUMN content,
    DROP CONSTRAINT bridge_event_status_never_pending,
    DROP CONSTRAINT bridge_event_days_only_time;

ALTER TYPE bridge_event_kind RENAME TO bridge_event_kind_phase_three;
CREATE TYPE bridge_event_kind AS ENUM ('STATUS', 'TIME_ADVANCED');
ALTER TABLE bridge_event ALTER COLUMN kind TYPE bridge_event_kind USING kind::text::bridge_event_kind;
DROP TYPE bridge_event_kind_phase_three;

ALTER TABLE bridge_event
    ADD CONSTRAINT bridge_event_days_only_time CHECK ((kind = 'TIME_ADVANCED') = (days IS NOT NULL)),
    ADD CONSTRAINT bridge_event_status_never_pending CHECK (kind = 'TIME_ADVANCED' OR status = 'ACCEPTED');

ALTER TYPE bridge_outbox_kind RENAME TO bridge_outbox_kind_phase_three;
CREATE TYPE bridge_outbox_kind AS ENUM (
    'COLONY_SUMMARY', 'DAYS_ADVANCED', 'NPC_UPSERT', 'NPC_WITHDRAW', 'PLACE_UPSERT', 'PLACE_WITHDRAW',
    'QUEST_UPSERT', 'QUEST_WITHDRAW', 'DELIVERY', 'DELIVERY_UNDONE', 'POLL_POSTED', 'POLL_CLOSED');
ALTER TABLE bridge_outbox ALTER COLUMN kind TYPE bridge_outbox_kind USING kind::text::bridge_outbox_kind;
DROP TYPE bridge_outbox_kind_phase_three;
