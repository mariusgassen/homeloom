package service_test

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func smartHome() *memory.SmartHome {
	return memory.NewSmartHome(
		[]domain.Scene{{ID: "scene.dinner", Name: "Dinner"}, {ID: "scene.movie", Name: "Movie"}},
		[]domain.Light{{ID: "light.kitchen", Name: "Kitchen", On: true}},
	)
}

// connect runs the sign-in at the fake Home Assistant for actor and returns the state it used.
func connect(t *testing.T, home *service.Home, actor domain.User, household domain.Membership) string {
	t.Helper()
	ctx := context.Background()
	to, err := home.Connect(ctx, actor, household.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(to)
	state := u.Query().Get("state")
	if err := home.CompleteConnect(ctx, actor, state, "good"); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestHomeConnectsThroughTheSignIn(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	households := service.NewHousehold(store)
	m, _ := households.Create(ctx, alice, "Home")
	smart := smartHome()
	home := service.NewHome(store, store, memory.NewSessions(), smart)

	h, err := home.Get(ctx, alice, m.Household.ID)
	if err != nil || h.Connected || len(h.Scenes) != 0 {
		t.Fatalf("before: %+v, %v", h, err)
	}
	if err := home.ActivateScene(ctx, alice, m.Household.ID, "scene.dinner"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("switch before connecting: %v", err)
	}

	// A state works once, only for the person who started the sign-in, and a bad code fails.
	to, _ := home.Connect(ctx, alice, m.Household.ID)
	u, _ := url.Parse(to)
	state := u.Query().Get("state")
	if err := home.CompleteConnect(ctx, bob, state, "good"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("someone else's state: %v", err)
	}
	to, _ = home.Connect(ctx, alice, m.Household.ID)
	u, _ = url.Parse(to)
	if err := home.CompleteConnect(ctx, alice, u.Query().Get("state"), "bad"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("bad code: %v", err)
	}
	state = connect(t, home, alice, m)
	if err := home.CompleteConnect(ctx, alice, state, "good"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("state reused: %v", err)
	}

	h, err = home.Get(ctx, alice, m.Household.ID)
	if err != nil || !h.Connected || len(h.Scenes) != 2 || len(h.Lights) != 1 {
		t.Fatalf("after: %+v, %v", h, err)
	}
	if _, err := home.Connect(ctx, bob, m.Household.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member connect: %v", err)
	}

	// Each person connects their own account: Bob, once a member, isn't connected through Alice.
	inv, _ := households.Invite(ctx, alice, m.Household.ID)
	if _, err := households.Join(ctx, bob, inv.Code); err != nil {
		t.Fatal(err)
	}
	if h, err := home.Get(ctx, bob, m.Household.ID); err != nil || h.Connected {
		t.Fatalf("bob: %+v, %v", h, err)
	}
	if err := home.ActivateScene(ctx, bob, m.Household.ID, "scene.dinner"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("bob switching through alice: %v", err)
	}

	// Revoked in Home Assistant: the household is asked to connect again.
	smart.Revoked = append(smart.Revoked, "refresh-good")
	if h, err := home.Get(ctx, alice, m.Household.ID); err != nil || h.Connected {
		t.Fatalf("revoked: %+v, %v", h, err)
	}
	if _, err := store.HomeConnection(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("revoked connection kept: %v", err)
	}
}

func TestHomeOnlySwitchesWhatTheSmartHomeLists(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	smart := smartHome()
	home := service.NewHome(store, store, memory.NewSessions(), smart)
	connect(t, home, alice, m)

	if err := home.ActivateScene(ctx, alice, m.Household.ID, "scene.movie"); err != nil {
		t.Fatal(err)
	}
	if l, err := home.ToggleLight(ctx, alice, m.Household.ID, "light.kitchen"); err != nil || l.On {
		t.Fatalf("toggle: %+v, %v", l, err)
	}
	for name, err := range map[string]error{
		"unknown scene": home.ActivateScene(ctx, alice, m.Household.ID, "scene.party"),
		"lock":          home.ActivateScene(ctx, alice, m.Household.ID, "lock.front_door"),
		"unknown light": func() error { _, err := home.ToggleLight(ctx, alice, m.Household.ID, "light.garage"); return err }(),
		"non-member":    home.ActivateScene(ctx, eve, m.Household.ID, "scene.dinner"),
	} {
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if !slices.Equal(smart.Activated, []string{"scene.movie"}) {
		t.Fatalf("activated %v", smart.Activated)
	}

	if err := home.Disconnect(ctx, alice, m.Household.ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(smart.Revoked, []string{"refresh-good"}) {
		t.Fatalf("revoked %v", smart.Revoked)
	}
	if h, err := home.Get(ctx, alice, m.Household.ID); err != nil || h.Connected {
		t.Fatalf("after disconnect: %+v, %v", h, err)
	}
}

func TestHomeIsUnavailableWithoutHomeAssistant(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	down := smartHome()
	home := service.NewHome(store, store, memory.NewSessions(), down)
	connect(t, home, alice, m)
	down.Down = true

	for name, home := range map[string]*service.Home{"not configured": service.NewHome(store, store, memory.NewSessions(), nil), "down": home} {
		if _, err := home.Get(ctx, alice, m.Household.ID); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestHomeGroupsByRoom(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	smart := smartHome()
	smart.Readings = []domain.Reading{
		{ID: "sensor.kitchen_temperature", Name: "Kitchen", Kind: domain.Temperature, Value: 21.5, Unit: "°C"},
		{ID: "sensor.fridge_temperature", Name: "Fridge", Kind: domain.Temperature, Value: 4},
	}
	smart.Floorplan = domain.Layout{
		Rooms:  []domain.Room{{ID: "bedroom", Name: "Bedroom"}, {ID: "kitchen", Name: "Kitchen"}, {ID: "living", Name: "Living room"}},
		Areas:  map[string]string{"light.kitchen": "kitchen", "sensor.kitchen_temperature": "kitchen", "scene.movie": "living"},
		Hidden: map[string]bool{"sensor.fridge_temperature": true},
	}
	home := service.NewHome(store, store, memory.NewSessions(), smart)
	connect(t, home, alice, m)

	h, err := home.Get(ctx, alice, m.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Empty rooms and hidden entities are left out; the rest keep their room or have none.
	if !slices.Equal(h.Rooms, []domain.Room{{ID: "kitchen", Name: "Kitchen"}, {ID: "living", Name: "Living room"}}) {
		t.Fatalf("rooms %v", h.Rooms)
	}
	if !slices.Equal(h.Scenes, []domain.Scene{{ID: "scene.dinner", Name: "Dinner"}, {ID: "scene.movie", Name: "Movie", Room: "living"}}) ||
		!slices.Equal(h.Lights, []domain.Light{{ID: "light.kitchen", Name: "Kitchen", Room: "kitchen", On: true}}) ||
		!slices.Equal(h.Readings, []domain.Reading{{ID: "sensor.kitchen_temperature", Name: "Kitchen", Room: "kitchen", Kind: domain.Temperature, Value: 21.5, Unit: "°C"}}) {
		t.Fatalf("got %+v", h)
	}

	// Without the rooms, the home still shows, as one list.
	smart.LayoutDown = true
	h, err = home.Get(ctx, alice, m.Household.ID)
	if err != nil || len(h.Rooms) != 0 || len(h.Scenes) != 2 || len(h.Readings) != 2 || h.Lights[0].Room != "" {
		t.Fatalf("without rooms: %+v, %v", h, err)
	}
}
