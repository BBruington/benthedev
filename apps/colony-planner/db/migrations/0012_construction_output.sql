-- +goose Up

-- Construction, Phase 2 (docs/design/construction.md §11): a blueprint's ongoing output, live rather
-- than snapshotted (K18), and the Idle and Active switch on a finished building (K12).

CREATE TABLE building_blueprint_output (
    blueprint_id uuid NOT NULL,
    resource_key text NOT NULL,
    -- Positive is production, negative is upkeep, applied each day the building stands Active.
    daily_delta integer NOT NULL DEFAULT 0,
    -- Added to the resource's max while the building stands Active.
    storage_bonus integer NOT NULL DEFAULT 0 CHECK (storage_bonus >= 0),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, blueprint_id, resource_key),
    CONSTRAINT building_blueprint_output_does_something CHECK (daily_delta <> 0 OR storage_bonus > 0),
    CONSTRAINT building_blueprint_output_blueprint_fkey FOREIGN KEY (campaign_id, blueprint_id)
        REFERENCES building_blueprint (campaign_id, id) ON DELETE CASCADE,
    CONSTRAINT building_blueprint_output_resource_fkey FOREIGN KEY (campaign_id, resource_key) REFERENCES resource (campaign_id, key)
);

CREATE INDEX building_blueprint_output_campaign_resource_idx ON building_blueprint_output (campaign_id, resource_key);

-- active is meaningful only while FINISHED, and a new building starts Active. added marks a building the
-- DM placed with Add building (K19), which a finished day equal to the start day can't tell apart.
ALTER TABLE building
    ADD COLUMN active boolean NOT NULL DEFAULT true,
    ADD COLUMN added boolean NOT NULL DEFAULT false;

CREATE INDEX building_campaign_finished_idx ON building (campaign_id, blueprint_id) WHERE status = 'FINISHED';

-- K13: when a storage bonus ends, the amount stays above the cap, so only the minimum stays a CHECK.
-- internal/game enforces the effective max (the resource's own plus every Active bonus).
ALTER TABLE resource
    DROP CONSTRAINT resource_amount_within_bounds,
    ADD CONSTRAINT resource_amount_within_bounds CHECK (amount >= min_amount);

-- +goose Down

-- Stock a storage bonus held above its max has nowhere to go without the bonus, so it is cut to the max.
ALTER TABLE resource DROP CONSTRAINT resource_amount_within_bounds;
UPDATE resource SET amount = max_amount WHERE max_amount IS NOT NULL AND amount > max_amount;
ALTER TABLE resource
    ADD CONSTRAINT resource_amount_within_bounds CHECK (amount >= min_amount AND (max_amount IS NULL OR amount <= max_amount));
DROP INDEX building_campaign_finished_idx;
ALTER TABLE building DROP COLUMN active, DROP COLUMN added;
DROP TABLE building_blueprint_output;
