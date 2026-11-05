-- +goose Up
-- The household's someday list: things to watch, read or visit some day. A ticked item is done
-- (watched, read, been there) and stays listed until deleted.
CREATE TABLE bucket_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    kind         text NOT NULL CHECK (kind IN ('watch', 'read', 'visit')),
    text         text NOT NULL,
    added_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    done_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX bucket_items_household_kind_idx ON bucket_items (household_id, kind);

-- +goose Down
DROP TABLE bucket_items;
