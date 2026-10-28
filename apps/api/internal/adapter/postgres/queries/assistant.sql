-- name: CreateConversation :one
INSERT INTO conversations (household_id, user_id, title) VALUES ($1, $2, $3) RETURNING *;

-- name: GetConversation :one
SELECT * FROM conversations WHERE id = $1;

-- name: ListConversations :many
SELECT * FROM conversations
WHERE household_id = @household_id AND user_id = @user_id
ORDER BY updated_at DESC, id
LIMIT @lim;

-- name: DeleteConversation :exec
DELETE FROM conversations WHERE id = $1;

-- name: AddChatMessage :one
WITH touched AS (
    UPDATE conversations SET updated_at = now() WHERE id = @conversation_id
)
INSERT INTO chat_messages (conversation_id, role, text) VALUES (@conversation_id, @role, @text) RETURNING *;

-- name: ChatMessages :many
-- The last @lim messages, oldest first.
SELECT * FROM (
    SELECT * FROM chat_messages WHERE conversation_id = @conversation_id ORDER BY created_at DESC, id DESC LIMIT @lim
) last ORDER BY created_at, id;
