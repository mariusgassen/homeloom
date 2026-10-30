-- +goose Up
-- The household's shared money. An expense is paid by one member and split into shares that add
-- up to it; a settlement is money one member gave another, with the receiver as its one share.
CREATE TABLE expenses (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('expense', 'settlement')),
    cents        bigint NOT NULL CHECK (cents > 0),
    category     text NOT NULL,
    note         text,
    paid_by      uuid NOT NULL REFERENCES users (id),
    day          date NOT NULL,
    added_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX expenses_household_day_idx ON expenses (household_id, day DESC);

CREATE TABLE expense_shares (
    expense_id uuid NOT NULL REFERENCES expenses (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id),
    cents      bigint NOT NULL CHECK (cents >= 0),
    PRIMARY KEY (expense_id, user_id)
);

-- +goose Down
DROP TABLE expense_shares;
DROP TABLE expenses;
