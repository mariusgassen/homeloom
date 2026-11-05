-- +goose Up
-- Taste profiles: how each member rated a dish. The name is kept, so a rating stays readable
-- while Mealie is down.
CREATE TABLE meal_ratings (
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    recipe_slug  text NOT NULL,
    recipe_name  text NOT NULL,
    rating       text NOT NULL CHECK (rating IN ('love', 'fine', 'nope')),
    rated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id, recipe_slug)
);

-- Meal memories: a moment can be about a dish.
ALTER TABLE moments
    ADD COLUMN recipe_slug text NOT NULL DEFAULT '',
    ADD COLUMN recipe_name text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE moments DROP COLUMN recipe_name, DROP COLUMN recipe_slug;
DROP TABLE meal_ratings;
