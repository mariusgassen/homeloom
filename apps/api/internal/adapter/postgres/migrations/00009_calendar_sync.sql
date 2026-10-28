-- +goose Up
-- A personal token is either for the assistant's tools at /mcp or a password for calendar apps
-- at /dav; one never works for the other.
ALTER TABLE mcp_tokens ADD COLUMN access text NOT NULL DEFAULT 'tools' CHECK (access IN ('tools', 'calendar'));

-- An event or reminder a calendar app made keeps the UID and resource name the app gave it, so
-- the app finds it again where it put it.
ALTER TABLE entries ADD COLUMN ical_uid text, ADD COLUMN ical_name text;
CREATE UNIQUE INDEX entries_ical_name_idx ON entries (household_id, ical_name) WHERE ical_name IS NOT NULL;

-- +goose Down
DROP INDEX entries_ical_name_idx;
ALTER TABLE entries DROP COLUMN ical_name, DROP COLUMN ical_uid;
ALTER TABLE mcp_tokens DROP COLUMN access;
