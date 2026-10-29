package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/redis"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Runs against a real Redis when TEST_REDIS_URL is set (CI provides one).
func TestSessions(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	client, err := redis.Connect(url)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	s := redis.NewSessions(client, time.Minute)
	ctx := context.Background()

	state := uuid.NewString()
	if err := s.SaveLogin(ctx, state, domain.PendingLogin{Nonce: "n", Verifier: "v"}); err != nil {
		t.Fatal(err)
	}
	if l, err := s.TakeLogin(ctx, state); err != nil || l.Nonce != "n" || l.Verifier != "v" {
		t.Fatalf("take: %+v, %v", l, err)
	}
	if _, err := s.TakeLogin(ctx, state); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second take: want ErrNotFound, got %v", err)
	}

	token := uuid.NewString()
	pass := domain.GuestPass{HouseholdID: uuid.New(), CallID: uuid.New(), InvitedBy: "Alice"}
	if err := s.SaveGuestPass(ctx, token, pass); err != nil {
		t.Fatal(err)
	}
	for range 2 { // a guest link works more than once
		if got, err := s.GuestPass(ctx, token); err != nil || got != pass {
			t.Fatalf("guest pass: %+v, %v", got, err)
		}
	}
	if err := s.ClaimGuestPass(ctx, token, "phone"); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimGuestPass(ctx, token, "phone"); err != nil {
		t.Fatalf("same device again: %v", err)
	}
	if err := s.ClaimGuestPass(ctx, token, "laptop"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("another device: want ErrConflict, got %v", err)
	}
	if _, err := s.GuestPass(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown guest pass: want ErrNotFound, got %v", err)
	}

	room := "call-" + uuid.NewString()
	if k, err := s.CallKey(ctx, room, "first"); err != nil || k != "first" {
		t.Fatalf("call key: %q, %v", k, err)
	}
	if k, err := s.CallKey(ctx, room, "second"); err != nil || k != "first" {
		t.Fatalf("call key again: want the first, got %q, %v", k, err)
	}

	id := uuid.NewString()
	want := domain.Session{Identity: domain.Identity{Subject: "sub", Name: "Alice"}, IDToken: "tok"}
	if err := s.Save(ctx, id, want); err != nil {
		t.Fatal(err)
	}
	if keys, _ := client.Keys(ctx, "*"+id+"*").Result(); len(keys) != 0 {
		t.Errorf("session id must not appear in keys: %v", keys)
	}
	if got, err := s.Get(ctx, id); err != nil || got != want {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if err := s.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, id); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after delete: want ErrNotFound, got %v", err)
	}

	agentToken := "hla_" + uuid.NewString()
	grant := domain.AgentGrant{User: domain.User{ID: uuid.New(), Name: "Alice"}, HouseholdID: uuid.New(), ExpiresAt: time.Now().Add(time.Minute).UTC().Round(0)}
	if err := s.SaveAgentGrant(ctx, agentToken, grant); err != nil {
		t.Fatal(err)
	}
	if keys, _ := client.Keys(ctx, "*"+agentToken+"*").Result(); len(keys) != 0 {
		t.Errorf("agent token must not appear in keys: %v", keys)
	}
	if got, err := s.AgentGrant(ctx, agentToken); err != nil || got.User != grant.User || got.HouseholdID != grant.HouseholdID || !got.ExpiresAt.Equal(grant.ExpiresAt) {
		t.Fatalf("agent grant: %+v, %v", got, err)
	}
	if err := s.DeleteAgentGrant(ctx, agentToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AgentGrant(ctx, agentToken); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after delete: want ErrNotFound, got %v", err)
	}
}
