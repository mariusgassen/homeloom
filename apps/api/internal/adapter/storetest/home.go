package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// HomeConnections: one per person, saving replaces it, deleting a missing one is fine.
func HomeConnections(t *testing.T, s interface {
	Store
	port.HomeConnectionRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")

	if _, err := s.HomeConnection(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("none yet: %v", err)
	}
	for _, token := range []string{"first", "second"} {
		if err := s.SaveHomeConnection(ctx, domain.HomeConnection{UserID: alice.ID, RefreshToken: token}); err != nil {
			t.Fatal(err)
		}
	}
	if c, err := s.HomeConnection(ctx, alice.ID); err != nil || c.RefreshToken != "second" || c.UserID != alice.ID {
		t.Fatalf("got %+v, %v", c, err)
	}
	for range 2 {
		if err := s.DeleteHomeConnection(ctx, alice.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.HomeConnection(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
