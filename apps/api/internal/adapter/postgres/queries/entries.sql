-- name: CreateEntry :one
INSERT INTO entries (household_id, author_id, kind, visibility, reveal_at, text, due_at, starts_at, ends_at,
                     all_day, alert_minutes, repeat, assignee_id, notified_at, ical_uid, ical_name,
                     bill_cents, bill_category, bill_paid_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
RETURNING *;

-- name: GetEntry :one
SELECT * FROM entries WHERE id = $1;

-- name: GetEntryByICalName :one
SELECT * FROM entries WHERE household_id = $1 AND ical_name = $2;

-- name: ListEntries :many
-- Others' private entries never leave the database. Done reminders stay listed for 7 days, and
-- events while they end at or after @since (repeating events always).
SELECT * FROM entries
WHERE household_id = @household_id
  AND (author_id = @viewer OR visibility <> 'private')
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
  AND (done_at IS NULL OR done_at > now() - interval '7 days')
  AND (kind <> 'event' OR repeat <> 'never' OR coalesce(ends_at, starts_at) >= @since::timestamptz)
ORDER BY kind, done_at IS NOT NULL, coalesce(starts_at, due_at) NULLS LAST, created_at DESC, id
LIMIT @lim;

-- name: ReminderSuggestions :many
-- Titles of reminders the viewer can see, most used first, then most recent; a title with an
-- open reminder is left out.
SELECT (array_agg(text ORDER BY created_at DESC))[1]::text AS text, count(*)::integer AS uses
FROM entries
WHERE household_id = @household_id
  AND kind = 'reminder'
  AND (author_id = @viewer OR visibility <> 'private')
GROUP BY lower(text)
HAVING bool_and(done_at IS NOT NULL)
ORDER BY count(*) DESC, max(created_at) DESC
LIMIT @lim;

-- name: UpdateEntry :one
UPDATE entries
SET visibility = $2, reveal_at = $3, text = $4, due_at = $5, starts_at = $6, ends_at = $7, all_day = $8,
    alert_minutes = $9, repeat = $10, assignee_id = $11, done_at = $12, done_by = $13, notified_at = $14,
    bill_cents = $15, bill_category = $16, bill_paid_by = $17, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteEntry :exec
DELETE FROM entries WHERE id = $1;

-- name: ClaimDue :many
-- Marks open reminders and event alerts whose time has passed since their last notification,
-- skipping rows another instance is claiming, so each due time is notified once.
UPDATE entries SET notified_at = @now::timestamptz
WHERE id IN (
    SELECT e.id FROM entries e
    WHERE e.done_at IS NULL AND e.due_at <= @now
      AND (e.notified_at IS NULL OR e.notified_at < e.due_at)
    ORDER BY e.due_at
    LIMIT @lim
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: SaveCalendarFeed :exec
INSERT INTO calendar_feeds (token_hash, user_id, household_id) VALUES ($1, $2, $3)
ON CONFLICT (user_id, household_id) DO UPDATE SET token_hash = EXCLUDED.token_hash, created_at = now();

-- name: GetCalendarFeed :one
SELECT * FROM calendar_feeds WHERE token_hash = $1;

-- name: HasCalendarFeed :one
SELECT EXISTS (SELECT 1 FROM calendar_feeds WHERE user_id = $1 AND household_id = $2);

-- name: DeleteCalendarFeed :exec
DELETE FROM calendar_feeds WHERE user_id = $1 AND household_id = $2;

-- name: RenameEntry :execrows
UPDATE entries SET ical_name = $2, updated_at = now() WHERE id = $1;

-- name: AddEntryPhoto :one
INSERT INTO entry_photos (entry_id, content_type, data) VALUES ($1, $2, $3) RETURNING id;

-- name: GetEntryPhoto :one
SELECT content_type, data FROM entry_photos WHERE entry_id = $1 AND id = $2;

-- name: ListEntryPhotoIDs :many
SELECT id FROM entry_photos WHERE entry_id = $1 ORDER BY created_at, id;

-- name: DeleteEntryPhotos :exec
DELETE FROM entry_photos WHERE entry_id = @entry_id AND id = ANY(@ids::uuid[]);
