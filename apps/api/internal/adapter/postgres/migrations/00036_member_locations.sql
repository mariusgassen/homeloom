-- +goose Up
-- Where a member's device last said they are, for their own weather. A member who doesn't share
-- their location has no row; the household's place is used instead.
CREATE TABLE member_locations (
    user_id    uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    latitude   double precision NOT NULL,
    longitude  double precision NOT NULL,
    place      text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE member_locations;
