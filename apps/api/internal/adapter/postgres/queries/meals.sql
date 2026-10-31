-- name: UpsertMeal :exec
INSERT INTO meal_plan (household_id, day, recipe_slug, recipe_name, recipe_image, total_time, planned_by)
VALUES (@household_id, @day, @recipe_slug, @recipe_name, @recipe_image, @total_time, @planned_by)
ON CONFLICT (household_id, day) DO UPDATE SET recipe_slug = EXCLUDED.recipe_slug, recipe_name = EXCLUDED.recipe_name,
    recipe_image = EXCLUDED.recipe_image, total_time = EXCLUDED.total_time, planned_by = EXCLUDED.planned_by, planned_at = now();

-- name: ListMeals :many
SELECT * FROM meal_plan WHERE household_id = @household_id AND day >= @from_day AND day < @to_day ORDER BY day;

-- name: DeleteMeal :exec
DELETE FROM meal_plan WHERE household_id = $1 AND day = $2;
