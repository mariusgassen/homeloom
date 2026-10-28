package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Conversations: each person lists only their own, last used first; messages come back oldest
// first, the last ones when limited; deleting a conversation deletes its messages.
func Conversations(t *testing.T, s interface {
	Store
	port.ConversationRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Household.ID
	join(t, s, h, alice, bob)
	start := func(u domain.User, title string) domain.Conversation {
		t.Helper()
		c, err := s.CreateConversation(ctx, domain.Conversation{HouseholdID: h, UserID: u.ID, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	say := func(c domain.Conversation, role domain.ChatRole, text string) {
		t.Helper()
		if _, err := s.AddChatMessage(ctx, domain.ChatMessage{ConversationID: c.ID, Role: role, Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	first, second := start(alice, "first"), start(alice, "second")
	start(bob, "bob's")
	say(first, domain.RoleUser, "one")
	say(first, domain.RoleAssistant, "two")
	say(first, domain.RoleUser, "three")

	got, err := s.GetConversation(ctx, first.ID)
	if err != nil || got.Title != "first" || got.UserID != alice.ID || got.HouseholdID != h {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if _, err := s.GetConversation(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown conversation: want ErrNotFound, got %v", err)
	}
	list, err := s.ListConversations(ctx, h, alice.ID, 10)
	if err != nil || len(list) != 2 || list[0].ID != first.ID || list[1].ID != second.ID {
		t.Fatalf("list: the one just used comes first, bob's is left out: %+v, %v", list, err)
	}
	ms, err := s.ChatMessages(ctx, first.ID, 2)
	if err != nil || len(ms) != 2 || ms[0].Text != "two" || ms[0].Role != domain.RoleAssistant || ms[1].Text != "three" {
		t.Fatalf("messages: %+v, %v", ms, err)
	}

	if err := s.DeleteConversation(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if ms, err := s.ChatMessages(ctx, first.ID, 10); err != nil || len(ms) != 0 {
		t.Fatalf("messages after delete: %+v, %v", ms, err)
	}
	if list, err := s.ListConversations(ctx, h, alice.ID, 10); err != nil || len(list) != 1 {
		t.Fatalf("list after delete: %+v, %v", list, err)
	}
}
