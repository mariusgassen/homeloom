-- +goose Up
-- Each item sits in an aisle, so the list reads in the order of a walk through the shop.
ALTER TABLE shopping_items ADD COLUMN aisle text NOT NULL DEFAULT 'other'
    CHECK (aisle IN ('produce', 'bakery', 'meat', 'dairy', 'frozen', 'pantry', 'drinks', 'household', 'other'));

-- Which aisle a household puts a thing in, by its lower-cased text: what Claude chose, or where a
-- member moved it. It outlives the items, so the same thing lands in the same aisle next time.
CREATE TABLE shopping_aisles (
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name         text NOT NULL,
    aisle        text NOT NULL
        CHECK (aisle IN ('produce', 'bakery', 'meat', 'dairy', 'frozen', 'pantry', 'drinks', 'household', 'other')),
    PRIMARY KEY (household_id, name)
);

-- +goose Down
DROP TABLE shopping_aisles;
ALTER TABLE shopping_items DROP COLUMN aisle;
