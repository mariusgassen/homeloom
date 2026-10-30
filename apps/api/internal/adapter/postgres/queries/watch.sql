-- name: UpsertWatchConnection :exec
INSERT INTO watch_connections (user_id, token) VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE SET token = EXCLUDED.token, created_at = now();

-- name: GetWatchConnection :one
SELECT * FROM watch_connections WHERE user_id = $1;

-- name: DeleteWatchConnection :exec
DELETE FROM watch_connections WHERE user_id = $1;

-- name: SetWatchLibraryHidden :execrows
UPDATE watch_connections SET hidden_libraries = CASE
    WHEN @hidden::boolean THEN array_append(array_remove(hidden_libraries, @library_id::text), @library_id::text)
    ELSE array_remove(hidden_libraries, @library_id::text)
END
WHERE user_id = @user_id;

-- name: UpsertTonight :exec
INSERT INTO watch_tonight (household_id, title_id, kind, name, year, url, picked_by, picked_at)
VALUES (@household_id, @title_id, @kind, @name, @year, @url, @picked_by, @picked_at)
ON CONFLICT (household_id) DO UPDATE SET title_id = EXCLUDED.title_id, kind = EXCLUDED.kind, name = EXCLUDED.name,
    year = EXCLUDED.year, url = EXCLUDED.url, picked_by = EXCLUDED.picked_by, picked_at = EXCLUDED.picked_at;

-- name: GetTonight :one
SELECT * FROM watch_tonight WHERE household_id = $1;

-- name: DeleteTonight :exec
DELETE FROM watch_tonight WHERE household_id = $1;
