package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateConversation(_ context.Context, c domain.Conversation) (domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	c.ID, c.CreatedAt, c.UpdatedAt = uuid.New(), now, now
	s.conversations = append(s.conversations, c)
	return c, nil
}

func (s *Store) GetConversation(_ context.Context, id uuid.UUID) (domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conversations {
		if c.ID == id {
			return c, nil
		}
	}
	return domain.Conversation{}, domain.ErrNotFound
}

func (s *Store) ListConversations(_ context.Context, householdID, userID uuid.UUID, limit int) ([]domain.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Conversation{}
	for _, c := range s.conversations {
		if c.HouseholdID == householdID && c.UserID == userID {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.Conversation) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	return out[:min(limit, len(out))], nil
}

func (s *Store) DeleteConversation(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conversations = slices.DeleteFunc(s.conversations, func(c domain.Conversation) bool { return c.ID == id })
	s.chatMessages = slices.DeleteFunc(s.chatMessages, func(m domain.ChatMessage) bool { return m.ConversationID == id })
	return nil
}

func (s *Store) AddChatMessage(_ context.Context, m domain.ChatMessage) (domain.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.conversations, func(c domain.Conversation) bool { return c.ID == m.ConversationID })
	if i < 0 {
		return domain.ChatMessage{}, domain.ErrNotFound
	}
	m.ID, m.CreatedAt = uuid.New(), time.Now()
	s.conversations[i].UpdatedAt = m.CreatedAt
	s.chatMessages = append(s.chatMessages, m)
	return m, nil
}

func (s *Store) ChatMessages(_ context.Context, conversationID uuid.UUID, limit int) ([]domain.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.ChatMessage{}
	for _, m := range s.chatMessages {
		if m.ConversationID == conversationID {
			out = append(out, m)
		}
	}
	return out[max(0, len(out)-limit):], nil
}
