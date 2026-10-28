package service_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func mediaLibrary() *memory.MediaLibrary {
	return memory.NewMediaLibrary(
		[]domain.Library{{ID: "abc.1", Name: "Movies", Kind: domain.Movies, Server: "Home", Owned: true}},
		map[string][]domain.Title{"abc.1": {{ID: "abc.10", Kind: domain.Movies, Name: "Arrival"}}},
	)
}

// signIn runs the sign-in at the fake Plex for actor and returns the state it used.
func signIn(t *testing.T, watch *service.Watch, actor domain.User, household domain.Membership) string {
	t.Helper()
	ctx := context.Background()
	to, err := watch.Connect(ctx, actor, household.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(to)
	state := u.Query().Get("state")
	if err := watch.CompleteConnect(ctx, actor, state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestWatchConnectsEachPersonThroughTheSignIn(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	media := mediaLibrary()
	watch := service.NewWatch(store, store, memory.NewSessions(), media)
	hid := m.Household.ID

	s, err := watch.Shelf(ctx, alice, hid)
	if err != nil || s.Connected || len(s.Libraries) != 0 {
		t.Fatalf("before: %+v, %v", s, err)
	}
	if _, err := watch.Titles(ctx, alice, hid, "abc.1"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("titles before connecting: %v", err)
	}
	if _, err := watch.Connect(ctx, bob, hid); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member connect: %v", err)
	}

	// A state works once, only for the person who started it, and only once Plex approved it.
	to, _ := watch.Connect(ctx, alice, hid)
	u, _ := url.Parse(to)
	state := u.Query().Get("state")
	if err := watch.CompleteConnect(ctx, bob, state); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("someone else's state: %v", err)
	}
	media.Approved["pin-2"] = false
	to, _ = watch.Connect(ctx, alice, hid)
	u, _ = url.Parse(to)
	if err := watch.CompleteConnect(ctx, alice, u.Query().Get("state")); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("not approved: %v", err)
	}
	state = signIn(t, watch, alice, m)
	if err := watch.CompleteConnect(ctx, alice, state); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("state reused: %v", err)
	}

	s, err = watch.Shelf(ctx, alice, hid)
	if err != nil || !s.Connected || len(s.Libraries) != 1 {
		t.Fatalf("after: %+v, %v", s, err)
	}
	titles, err := watch.Titles(ctx, alice, hid, "abc.1")
	if err != nil || len(titles) != 1 {
		t.Fatalf("titles: %+v, %v", titles, err)
	}
	if p, err := watch.Poster(ctx, alice, hid, "abc.10"); err != nil || len(p.Body) == 0 {
		t.Fatalf("poster: %+v, %v", p, err)
	}
	if _, err := watch.Titles(ctx, alice, hid, "zzz.1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown library: %v", err)
	}
	// Non-members learn nothing, and Bob isn't connected through Alice.
	if _, err := watch.Titles(ctx, bob, hid, "abc.1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member titles: %v", err)
	}

	// Plex being down makes it unavailable; a token Plex signed out is forgotten.
	media.Down = true
	if _, err := watch.Shelf(ctx, alice, hid); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("down: %v", err)
	}
	media.Down = false
	c, _ := store.WatchConnection(ctx, alice.ID)
	media.SignedOut = append(media.SignedOut, c.Token)
	if s, err := watch.Shelf(ctx, alice, hid); err != nil || s.Connected {
		t.Fatalf("signed out: %+v, %v", s, err)
	}
	if _, err := store.WatchConnection(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("kept a signed-out token: %v", err)
	}

	signIn(t, watch, alice, m)
	if err := watch.Disconnect(ctx, alice, hid); err != nil {
		t.Fatal(err)
	}
	if s, _ := watch.Shelf(ctx, alice, hid); s.Connected {
		t.Fatal("still connected after disconnecting")
	}
}

func TestWatchWithoutPlexIsUnavailable(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	watch := service.NewWatch(store, store, memory.NewSessions(), nil)
	if _, err := watch.Shelf(ctx, alice, m.Household.ID); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("shelf: %v", err)
	}
	if err := watch.CompleteConnect(ctx, alice, "x"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("complete: %v", err)
	}
}
