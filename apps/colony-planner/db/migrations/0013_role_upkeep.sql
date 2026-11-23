-- +goose Up

-- Silver each colonist in the role costs per day. Advance totals it over the days advanced and pays it
-- from gold, rounded up to whole gold once per advance (spec §5.2).
ALTER TABLE role
    ADD COLUMN upkeep_silver integer NOT NULL DEFAULT 0 CHECK (upkeep_silver >= 0);

-- +goose Down

ALTER TABLE role DROP COLUMN upkeep_silver;
