package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres"
	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/storetest"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// store connects to TEST_DATABASE_URL (CI provides one) and migrates it, or skips the test.
func store(t *testing.T) *postgres.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return postgres.NewStore(pool)
}

func TestInvites(t *testing.T)             { storetest.Invites(t, store(t)) }
func TestAvatars(t *testing.T)             { storetest.Avatars(t, store(t)) }
func TestQuestions(t *testing.T)           { storetest.Questions(t, store(t)) }
func TestHomeConnections(t *testing.T)     { storetest.HomeConnections(t, store(t)) }
func TestWatchConnections(t *testing.T)    { storetest.WatchConnections(t, store(t)) }
func TestTonight(t *testing.T)             { storetest.Tonight(t, store(t)) }
func TestPushSubscriptions(t *testing.T)   { storetest.PushSubscriptions(t, store(t)) }
func TestEntries(t *testing.T)             { storetest.Entries(t, store(t)) }
func TestCalendarFeeds(t *testing.T)       { storetest.CalendarFeeds(t, store(t)) }
func TestShoppingItems(t *testing.T)       { storetest.ShoppingItems(t, store(t)) }
func TestShoppingSuggestions(t *testing.T) { storetest.ShoppingSuggestions(t, store(t)) }
func TestReminderSuggestions(t *testing.T) { storetest.ReminderSuggestions(t, store(t)) }
func TestConversations(t *testing.T)       { storetest.Conversations(t, store(t)) }
func TestMCPTokens(t *testing.T)           { storetest.MCPTokens(t, store(t)) }
func TestMoments(t *testing.T)             { storetest.Moments(t, store(t)) }
func TestVoiceSettings(t *testing.T)       { storetest.VoiceSettings(t, store(t)) }

func TestStore(t *testing.T) {
	ctx := context.Background()
	s := store(t)

	sub := "sub-" + uuid.NewString()
	alice, err := s.UpsertByIdentity(ctx, domain.Identity{Subject: sub, Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.UpsertByIdentity(ctx, domain.Identity{Subject: sub, Name: "Alice B", Email: "a@example.com"})
	if err != nil || again.ID != alice.ID || again.Name != "Alice B" || again.Email != "a@example.com" {
		t.Fatalf("upsert should update in place: %+v %v", again, err)
	}

	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil || m.Role != domain.RoleOwner {
		t.Fatalf("create: %+v %v", m, err)
	}
	if role, err := s.Role(ctx, m.Household.ID, alice.ID); err != nil || role != domain.RoleOwner {
		t.Fatalf("role: %v %v", role, err)
	}
	if _, err := s.Role(ctx, m.Household.ID, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stranger role: want ErrNotFound, got %v", err)
	}
	if _, err := s.Get(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing household: want ErrNotFound, got %v", err)
	}
	list, err := s.ListForUser(ctx, alice.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	members, err := s.Members(ctx, m.Household.ID)
	if err != nil || len(members) != 1 || members[0].User.ID != alice.ID {
		t.Fatalf("members: %v %v", members, err)
	}
}
