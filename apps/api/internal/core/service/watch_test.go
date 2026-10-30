package service_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
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
	watch := service.NewWatch(store, store, memory.NewSessions(), media, nil)
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
	watch := service.NewWatch(store, store, memory.NewSessions(), nil, nil)
	if _, err := watch.Shelf(ctx, alice, m.Household.ID); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("shelf: %v", err)
	}
	if err := watch.CompleteConnect(ctx, alice, "x"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("complete: %v", err)
	}
}

// critic answers with fixed picks and remembers what it was shown.
type critic struct {
	picks            []domain.WatchPick
	candidates, seen []domain.Title
}

func (c *critic) Pick(_ context.Context, _, _ string, candidates, seen []domain.Title, _ int) ([]domain.WatchPick, error) {
	c.candidates, c.seen = candidates, seen
	return c.picks, nil
}

func TestWatchIdeas(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hid := m.Household.ID
	media := memory.NewMediaLibrary(
		[]domain.Library{{ID: "abc.1", Name: "Movies", Kind: domain.Movies}},
		map[string][]domain.Title{"abc.1": {{ID: "abc.10", Kind: domain.Movies, Name: "Arrival"}, {ID: "abc.11", Kind: domain.Movies, Name: "Heat", Watched: true}}},
	)
	fake := &critic{picks: []domain.WatchPick{
		{TitleID: "abc.10", Reason: "Quiet and big."},
		{TitleID: "abc.10", Reason: "Twice."},
		{TitleID: "abc.99", Name: "Made up"},
		{Name: " Paddington 2 ", Year: 2017, Kind: "film", Reason: "Pure joy."},
		{Name: "Dark", Kind: domain.Shows},
	}}
	watch := service.NewWatch(store, store, memory.NewSessions(), media, fake)

	// Without Plex connected, ideas come from outside the libraries only.
	ideas, err := watch.Suggest(ctx, alice, hid, "", "en")
	if err != nil || len(fake.candidates) != 0 || len(ideas) != 2 || ideas[0].Name != "Paddington 2" || ideas[0].Kind != domain.Movies || ideas[1].Kind != domain.Shows {
		t.Fatalf("unconnected: %+v, %v", ideas, err)
	}

	signIn(t, watch, alice, m)
	ideas, err = watch.Suggest(ctx, alice, hid, "something calm", "en")
	if err != nil || len(ideas) != 3 || ideas[0].Title == nil || ideas[0].Title.ID != "abc.10" || ideas[0].Reason != "Quiet and big." || ideas[1].Title != nil {
		t.Fatalf("connected: %+v, %v", ideas, err)
	}
	if len(fake.candidates) != 1 || fake.candidates[0].ID != "abc.10" || len(fake.seen) != 1 || fake.seen[0].Name != "Heat" {
		t.Errorf("critic shown %+v and seen %+v", fake.candidates, fake.seen)
	}

	if _, err := watch.Suggest(ctx, bob, hid, "", "en"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("stranger: want ErrNotFound, got %v", err)
	}
	if _, err := watch.Suggest(ctx, alice, hid, strings.Repeat("x", domain.MaxWatchMood+1), "en"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("long mood: want ErrInvalid, got %v", err)
	}
	fake.picks = []domain.WatchPick{{TitleID: "abc.99"}}
	if _, err := watch.Suggest(ctx, alice, hid, "", "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("only made-up picks: want ErrUnavailable, got %v", err)
	}
	if _, err := service.NewWatch(store, store, memory.NewSessions(), media, nil).Suggest(ctx, alice, hid, "", "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("without an LLM: want ErrUnavailable, got %v", err)
	}
}

func TestWatchLeavesOutTheLibrariesYouChoose(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hid := m.Household.ID
	media := memory.NewMediaLibrary(
		[]domain.Library{{ID: "abc.1", Name: "Movies", Kind: domain.Movies}, {ID: "abc.2", Name: "Kids", Kind: domain.Movies}},
		map[string][]domain.Title{"abc.1": {{ID: "abc.10", Name: "Arrival"}}, "abc.2": {{ID: "abc.20", Name: "Paddington"}}},
	)
	fake := &critic{picks: []domain.WatchPick{{Name: "Dark", Kind: domain.Shows}}}
	watch := service.NewWatch(store, store, memory.NewSessions(), media, fake)

	if err := watch.Include(ctx, alice, hid, "abc.2", false); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("before connecting: %v", err)
	}
	signIn(t, watch, alice, m)
	if err := watch.Include(ctx, alice, hid, "abc.2", false); err != nil {
		t.Fatal(err)
	}
	s, err := watch.Shelf(ctx, alice, hid)
	if err != nil || len(s.Libraries) != 2 || !s.Libraries[0].Included || s.Libraries[1].Included {
		t.Fatalf("shelf: %+v, %v", s, err)
	}
	if _, err := watch.Suggest(ctx, alice, hid, "", "en"); err != nil || len(fake.candidates) != 1 || fake.candidates[0].ID != "abc.10" {
		t.Fatalf("ideas used %+v, %v", fake.candidates, err)
	}

	if err := watch.Include(ctx, alice, hid, "zzz.1", false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown library: %v", err)
	}
	if err := watch.Include(ctx, bob, hid, "abc.1", false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("non-member: %v", err)
	}
	if err := watch.Include(ctx, alice, hid, "abc.2", true); err != nil {
		t.Fatal(err)
	}
	if s, _ := watch.Shelf(ctx, alice, hid); !s.Libraries[1].Included {
		t.Errorf("put back: %+v", s)
	}
}
