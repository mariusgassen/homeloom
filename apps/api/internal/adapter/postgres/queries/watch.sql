-- name: UpsertWatchConnection :exec
INSERT INTO watch_connections (user_id, token) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET token = EXCLUDED.token, created_at = now();

-- name: GetWatchConnection :one
SELECT * FROM watch_connections WHERE user_id = $1;

-- name: DeleteWatchConnection :exec
DELETE FROM watch_connections WHERE user_id = $1;
