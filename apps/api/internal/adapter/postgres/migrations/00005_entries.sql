-- +goose Up
-- Entries are what members write for themselves or each other: notes, reminders and calendar
-- events. Every kind shares the visibility model.
CREATE TABLE entries (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    author_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('note', 'reminder', 'event')),
    visibility    text NOT NULL CHECK (visibility IN ('private', 'shared', 'reveal')),
    reveal_at     timestamptz,
    text          text NOT NULL,
    -- due_at is when a reminder is due, or when an event's alert goes out.
    due_at        timestamptz,
    starts_at     timestamptz,
    ends_at       timestamptz,
    all_day       boolean NOT NULL DEFAULT false,
    alert_minutes integer,
    repeat        text NOT NULL DEFAULT 'never' CHECK (repeat IN ('never', 'daily', 'weekly', 'monthly', 'yearly')),
    assignee_id   uuid REFERENCES users (id) ON DELETE SET NULL,
    done_at       timestamptz,
    done_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    -- notified_at is when the last notification went out; one is due again once due_at passes it.
    notified_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((visibility = 'reveal') = (reveal_at IS NOT NULL)),
    CHECK ((kind = 'event') = (starts_at IS NOT NULL)),
    CHECK (ends_at IS NULL OR ends_at >= starts_at)
);

CREATE INDEX entries_household_kind_idx ON entries (household_id, kind);
CREATE INDEX entries_due_idx ON entries (due_at) WHERE done_at IS NULL;

-- A calendar feed lets a calendar app subscribe to what one member can see. Only a hash of its
-- token is stored.
CREATE TABLE calendar_feeds (
    token_hash   bytea PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, household_id)
);

-- +goose Down
DROP TABLE calendar_feeds;
DROP TABLE entries;
