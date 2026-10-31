-- name: ListChores :many
SELECT * FROM chores
WHERE household_id = @household_id AND done_at IS NULL
ORDER BY due_on, created_at, id
LIMIT @lim;

-- name: CreateChore :one
INSERT INTO chores (household_id, title, points, repeat, rotate, assignee_id, due_on, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetChore :one
SELECT * FROM chores WHERE id = $1 AND household_id = $2;

-- name: LockChore :one
SELECT * FROM chores WHERE id = $1 AND household_id = $2 FOR UPDATE;

-- name: UpdateChore :one
UPDATE chores
SET title = @title, points = @points, repeat = @repeat, rotate = @rotate, assignee_id = @assignee_id, due_on = @due_on
WHERE id = @id AND household_id = @household_id
RETURNING *;

-- name: SetChoreState :exec
UPDATE chores SET due_on = @due_on, assignee_id = @assignee_id, done_at = @done_at WHERE id = @id;

-- name: DeleteChore :execrows
DELETE FROM chores WHERE id = $1 AND household_id = $2;

-- name: AddChoreCompletion :one
INSERT INTO chore_completions (household_id, chore_id, title, points, user_id, day, prev_due_on, prev_assignee_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: LastChoreCompletion :one
SELECT * FROM chore_completions
WHERE household_id = @household_id AND chore_id = @chore_id
ORDER BY done_at DESC, id DESC
LIMIT 1;

-- name: GetChoreCompletion :one
SELECT * FROM chore_completions WHERE id = $1 AND household_id = $2;

-- name: DeleteChoreCompletion :exec
DELETE FROM chore_completions WHERE id = $1;

-- name: ListChoreCompletions :many
SELECT * FROM chore_completions
WHERE household_id = @household_id AND day >= @since
ORDER BY done_at DESC, id DESC
LIMIT @lim;

-- name: ChoreThanks :many
SELECT completion_id, user_id FROM chore_thanks
WHERE completion_id = ANY(@ids::uuid[])
ORDER BY created_at, user_id;

-- name: ThankChoreCompletion :execrows
INSERT INTO chore_thanks (completion_id, user_id)
SELECT id, @user_id FROM chore_completions WHERE id = @id AND household_id = @household_id
ON CONFLICT DO NOTHING;
