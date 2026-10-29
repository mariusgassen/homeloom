-- +goose Up
-- Moments are what a household keeps of its life together: moments (a line and maybe a photo,
-- on the day they happened) and gratitude lines (one per person and day). They share the
-- visibility model of entries; 'reveal' opens at reveal_at, for letters to the future.
CREATE TABLE moments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    author_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('moment', 'gratitude')),
    day          date NOT NULL,
    visibility   text NOT NULL CHECK (visibility IN ('private', 'shared', 'reveal')),
    reveal_at    timestamptz,
    text         text NOT NULL,
    -- photo_at is when the photo was last set; NULL means the moment has none.
    photo_at     timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((visibility = 'reveal') = (reveal_at IS NOT NULL))
);

CREATE INDEX moments_household_day_idx ON moments (household_id, day DESC);
CREATE UNIQUE INDEX moments_gratitude_daily_idx ON moments (household_id, author_id, day) WHERE kind = 'gratitude';

-- Photos are kept apart so listing moments never reads them.
CREATE TABLE moment_photos (
    moment_id    uuid PRIMARY KEY REFERENCES moments (id) ON DELETE CASCADE,
    content_type text NOT NULL,
    data         bytea NOT NULL
);

-- A daily job runs once per day, whichever API instance gets there first.
CREATE TABLE job_runs (
    job text NOT NULL,
    day date NOT NULL,
    PRIMARY KEY (job, day)
);

-- +goose Down
DROP TABLE job_runs;
DROP TABLE moment_photos;
DROP TABLE moments;
