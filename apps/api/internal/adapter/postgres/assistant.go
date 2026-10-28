package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateConversation(ctx context.Context, c domain.Conversation) (domain.Conversation, error) {
	row, err := s.q.CreateConversation(ctx, sqlcgen.CreateConversationParams{HouseholdID: c.HouseholdID, UserID: c.UserID, Title: c.Title})
	if err != nil {
		return domain.Conversation{}, err
	}
	return toConversation(row), nil
}

func (s *Store) GetConversation(ctx context.Context, id uuid.UUID) (domain.Conversation, error) {
	row, err := s.q.GetConversation(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Conversation{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Conversation{}, err
	}
	return toConversation(row), nil
}

func (s *Store) ListConversations(ctx context.Context, householdID, userID uuid.UUID, limit int) ([]domain.Conversation, error) {
	rows, err := s.q.ListConversations(ctx, sqlcgen.ListConversationsParams{
		HouseholdID: householdID, UserID: userID, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Conversation, len(rows))
	for i, r := range rows {
		out[i] = toConversation(r)
	}
	return out, nil
}

func (s *Store) DeleteConversation(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteConversation(ctx, id)
}

func (s *Store) AddChatMessage(ctx context.Context, m domain.ChatMessage) (domain.ChatMessage, error) {
	row, err := s.q.AddChatMessage(ctx, sqlcgen.AddChatMessageParams{ConversationID: m.ConversationID, Role: string(m.Role), Text: m.Text})
	if err != nil {
		return domain.ChatMessage{}, err
	}
	return toChatMessage(row), nil
}

func (s *Store) ChatMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]domain.ChatMessage, error) {
	rows, err := s.q.ChatMessages(ctx, sqlcgen.ChatMessagesParams{ConversationID: conversationID, Lim: int32(min(limit, 1000))}) //nolint:gosec // bounded above
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChatMessage, len(rows))
	for i, r := range rows {
		out[i] = toChatMessage(r)
	}
	return out, nil
}

func toConversation(r sqlcgen.Conversation) domain.Conversation {
	return domain.Conversation{ID: r.ID, HouseholdID: r.HouseholdID, UserID: r.UserID, Title: r.Title, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func toChatMessage(r sqlcgen.ChatMessage) domain.ChatMessage {
	return domain.ChatMessage{ID: r.ID, ConversationID: r.ConversationID, Role: domain.ChatRole(r.Role), Text: r.Text, CreatedAt: r.CreatedAt}
}
