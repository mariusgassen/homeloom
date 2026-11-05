-- +goose Up
-- Trips the household plans together. Start and end are optional dates. chosen_option_id is the
-- option they decided on; it is set to NULL if that option goes (the foreign key is added below,
-- once the options table exists).
CREATE TABLE trips (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id     uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name             text NOT NULL,
    destination      text NOT NULL DEFAULT '',
    start_on         date,
    end_on           date,
    notes            text NOT NULL DEFAULT '',
    chosen_option_id uuid,
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (end_on IS NULL OR start_on IS NULL OR end_on >= start_on)
);

CREATE INDEX trips_household_start_idx ON trips (household_id, start_on);

-- A place to stay or a plan the household is weighing up for a trip.
CREATE TABLE trip_options (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    title      text NOT NULL,
    url        text NOT NULL DEFAULT '',
    notes      text NOT NULL DEFAULT '',
    added_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX trip_options_trip_idx ON trip_options (trip_id, created_at);

ALTER TABLE trips ADD CONSTRAINT trips_chosen_option_fk
    FOREIGN KEY (chosen_option_id) REFERENCES trip_options (id) ON DELETE SET NULL;

-- The photos themselves are kept in S3 under trips/<id>.jpg; only their ids live here.
CREATE TABLE trip_photos (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    option_id  uuid NOT NULL REFERENCES trip_options (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX trip_photos_option_idx ON trip_photos (option_id, created_at);

-- Each member's favourite option of a trip.
CREATE TABLE trip_votes (
    trip_id   uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    option_id uuid NOT NULL REFERENCES trip_options (id) ON DELETE CASCADE,
    PRIMARY KEY (trip_id, user_id)
);

CREATE TABLE trip_packing (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id    uuid NOT NULL REFERENCES trips (id) ON DELETE CASCADE,
    text       text NOT NULL,
    done       boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX trip_packing_trip_idx ON trip_packing (trip_id, created_at);

-- +goose Down
DROP TABLE trip_packing;
DROP TABLE trip_votes;
DROP TABLE trip_photos;
ALTER TABLE trips DROP CONSTRAINT trips_chosen_option_fk;
DROP TABLE trip_options;
DROP TABLE trips;
