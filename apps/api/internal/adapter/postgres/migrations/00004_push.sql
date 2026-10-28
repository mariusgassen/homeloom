-- +goose Up
-- One row per device that gets notifications. lang is the language its notifications are written in.
CREATE TABLE push_subscriptions (
    endpoint   text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    p256dh     text NOT NULL,
    auth       text NOT NULL,
    lang       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX push_subscriptions_user_id ON push_subscriptions (user_id);

-- +goose Down
DROP TABLE push_subscriptions;
