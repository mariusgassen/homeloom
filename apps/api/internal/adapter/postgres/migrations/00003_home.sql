-- +goose Up
-- Each person's Home Assistant sign-in. refresh_token is sealed by the API (secretbox) before it gets here.
CREATE TABLE home_connections (
    user_id       uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    refresh_token text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE home_connections;
