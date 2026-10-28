-- +goose Up
CREATE TABLE invites (
    code_hash    bytea PRIMARY KEY,
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    created_by   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at   timestamptz NOT NULL,
    used_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    used_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE questions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    asked_by     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    text         text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revealed_at  timestamptz
);

CREATE INDEX questions_household_created_idx ON questions (household_id, created_at DESC);

CREATE TABLE answers (
    question_id uuid NOT NULL REFERENCES questions (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    text        text NOT NULL,
    answered_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (question_id, user_id)
);

-- +goose Down
DROP TABLE answers;
DROP TABLE questions;
DROP TABLE invites;
