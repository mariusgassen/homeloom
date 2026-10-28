-- name: ListShoppingItems :many
-- Open items first, oldest first; ticked ones after, most recently ticked first.
SELECT * FROM shopping_items
WHERE household_id = @household_id
ORDER BY done_at IS NOT NULL, done_at DESC, created_at, id
LIMIT @lim;

-- name: AddShoppingItem :one
-- clock_timestamp keeps the order of items added in one transaction.
INSERT INTO shopping_items (household_id, text, recipe, added_by, created_at) VALUES ($1, $2, $3, $4, clock_timestamp())
RETURNING *;

-- name: SetShoppingItemDone :one
UPDATE shopping_items
SET done_at = CASE WHEN @done::boolean THEN coalesce(done_at, now()) END
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: DeleteShoppingItem :execrows
DELETE FROM shopping_items WHERE id = $1 AND household_id = $2;

-- name: ClearDoneShoppingItems :exec
DELETE FROM shopping_items WHERE household_id = $1 AND done_at IS NOT NULL;
