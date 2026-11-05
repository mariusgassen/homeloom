-- +goose Up
-- A member's birthday, as month and day; the year of birth isn't kept.
CREATE TABLE birthdays (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    month   smallint NOT NULL CHECK (month BETWEEN 1 AND 12),
    day     smallint NOT NULL CHECK (day BETWEEN 1 AND 31)
);

-- Gift ideas a member keeps for another member. They are private to the author: every query
-- filters on author_id, and nothing reads them for anyone else.
CREATE TABLE gift_ideas (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    author_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    for_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    text         text NOT NULL,
    given_at     timestamptz,
    created_at   timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX gift_ideas_author_idx ON gift_ideas (author_id, household_id, created_at);

-- +goose Down
DROP TABLE gift_ideas;
DROP TABLE birthdays;
