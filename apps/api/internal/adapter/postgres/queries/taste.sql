-- name: UpsertMealRating :exec
INSERT INTO meal_ratings (household_id, user_id, recipe_slug, recipe_name, rating)
VALUES (@household_id, @user_id, @recipe_slug, @recipe_name, @rating)
ON CONFLICT (household_id, user_id, recipe_slug)
DO UPDATE SET recipe_name = EXCLUDED.recipe_name, rating = EXCLUDED.rating, rated_at = now();

-- name: DeleteMealRating :exec
DELETE FROM meal_ratings WHERE household_id = $1 AND user_id = $2 AND recipe_slug = $3;

-- name: ListMealRatings :many
SELECT * FROM meal_ratings WHERE household_id = $1 ORDER BY rated_at DESC;
