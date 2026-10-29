-- +goose Up
-- Photos in notes. A note's Markdown links each one by its id; they go with the note.
CREATE TABLE entry_photos (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entry_id     uuid NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
    content_type text NOT NULL,
    data         bytea NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entry_photos_entry_idx ON entry_photos (entry_id);

-- +goose Down
DROP TABLE entry_photos;
