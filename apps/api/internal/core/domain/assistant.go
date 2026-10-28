package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	MaxChatText = 2000
	// MaxChatTitle is how much of the first message names a conversation.
	MaxChatTitle = 60
)

// ChatRole is who wrote a chat message: the person or the assistant.
type ChatRole string

const (
	RoleUser      ChatRole = "user"
	RoleAssistant ChatRole = "assistant"
)

// Conversation is one person's chat with the assistant in a household. Only that person sees it.
type Conversation struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	UserID      uuid.UUID
	Title       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ChatMessage struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Role           ChatRole
	Text           string
	CreatedAt      time.Time
}

// Tool is something the assistant may do for the person it talks to. Schema is the JSON Schema
// of its input.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall is the assistant asking for a tool.
type ToolCall struct {
	Name  string
	Input json.RawMessage
}

// ChatEvent is what the assistant streams while it answers: a piece of its text, or the name of a
// tool it is about to use.
type ChatEvent struct {
	Text string
	Tool string
}

// AssistantBrief is what the assistant knows about whom it talks to and when.
type AssistantBrief struct {
	// Lang is the BCP 47 tag of the language to answer in.
	Lang      string
	Now       time.Time
	Location  *time.Location
	Me        string
	Household string
	// Others are the names of the other members.
	Others []string
}
