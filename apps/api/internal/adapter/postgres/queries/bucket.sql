-- name: ListBucket :many
-- Open items oldest first; ticked ones after, most recently ticked first. @kind '' means all kinds.
SELECT * FROM bucket_items
WHERE household_id = @household_id AND (@kind::text = '' OR kind = @kind::text)
ORDER BY done_at IS NOT NULL, done_at DESC, created_at, id
LIMIT @lim;

-- name: AddBucketItem :one
INSERT INTO bucket_items (household_id, kind, text, added_by) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: SetBucketItemDone :one
UPDATE bucket_items
SET done_at = CASE WHEN @done::boolean THEN coalesce(done_at, now()) END
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: DeleteBucketItem :execrows
DELETE FROM bucket_items WHERE id = $1 AND household_id = $2;
