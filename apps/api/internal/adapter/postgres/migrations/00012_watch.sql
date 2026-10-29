-- +goose Up
-- Each person's Plex sign-in. token is sealed by the API (secretbox) before it gets here.
CREATE TABLE watch_connections (
    user_id    uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    token      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE watch_connections;
