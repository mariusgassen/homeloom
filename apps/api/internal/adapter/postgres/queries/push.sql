-- name: UpsertPushSubscription :exec
-- A browser that moves to another account takes its endpoint with it.
INSERT INTO push_subscriptions (endpoint, user_id, p256dh, auth, lang) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (endpoint) DO UPDATE
SET user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, lang = EXCLUDED.lang, created_at = now();

-- name: ListPushSubscriptions :many
SELECT * FROM push_subscriptions WHERE user_id = ANY(sqlc.arg(user_ids)::uuid[]) ORDER BY created_at;

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE endpoint = $1 AND user_id = $2;

-- name: DeletePushEndpoint :exec
DELETE FROM push_subscriptions WHERE endpoint = $1;
