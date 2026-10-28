-- name: UpsertHomeConnection :exec
INSERT INTO home_connections (user_id, refresh_token) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET refresh_token = EXCLUDED.refresh_token, created_at = now();

-- name: GetHomeConnection :one
SELECT * FROM home_connections WHERE user_id = $1;

-- name: DeleteHomeConnection :exec
DELETE FROM home_connections WHERE user_id = $1;
