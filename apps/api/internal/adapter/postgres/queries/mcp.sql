-- name: CreateMCPToken :one
INSERT INTO mcp_tokens (id, token_hash, user_id, household_id, name, access)
VALUES (coalesce(sqlc.narg(id)::uuid, gen_random_uuid()), @token_hash, @user_id, @household_id, @name, @access)
RETURNING id, user_id, household_id, name, access, created_at;

-- name: ListMCPTokens :many
SELECT id, user_id, household_id, name, access, created_at FROM mcp_tokens
WHERE household_id = $1 AND user_id = $2
ORDER BY created_at DESC;

-- name: DeleteMCPToken :execrows
DELETE FROM mcp_tokens WHERE id = $1 AND user_id = $2;

-- name: GetMCPTokenByHash :one
SELECT t.id, t.user_id, t.household_id, t.name, t.access, t.created_at, u.name AS user_name, u.email, u.avatar_url, u.avatar_at
FROM mcp_tokens t JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1;

-- name: GetMCPTokenByID :one
SELECT t.id, t.user_id, t.household_id, t.name, t.access, t.created_at, t.token_hash, u.name AS user_name, u.email, u.avatar_url, u.avatar_at
FROM mcp_tokens t JOIN users u ON u.id = t.user_id
WHERE t.id = $1;
