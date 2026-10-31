-- +goose Up
-- Each household's dinners, one per day. The recipe's name, picture version and time are kept, so
-- the plan stays readable while Mealie is down.
CREATE TABLE meal_plan (
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    day          date NOT NULL,
    recipe_slug  text NOT NULL,
    recipe_name  text NOT NULL,
    recipe_image text NOT NULL DEFAULT '',
    total_time   text NOT NULL DEFAULT '',
    planned_by   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    planned_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, day)
);

-- +goose Down
DROP TABLE meal_plan;
