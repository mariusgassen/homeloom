-- name: CreateInvite :exec
INSERT INTO invites (code_hash, household_id, created_by, expires_at) VALUES ($1, $2, $3, $4);

-- name: LockOpenInvite :one
SELECT household_id FROM invites
WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
FOR UPDATE;

-- name: UseInvite :exec
UPDATE invites SET used_by = $2, used_at = now() WHERE code_hash = $1;

-- name: CreateQuestion :one
INSERT INTO questions (household_id, asked_by, text) VALUES ($1, $2, $3) RETURNING *;

-- name: GetQuestion :one
SELECT * FROM questions WHERE id = $1;

-- name: LockQuestion :one
SELECT * FROM questions WHERE id = $1 FOR UPDATE;

-- name: ListQuestions :many
SELECT * FROM questions WHERE household_id = $1 ORDER BY created_at DESC, id LIMIT $2;

-- name: ListAnswers :many
SELECT * FROM answers WHERE question_id = ANY(@question_ids::uuid[]) ORDER BY answered_at, user_id;

-- name: UpsertAnswer :exec
INSERT INTO answers (question_id, user_id, text) VALUES ($1, $2, $3)
ON CONFLICT (question_id, user_id) DO UPDATE SET text = EXCLUDED.text, answered_at = now();

-- name: AnswerProgress :one
SELECT count(*) AS members, count(a.user_id) AS answered
FROM memberships m
LEFT JOIN answers a ON a.question_id = @question_id AND a.user_id = m.user_id
WHERE m.household_id = @household_id;

-- name: RevealQuestion :exec
UPDATE questions SET revealed_at = now() WHERE id = $1;
