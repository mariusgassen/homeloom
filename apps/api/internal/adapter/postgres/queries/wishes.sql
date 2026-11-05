-- name: ListWishes :many
SELECT * FROM wishes WHERE household_id = @household_id ORDER BY created_at DESC, id LIMIT @lim;

-- name: CreateWish :one
INSERT INTO wishes (household_id, title, note, url, price_cents, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWish :one
SELECT * FROM wishes WHERE id = $1 AND household_id = $2;

-- name: UpdateWish :one
UPDATE wishes SET title = @title, note = @note, url = @url, price_cents = @price_cents
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: DeleteWish :execrows
DELETE FROM wishes WHERE id = $1 AND household_id = $2;

-- name: SetWishGot :one
UPDATE wishes SET got_at = @got_at WHERE id = @id AND household_id = @household_id RETURNING *;

-- name: SetWishPhoto :one
UPDATE wishes SET photo_at = @photo_at WHERE id = @id AND household_id = @household_id RETURNING *;

-- name: WishVotes :many
SELECT wish_id, user_id, hearts FROM wish_votes WHERE wish_id = ANY(@ids::uuid[]) ORDER BY wish_id, user_id;

-- name: VoteWish :execrows
INSERT INTO wish_votes (wish_id, user_id, hearts)
SELECT id, @user_id, @hearts FROM wishes WHERE id = @id AND household_id = @household_id
ON CONFLICT (wish_id, user_id) DO UPDATE SET hearts = EXCLUDED.hearts;

-- name: UnvoteWish :exec
DELETE FROM wish_votes WHERE wish_id = @id AND user_id = @user_id;
