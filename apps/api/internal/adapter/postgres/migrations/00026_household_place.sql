-- +goose Up
-- The town the household lives in, as typed: the morning briefing looks up its weather there.
ALTER TABLE households ADD COLUMN place text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE households DROP COLUMN place;
