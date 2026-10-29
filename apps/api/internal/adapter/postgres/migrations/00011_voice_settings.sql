-- +goose Up
-- How Homeloom sounds on each member's calls with it; members without a row get the defaults.
CREATE TABLE voice_settings (
    user_id    uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    voice      text NOT NULL,
    speed      double precision NOT NULL,
    tone       text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE voice_settings;
