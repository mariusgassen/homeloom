-- name: ListDocuments :many
SELECT * FROM documents
WHERE household_id = @household_id AND (owner_id = @viewer OR shared)
ORDER BY created_at DESC, id
LIMIT @lim;

-- name: GetDocument :one
SELECT * FROM documents
WHERE id = @id AND household_id = @household_id AND (owner_id = @viewer OR shared);

-- name: CreateDocument :one
INSERT INTO documents (id, household_id, owner_id, name, content_type, size, shared)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateDocument :one
UPDATE documents SET name = @name, shared = @shared
WHERE id = @id AND household_id = @household_id AND owner_id = @owner_id
RETURNING *;

-- name: DeleteDocument :execrows
DELETE FROM documents WHERE id = @id AND household_id = @household_id AND owner_id = @owner_id;
