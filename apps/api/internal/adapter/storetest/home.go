package storetest

import (
	"context"
	"errors"
	"slices"
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

// WatchConnections: one per person, saving replaces it, deleting a missing one is fine.
func WatchConnections(t *testing.T, s interface {
	Store
	port.WatchConnectionRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")

	if _, err := s.WatchConnection(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("none yet: %v", err)
	}
	for _, token := range []string{"first", "second"} {
		if err := s.SaveWatchConnection(ctx, domain.WatchConnection{UserID: alice.ID, Token: token}); err != nil {
			t.Fatal(err)
		}
	}
	if c, err := s.WatchConnection(ctx, alice.ID); err != nil || c.Token != "second" || c.UserID != alice.ID || len(c.Hidden) != 0 {
		t.Fatalf("got %+v, %v", c, err)
	}

	// Leaving a library out twice lists it once; signing in again keeps the list.
	for _, step := range []struct {
		id     string
		hidden bool
	}{{"a.1", true}, {"a.2", true}, {"a.1", true}, {"a.2", false}} {
		if err := s.SetWatchLibraryHidden(ctx, alice.ID, step.id, step.hidden); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveWatchConnection(ctx, domain.WatchConnection{UserID: alice.ID, Token: "third"}); err != nil {
		t.Fatal(err)
	}
	if c, err := s.WatchConnection(ctx, alice.ID); err != nil || !slices.Equal(c.Hidden, []string{"a.1"}) {
		t.Fatalf("hidden: %+v, %v", c, err)
	}
	bob := user(t, s, "Bob")
	if err := s.SetWatchLibraryHidden(ctx, bob.ID, "a.1", true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("hide without a connection: %v", err)
	}

	for range 2 {
		if err := s.DeleteWatchConnection(ctx, alice.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.WatchConnection(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
