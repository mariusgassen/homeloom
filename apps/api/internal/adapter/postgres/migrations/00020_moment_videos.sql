-- +goose Up
-- video_at is when the moment's video was last set; NULL means it has none. Videos are kept in the
-- S3 store, never here.
ALTER TABLE moments ADD COLUMN video_at timestamptz;

-- +goose Down
ALTER TABLE moments DROP COLUMN video_at;
