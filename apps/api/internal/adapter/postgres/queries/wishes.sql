-- name: ListWishes :many
-- Open wishes oldest first; ticked ones after, most recently ticked first. @kind '' means all kinds.
SELECT * FROM wishes
WHERE household_id = @household_id AND (@kind::text = '' OR kind = @kind::text)
ORDER BY done_at IS NOT NULL, done_at DESC, created_at, id
LIMIT @lim;

-- name: AddWish :one
INSERT INTO wishes (household_id, kind, text, added_by) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: SetWishDone :one
UPDATE wishes
SET done_at = CASE WHEN @done::boolean THEN coalesce(done_at, now()) END
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: DeleteWish :execrows
DELETE FROM wishes WHERE id = $1 AND household_id = $2;
