-- +goose Up
-- Topics a member switched off. Every topic is on unless it has a row here.
CREATE TABLE push_muted_topics (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    topic   text NOT NULL,
    PRIMARY KEY (user_id, topic)
);

-- +goose Down
DROP TABLE push_muted_topics;
