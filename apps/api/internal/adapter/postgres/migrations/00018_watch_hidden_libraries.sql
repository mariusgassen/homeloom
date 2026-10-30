-- +goose Up
-- The libraries a person left out of Movies & shows and of ideas, by Plex library id. Every library
-- counts until it is listed here, so a new one shows up on its own. Disconnecting forgets the list.
ALTER TABLE watch_connections ADD COLUMN hidden_libraries text[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE watch_connections DROP COLUMN hidden_libraries;
