-- +goose Up
-- A conversation is one person's chat with the assistant in a household; only that person sees it.
CREATE TABLE conversations (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX conversations_user_idx ON conversations (household_id, user_id, updated_at DESC);

CREATE TABLE chat_messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('user', 'assistant')),
    text            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX chat_messages_conversation_idx ON chat_messages (conversation_id, created_at);

-- +goose Down
DROP TABLE chat_messages;
DROP TABLE conversations;
