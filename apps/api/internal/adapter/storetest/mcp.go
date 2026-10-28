package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// MCPTokens: each person lists only their own tokens in a household, newest first; a token is
// found by its hash with its owner; only its owner deletes it.
func MCPTokens(t *testing.T, s interface {
	Store
	port.MCPTokenRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Household.ID
	join(t, s, h, alice, bob)
	// Hashes are unique across runs, as the test database is kept.
	run := uuid.NewString()
	create := func(u domain.User, hash, name string) domain.MCPToken {
		t.Helper()
		tok, err := s.CreateMCPToken(ctx, []byte(run+hash), domain.MCPToken{UserID: u.ID, HouseholdID: h, Name: name})
		if err != nil || tok.Name != name || tok.UserID != u.ID || tok.CreatedAt.IsZero() {
			t.Fatalf("create %s: %+v, %v", name, tok, err)
		}
		return tok
	}
	phone := create(alice, "hash-phone", "Phone")
	laptop := create(alice, "hash-laptop", "Laptop")
	create(bob, "hash-bob", "Bob's")

	ts, err := s.ListMCPTokens(ctx, h, alice.ID)
	if err != nil || len(ts) != 2 || ts[0].ID != laptop.ID || ts[1].ID != phone.ID {
		t.Fatalf("alice's tokens: %+v, %v", ts, err)
	}
	tok, owner, err := s.MCPTokenByHash(ctx, []byte(run+"hash-phone"))
	if err != nil || tok.ID != phone.ID || tok.HouseholdID != h || owner.ID != alice.ID || owner.Name != "Alice" {
		t.Fatalf("by hash: %+v %+v, %v", tok, owner, err)
	}
	if _, _, err := s.MCPTokenByHash(ctx, []byte(run+"hash-nope")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown hash: %v", err)
	}
	if err := s.DeleteMCPToken(ctx, phone.ID, bob.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob deleted alice's token: %v", err)
	}
	if err := s.DeleteMCPToken(ctx, phone.ID, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.MCPTokenByHash(ctx, []byte(run+"hash-phone")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleted token: %v", err)
	}
}
