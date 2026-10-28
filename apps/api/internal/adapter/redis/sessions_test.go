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
	if _, err := s.GuestPass(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown guest pass: want ErrNotFound, got %v", err)
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
}
