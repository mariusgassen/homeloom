-- +goose Up
-- The household's shared shopping list. recipe is the name of the recipe an item came from.
CREATE TABLE shopping_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    text         text NOT NULL,
    recipe       text,
    added_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    done_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX shopping_items_household_idx ON shopping_items (household_id);

-- +goose Down
DROP TABLE shopping_items;
