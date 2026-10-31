-- +goose Up
-- The household's chores. due_on is the day it is due in the household's time zone; a chore that
-- doesn't repeat gets done_at once done and is no longer listed. assignee_id NULL means anyone.
CREATE TABLE chores (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    title        text NOT NULL,
    points       integer NOT NULL CHECK (points BETWEEN 1 AND 3),
    repeat       text NOT NULL CHECK (repeat IN ('never', 'daily', 'weekly', 'monthly', 'yearly')),
    rotate       boolean NOT NULL DEFAULT false,
    assignee_id  uuid REFERENCES users (id) ON DELETE SET NULL,
    due_on       date NOT NULL,
    done_at      timestamptz,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX chores_household_due_idx ON chores (household_id, due_on);

-- Each time someone did a chore. Title and points are kept, so history and fairness outlive the
-- chore; prev_* is how the chore was before, so the latest completion can be undone.
CREATE TABLE chore_completions (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id     uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    chore_id         uuid REFERENCES chores (id) ON DELETE SET NULL,
    title            text NOT NULL,
    points           integer NOT NULL,
    user_id          uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day              date NOT NULL,
    done_at          timestamptz NOT NULL DEFAULT clock_timestamp(),
    prev_due_on      date NOT NULL,
    prev_assignee_id uuid REFERENCES users (id) ON DELETE SET NULL
);

CREATE INDEX chore_completions_household_day_idx ON chore_completions (household_id, day DESC);
CREATE INDEX chore_completions_chore_idx ON chore_completions (chore_id, done_at DESC);

CREATE TABLE chore_thanks (
    completion_id uuid NOT NULL REFERENCES chore_completions (id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (completion_id, user_id)
);

-- +goose Down
DROP TABLE chore_thanks;
DROP TABLE chore_completions;
DROP TABLE chores;
