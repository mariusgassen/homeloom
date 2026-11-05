-- +goose Up
-- A member answers a moment or gratitude line they can read with one emoji; one per person.
CREATE TABLE moment_reactions (
    moment_id  uuid NOT NULL REFERENCES moments (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    emoji      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (moment_id, user_id)
);

-- +goose Down
DROP TABLE moment_reactions;
