package service_test

import (
	"context"
	"errors"
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

func TestHomeOnlySwitchesWhatTheSmartHomeLists(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	smart := smartHome()
	home := service.NewHome(store, smart)

	h, err := home.Get(ctx, alice, m.Household.ID)
	if err != nil || len(h.Scenes) != 2 || len(h.Lights) != 1 {
		t.Fatalf("get: %+v, %v", h, err)
	}
	if err := home.ActivateScene(ctx, alice, m.Household.ID, "scene.dinner"); err != nil {
		t.Fatal(err)
	}
	if err := home.ActivateScene(ctx, alice, m.Household.ID, "scene.movie"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(smart.Activated, []string{"scene.dinner", "scene.movie"}) {
		t.Fatalf("activated %v", smart.Activated)
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
	if len(smart.Activated) != 2 {
		t.Fatalf("activated %v", smart.Activated)
	}
}

func TestHomeIsUnavailableWithoutHomeAssistant(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	down := smartHome()
	down.Down = true

	for name, home := range map[string]*service.Home{"not configured": service.NewHome(store, nil), "down": service.NewHome(store, down)} {
		if _, err := home.Get(ctx, alice, m.Household.ID); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}
