-- +goose Up

-- The Foundry bridge's second phase (docs/design/foundry-bridge.md §13): revealed NPCs, places and quests,
-- paid shipments and polls reach Foundry, and a special item may name the Foundry compendium item a paid
-- shipment hands the party. No new kind is used here, since Postgres refuses a value added in the same
-- transaction.
ALTER TYPE bridge_outbox_kind ADD VALUE 'NPC_UPSERT';
ALTER TYPE bridge_outbox_kind ADD VALUE 'NPC_WITHDRAW';
ALTER TYPE bridge_outbox_kind ADD VALUE 'PLACE_UPSERT';
ALTER TYPE bridge_outbox_kind ADD VALUE 'PLACE_WITHDRAW';
ALTER TYPE bridge_outbox_kind ADD VALUE 'QUEST_UPSERT';
ALTER TYPE bridge_outbox_kind ADD VALUE 'QUEST_WITHDRAW';
ALTER TYPE bridge_outbox_kind ADD VALUE 'DELIVERY';
ALTER TYPE bridge_outbox_kind ADD VALUE 'DELIVERY_UNDONE';
ALTER TYPE bridge_outbox_kind ADD VALUE 'POLL_POSTED';
ALTER TYPE bridge_outbox_kind ADD VALUE 'POLL_CLOSED';

-- Foundry v13's full compendium UUID, as Copy UUID gives it; only a special item carries one.
ALTER TABLE market_item
    ADD COLUMN compendium_uuid text,
    ADD CONSTRAINT market_item_compendium_uuid_special CHECK (compendium_uuid IS NULL OR (resource_key IS NULL AND role_key IS NULL)),
    ADD CONSTRAINT market_item_compendium_uuid_length CHECK (length(compendium_uuid) <= 200),
    ADD CONSTRAINT market_item_compendium_uuid_form CHECK (
        compendium_uuid ~ '^Compendium\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.Item\.[A-Za-z0-9]{16}$');

-- +goose Down

ALTER TABLE market_item DROP COLUMN compendium_uuid;

DELETE FROM bridge_outbox WHERE kind::text NOT IN ('COLONY_SUMMARY', 'DAYS_ADVANCED');
ALTER TYPE bridge_outbox_kind RENAME TO bridge_outbox_kind_phase_two;
CREATE TYPE bridge_outbox_kind AS ENUM ('COLONY_SUMMARY', 'DAYS_ADVANCED');
ALTER TABLE bridge_outbox ALTER COLUMN kind TYPE bridge_outbox_kind USING kind::text::bridge_outbox_kind;
DROP TYPE bridge_outbox_kind_phase_two;
