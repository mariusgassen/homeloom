-- +goose Up
-- A picture a member set in Homeloom, which wins over the one authentik sends. Like moment
-- photos, the image is kept apart so reading users never reads it.
ALTER TABLE users ADD COLUMN avatar_at timestamptz;

CREATE TABLE user_avatars (
    user_id      uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    content_type text NOT NULL,
    data         bytea NOT NULL
);

-- +goose Down
DROP TABLE user_avatars;
ALTER TABLE users DROP COLUMN avatar_at;
