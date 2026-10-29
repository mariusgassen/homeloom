-- name: UpsertUser :one
INSERT INTO users (authentik_sub, name, email, avatar_url)
VALUES ($1, $2, $3, $4)
ON CONFLICT (authentik_sub) DO UPDATE
SET name = EXCLUDED.name,
    email = EXCLUDED.email,
    avatar_url = EXCLUDED.avatar_url,
    updated_at = now()
RETURNING *;

-- name: CreateHousehold :one
INSERT INTO households (name) VALUES ($1) RETURNING *;

-- name: AddMember :exec
INSERT INTO memberships (household_id, user_id, role) VALUES ($1, $2, $3);

-- name: GetHousehold :one
SELECT * FROM households WHERE id = $1;

-- name: GetMembershipRole :one
SELECT role FROM memberships WHERE household_id = $1 AND user_id = $2;

-- name: ListHouseholdsForUser :many
SELECT h.id, h.name, m.role
FROM households h
JOIN memberships m ON m.household_id = h.id
WHERE m.user_id = $1
ORDER BY h.created_at, h.id;

-- name: SetUserAvatar :one
WITH avatar AS (
    INSERT INTO user_avatars (user_id, content_type, data) VALUES (@user_id, @content_type, @data)
    ON CONFLICT (user_id) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data
)
UPDATE users SET avatar_at = now(), updated_at = now() WHERE id = @user_id
RETURNING *;

-- name: GetUserAvatar :one
SELECT content_type, data FROM user_avatars WHERE user_id = $1;

-- name: DeleteUserAvatar :one
WITH avatar AS (
    DELETE FROM user_avatars WHERE user_id = @user_id
)
UPDATE users SET avatar_at = NULL, updated_at = now() WHERE id = @user_id
RETURNING *;

-- name: SharesHousehold :one
SELECT EXISTS (
    SELECT 1 FROM memberships a
    JOIN memberships b ON b.household_id = a.household_id
    WHERE a.user_id = @a AND b.user_id = @b
);

-- name: ListMembers :many
SELECT u.id, u.name, u.email, u.avatar_url, u.avatar_at, m.role
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.household_id = $1
ORDER BY m.created_at, u.id;
