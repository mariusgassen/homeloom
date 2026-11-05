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

-- name: ListMutedPushTopics :many
SELECT topic FROM push_muted_topics WHERE user_id = $1;

-- name: ListUsersMutingPushTopic :many
SELECT user_id FROM push_muted_topics WHERE topic = $1 AND user_id = ANY(sqlc.arg(user_ids)::uuid[]);

-- name: MutePushTopic :exec
INSERT INTO push_muted_topics (user_id, topic) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: UnmutePushTopic :exec
DELETE FROM push_muted_topics WHERE user_id = $1 AND topic = $2;
