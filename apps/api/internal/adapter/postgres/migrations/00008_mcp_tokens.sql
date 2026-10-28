-- +goose Up
-- A personal token lets another app use the tools at /mcp as one member in one household, until
-- it is deleted. Only a hash of it is stored.
CREATE TABLE mcp_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash   bytea NOT NULL UNIQUE,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    name         text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX mcp_tokens_user_idx ON mcp_tokens (household_id, user_id, created_at DESC);

-- +goose Down
DROP TABLE mcp_tokens;
