-- +goose Up
-- A date the household keeps every year, such as Christmas or a wedding anniversary. Any member
-- can add one; the year is optional and says when it first happened.
CREATE TABLE occasions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name         text NOT NULL,
    month        smallint NOT NULL CHECK (month BETWEEN 1 AND 12),
    day          smallint NOT NULL CHECK (day BETWEEN 1 AND 31),
    year         smallint CHECK (year BETWEEN 1900 AND 9999),
    created_at   timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX occasions_household_idx ON occasions (household_id);

-- A gift idea may be for an occasion; if the occasion goes, the idea stays.
ALTER TABLE gift_ideas ADD COLUMN occasion_id uuid REFERENCES occasions (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE gift_ideas DROP COLUMN occasion_id;
DROP TABLE occasions;
