-- +goose Up

-- A hidden resource reaches only the DM: the board, /colony, Foundry's summary and entry deltas leave it out.
ALTER TABLE resource
    ADD COLUMN hidden boolean NOT NULL DEFAULT false;

-- +goose Down

ALTER TABLE resource DROP COLUMN hidden;
