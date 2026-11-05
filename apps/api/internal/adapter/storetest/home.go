package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

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

// Tonight: one pick per household, saving replaces it, deleting a missing one is fine.
func Tonight(t *testing.T, s interface {
	Store
	port.TonightRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hid := m.Household.ID
	if _, err := s.Tonight(ctx, hid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("none yet: %v", err)
	}
	at := time.Date(2026, 10, 30, 19, 30, 0, 0, time.UTC)
	for _, name := range []string{"Heat", "Arrival"} {
		p := domain.TonightPick{HouseholdID: hid, TitleID: "abc.10", Kind: domain.Movies, Name: name, Year: 2016, URL: "https://app.plex.tv/x", PickedBy: alice.ID, PickedAt: at}
		if err := s.SaveTonight(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.Tonight(ctx, hid)
	if err != nil || p.Name != "Arrival" || p.Year != 2016 || p.URL != "https://app.plex.tv/x" || p.PickedBy != alice.ID || !p.PickedAt.Equal(at) || p.Kind != domain.Movies {
		t.Fatalf("got %+v, %v", p, err)
	}
	for range 2 {
		if err := s.DeleteTonight(ctx, hid); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Tonight(ctx, hid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

// Swipes: a member's swipe on a film replaces their last, old ones are left out, and they can be cleared.
func Swipes(t *testing.T, s interface {
	Store
	port.SwipeRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hid := m.Household.ID
	morning := time.Date(2026, 11, 4, 5, 0, 0, 0, time.UTC)
	for _, liked := range []bool{false, true} {
		w := domain.Swipe{HouseholdID: hid, UserID: alice.ID, Key: "arrival.2016", TitleID: "abc.10", Name: "Arrival", Year: 2016, Liked: liked, SwipedAt: morning.Add(time.Hour)}
		if err := s.SaveSwipe(ctx, w); err != nil {
			t.Fatal(err)
		}
	}
	old := domain.Swipe{HouseholdID: hid, UserID: alice.ID, Key: "heat.1995", TitleID: "abc.11", Name: "Heat", Year: 1995, Liked: true, SwipedAt: morning.Add(-time.Hour)}
	if err := s.SaveSwipe(ctx, old); err != nil {
		t.Fatal(err)
	}
	got, err := s.Swipes(ctx, hid, morning)
	if err != nil || len(got) != 1 || got[0].Name != "Arrival" || !got[0].Liked || got[0].Year != 2016 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := s.DeleteSwipes(ctx, hid); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Swipes(ctx, hid, morning.Add(-24*time.Hour)); err != nil || len(got) != 0 {
		t.Fatalf("after delete: %+v, %v", got, err)
	}
}
