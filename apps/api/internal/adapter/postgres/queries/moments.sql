-- name: CreateMoment :one
INSERT INTO moments (household_id, author_id, kind, day, visibility, reveal_at, text)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: SaveGratitude :one
-- A second line on the same day replaces the first.
INSERT INTO moments (household_id, author_id, kind, day, visibility, text)
VALUES (@household_id, @author_id, 'gratitude', @day, @visibility, @text)
ON CONFLICT (household_id, author_id, day) WHERE kind = 'gratitude'
DO UPDATE SET visibility = EXCLUDED.visibility, text = EXCLUDED.text, updated_at = now()
RETURNING *;

-- name: GetMoment :one
SELECT * FROM moments WHERE id = $1;

-- name: UpdateMoment :one
UPDATE moments SET day = $2, visibility = $3, reveal_at = $4, text = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMoment :exec
DELETE FROM moments WHERE id = $1;

-- name: ListMoments :many
-- Others' private moments never leave the database. Newest day first.
SELECT * FROM moments
WHERE household_id = @household_id
  AND (author_id = @viewer OR visibility <> 'private')
  AND day >= @from_day AND day < @to_day
ORDER BY day DESC, created_at DESC, id
LIMIT @lim;

-- name: LatestMomentDay :one
-- The newest day before @before_day that has a moment the viewer can see.
SELECT day FROM moments
WHERE household_id = @household_id
  AND (author_id = @viewer OR visibility <> 'private')
  AND day < @before_day
ORDER BY day DESC
LIMIT 1;

-- name: MomentsOnDay :many
-- What the viewer can see from this month and day in earlier years, newest first.
SELECT * FROM moments
WHERE household_id = @household_id
  AND (author_id = @viewer OR visibility <> 'private')
  AND extract(month FROM day) = extract(month FROM @today::date)
  AND extract(day FROM day) = extract(day FROM @today::date)
  AND day < @today
ORDER BY day DESC, created_at DESC
LIMIT @lim;

-- name: RandomGratitude :one
-- One gratitude line the viewer can see, from before @before_day, picked at random.
SELECT * FROM moments
WHERE household_id = @household_id
  AND kind = 'gratitude'
  AND (author_id = @viewer OR visibility <> 'private')
  AND day < @before_day
ORDER BY random()
LIMIT 1;

-- name: SetMomentPhoto :exec
WITH photo AS (
    INSERT INTO moment_photos (moment_id, content_type, data) VALUES (@moment_id, @content_type, @data)
    ON CONFLICT (moment_id) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data
)
UPDATE moments SET photo_at = now(), updated_at = now() WHERE id = @moment_id;

-- name: GetMomentPhoto :one
SELECT content_type, data FROM moment_photos WHERE moment_id = $1;

-- name: DeleteMomentPhoto :exec
WITH photo AS (
    DELETE FROM moment_photos WHERE moment_id = @moment_id
)
UPDATE moments SET photo_at = NULL, updated_at = now() WHERE id = @moment_id;

-- name: ListHouseholdIDs :many
SELECT id FROM households ORDER BY id;

-- name: ClaimJobRun :execrows
INSERT INTO job_runs (job, day) VALUES ($1, $2) ON CONFLICT DO NOTHING;
