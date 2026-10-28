-- +goose Up
-- A personal password can also be for a notes app, which reaches notes as files at /webdav.
ALTER TABLE mcp_tokens DROP CONSTRAINT mcp_tokens_access_check;
ALTER TABLE mcp_tokens ADD CONSTRAINT mcp_tokens_access_check CHECK (access IN ('tools', 'calendar', 'notes'));

-- +goose Down
DELETE FROM mcp_tokens WHERE access = 'notes';
ALTER TABLE mcp_tokens DROP CONSTRAINT mcp_tokens_access_check;
ALTER TABLE mcp_tokens ADD CONSTRAINT mcp_tokens_access_check CHECK (access IN ('tools', 'calendar'));
