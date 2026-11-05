-- name: ListTrips :many
-- Soonest start first, trips without a start last.
SELECT * FROM trips WHERE household_id = @household_id
ORDER BY start_on NULLS LAST, created_at, id
LIMIT @lim;

-- name: GetTrip :one
SELECT * FROM trips WHERE id = @id AND household_id = @household_id;

-- name: CreateTrip :one
INSERT INTO trips (household_id, name, destination, start_on, end_on, notes, created_by)
VALUES (@household_id, @name, @destination, @start_on, @end_on, @notes, @created_by)
RETURNING *;

-- name: UpdateTrip :one
UPDATE trips SET name = @name, destination = @destination, start_on = @start_on, end_on = @end_on,
    notes = @notes, chosen_option_id = @chosen_option_id
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: DeleteTrip :execrows
DELETE FROM trips WHERE id = @id AND household_id = @household_id;

-- name: ListTripOptions :many
SELECT o.* FROM trip_options o JOIN trips t ON t.id = o.trip_id
WHERE t.household_id = @household_id
ORDER BY o.created_at, o.id;

-- name: ListTripOptionsOf :many
SELECT * FROM trip_options WHERE trip_id = @trip_id ORDER BY created_at, id;

-- name: CreateTripOption :one
INSERT INTO trip_options (trip_id, title, url, notes, added_by)
VALUES (@trip_id, @title, @url, @notes, @added_by)
RETURNING *;

-- name: UpdateTripOption :execrows
UPDATE trip_options SET title = @title, url = @url, notes = @notes
WHERE id = @id AND trip_id = @trip_id;

-- name: DeleteTripOption :execrows
DELETE FROM trip_options WHERE id = @id AND trip_id = @trip_id;

-- name: ListTripPhotos :many
SELECT p.* FROM trip_photos p
JOIN trip_options o ON o.id = p.option_id
JOIN trips t ON t.id = o.trip_id
WHERE t.household_id = @household_id
ORDER BY p.created_at, p.id;

-- name: ListTripPhotosOf :many
SELECT p.* FROM trip_photos p JOIN trip_options o ON o.id = p.option_id
WHERE o.trip_id = @trip_id
ORDER BY p.created_at, p.id;

-- name: CreateTripPhoto :one
INSERT INTO trip_photos (option_id) VALUES (@option_id) RETURNING *;

-- name: DeleteTripPhoto :execrows
DELETE FROM trip_photos WHERE id = @id AND option_id = @option_id;

-- name: ListTripVotes :many
SELECT v.* FROM trip_votes v JOIN trips t ON t.id = v.trip_id
WHERE t.household_id = @household_id
ORDER BY v.trip_id, v.user_id;

-- name: ListTripVotesOf :many
SELECT * FROM trip_votes WHERE trip_id = @trip_id ORDER BY user_id;

-- name: SetTripVote :exec
INSERT INTO trip_votes (trip_id, user_id, option_id) VALUES (@trip_id, @user_id, @option_id)
ON CONFLICT (trip_id, user_id) DO UPDATE SET option_id = EXCLUDED.option_id;

-- name: ClearTripVote :exec
DELETE FROM trip_votes WHERE trip_id = @trip_id AND user_id = @user_id;

-- name: ListTripPacking :many
SELECT p.* FROM trip_packing p JOIN trips t ON t.id = p.trip_id
WHERE t.household_id = @household_id
ORDER BY p.created_at, p.id;

-- name: ListTripPackingOf :many
SELECT * FROM trip_packing WHERE trip_id = @trip_id ORDER BY created_at, id;

-- name: CreatePackingItem :one
INSERT INTO trip_packing (trip_id, text) VALUES (@trip_id, @text) RETURNING *;

-- name: UpdatePackingItem :execrows
UPDATE trip_packing SET text = @text, done = @done WHERE id = @id AND trip_id = @trip_id;

-- name: DeletePackingItem :execrows
DELETE FROM trip_packing WHERE id = @id AND trip_id = @trip_id;
