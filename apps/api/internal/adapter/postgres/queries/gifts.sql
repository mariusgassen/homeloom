-- name: SetBirthday :exec
INSERT INTO birthdays (user_id, month, day) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET month = excluded.month, day = excluded.day;

-- name: ClearBirthday :exec
DELETE FROM birthdays WHERE user_id = $1;

-- name: HouseholdBirthdays :many
SELECT b.user_id, b.month, b.day FROM birthdays b
JOIN memberships m ON m.user_id = b.user_id
WHERE m.household_id = $1;

-- name: ListGiftIdeas :many
-- Only the author's own ideas, ticked ones last.
SELECT * FROM gift_ideas
WHERE household_id = @household_id AND author_id = @author_id
ORDER BY given_at IS NOT NULL, created_at DESC, id
LIMIT @lim;

-- name: AddGiftIdea :one
INSERT INTO gift_ideas (household_id, author_id, for_id, text) VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: SetGiftIdeaGiven :one
UPDATE gift_ideas
SET given_at = CASE WHEN @given::boolean THEN coalesce(given_at, now()) END
WHERE id = @id AND household_id = @household_id AND author_id = @author_id
RETURNING *;

-- name: DeleteGiftIdea :execrows
DELETE FROM gift_ideas WHERE id = $1 AND household_id = $2 AND author_id = $3;
