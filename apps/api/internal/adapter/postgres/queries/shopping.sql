-- name: ListShoppingItems :many
-- Open items first, oldest first; ticked ones after, most recently ticked first.
SELECT * FROM shopping_items
WHERE household_id = @household_id
ORDER BY done_at IS NOT NULL, done_at DESC, created_at, id
LIMIT @lim;

-- name: AddShoppingItem :one
-- clock_timestamp keeps the order of items added in one transaction.
INSERT INTO shopping_items (household_id, text, recipe, added_by, aisle, created_at) VALUES ($1, $2, $3, $4, $5, clock_timestamp())
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

-- name: RecordShoppingHistory :exec
INSERT INTO shopping_history (household_id, key, text) VALUES (@household_id, lower(@text::text), @text::text)
ON CONFLICT (household_id, key) DO UPDATE SET text = excluded.text, uses = shopping_history.uses + 1, last_used = now();

-- name: ShoppingSuggestions :many
-- Most used first, then most recent; what's open on the list is left out.
SELECT h.text, h.uses FROM shopping_history h
WHERE h.household_id = @household_id
  AND NOT EXISTS (
    SELECT 1 FROM shopping_items i
    WHERE i.household_id = h.household_id AND lower(i.text) = h.key AND i.done_at IS NULL
  )
ORDER BY h.uses DESC, h.last_used DESC
LIMIT @lim;

-- name: SetShoppingItemAisle :one
UPDATE shopping_items SET aisle = @aisle
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: KnownShoppingAisles :many
SELECT name, aisle FROM shopping_aisles WHERE household_id = @household_id AND name = ANY(@names::text[]);

-- name: RememberShoppingAisle :exec
INSERT INTO shopping_aisles (household_id, name, aisle) VALUES ($1, $2, $3)
ON CONFLICT (household_id, name) DO UPDATE SET aisle = excluded.aisle;
