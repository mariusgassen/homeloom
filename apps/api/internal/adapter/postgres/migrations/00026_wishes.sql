-- +goose Up
-- The household's wishlist: things wanted for the home. photo_at is when a photo was kept in S3;
-- got_at is when the wish came true.
CREATE TABLE wishes (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    title        text NOT NULL,
    note         text NOT NULL DEFAULT '',
    url          text NOT NULL DEFAULT '',
    price_cents  bigint CHECK (price_cents > 0),
    photo_at     timestamptz,
    got_at       timestamptz,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX wishes_household_idx ON wishes (household_id, created_at DESC);

-- How much each member wants a wish, 1 to 3 hearts. The wishes are ranked by the sum.
CREATE TABLE wish_votes (
    wish_id uuid NOT NULL REFERENCES wishes (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    hearts  integer NOT NULL CHECK (hearts BETWEEN 1 AND 3),
    PRIMARY KEY (wish_id, user_id)
);

-- +goose Down
DROP TABLE wish_votes;
DROP TABLE wishes;
