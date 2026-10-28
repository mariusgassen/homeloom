package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestAgentTokens(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	agents := service.NewAgentAccess(f.store, memory.NewSessions(), f.store)
	agents.SetClock(func() time.Time { return f.now })

	if _, err := agents.Mint(ctx, f.eve, f.household, time.Hour); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a non-member got a token: %v", err)
	}
	for _, ttl := range []time.Duration{0, domain.MaxAgentTokenTTL + time.Second} {
		if _, err := agents.Mint(ctx, f.alice, f.household, ttl); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("ttl %s: %v", ttl, err)
		}
	}

	token, err := agents.Mint(ctx, f.alice, f.household, 10*time.Minute)
	if err != nil || !strings.HasPrefix(token, domain.AgentTokenPrefix) {
		t.Fatalf("mint: %q, %v", token, err)
	}
	g, err := agents.Authenticate(ctx, token)
	if err != nil || g.User.ID != f.alice.ID || g.HouseholdID != f.household {
		t.Fatalf("authenticate: %+v, %v", g, err)
	}
	for _, bad := range []string{"", "token-alice", domain.AgentTokenPrefix + "made-up", token + "x"} {
		if _, err := agents.Authenticate(ctx, bad); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("%q: %v", bad, err)
		}
	}

	f.now = f.now.Add(10 * time.Minute)
	if _, err := agents.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expired token: %v", err)
	}

	token, err = agents.Mint(ctx, f.bob, f.household, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked token: %v", err)
	}
}

func TestPersonalTokens(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	agents := service.NewAgentAccess(f.store, memory.NewSessions(), f.store)

	if _, _, err := agents.CreateToken(ctx, f.eve, f.household, "Claude"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a non-member made a token: %v", err)
	}
	var text domain.TextError
	for _, name := range []string{"  ", strings.Repeat("x", domain.MaxMCPTokenName+1)} {
		if _, _, err := agents.CreateToken(ctx, f.alice, f.household, name); !errors.As(err, &text) {
			t.Fatalf("name %q: %v", name, err)
		}
	}

	tok, secret, err := agents.CreateToken(ctx, f.alice, f.household, "  Claude   on my phone ")
	if err != nil || tok.Name != "Claude on my phone" || !strings.HasPrefix(secret, domain.MCPTokenPrefix) {
		t.Fatalf("create: %+v %q, %v", tok, secret, err)
	}
	g, err := agents.Authenticate(ctx, secret)
	if err != nil || g.User.ID != f.alice.ID || g.HouseholdID != f.household {
		t.Fatalf("authenticate: %+v, %v", g, err)
	}
	if _, err := agents.Authenticate(ctx, domain.MCPTokenPrefix+"made-up"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("made-up token: %v", err)
	}

	if ts, err := agents.Tokens(ctx, f.bob, f.household); err != nil || len(ts) != 0 {
		t.Fatalf("bob sees: %+v, %v", ts, err)
	}
	if _, err := agents.Tokens(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("eve lists: %v", err)
	}
	if ts, err := agents.Tokens(ctx, f.alice, f.household); err != nil || len(ts) != 1 || ts[0].ID != tok.ID {
		t.Fatalf("alice sees: %+v, %v", ts, err)
	}
	if err := agents.DeleteToken(ctx, f.bob, tok.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob deleted alice's token: %v", err)
	}
	if err := agents.DeleteToken(ctx, f.alice, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.Authenticate(ctx, secret); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("deleted token: %v", err)
	}
}
