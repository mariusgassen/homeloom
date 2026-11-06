-- name: UpsertMemberLocation :exec
INSERT INTO member_locations (user_id, latitude, longitude, place) VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE
SET latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude, place = EXCLUDED.place, updated_at = now();

-- name: DeleteMemberLocation :exec
DELETE FROM member_locations WHERE user_id = $1;

-- name: ListMemberLocations :many
SELECT * FROM member_locations WHERE user_id = ANY(sqlc.arg(user_ids)::uuid[]);
