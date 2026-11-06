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

func newUser(t *testing.T, store *memory.Store, sub string) domain.User {
	t.Helper()
	u, err := store.UpsertByIdentity(context.Background(), domain.Identity{Subject: sub, Name: sub})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCreateMakesCallerOwner(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := service.NewHousehold(store)
	alice := newUser(t, store, "alice")

	m, err := svc.Create(ctx, alice, "  Home  ")
	if err != nil {
		t.Fatal(err)
	}
	if m.Role != domain.RoleOwner || m.Household.Name != "Home" {
		t.Fatalf("got %+v", m)
	}
	list, _ := svc.List(ctx, alice)
	if len(list) != 1 {
		t.Fatalf("want 1 household, got %d", len(list))
	}
}

func TestCreateRejectsBadNames(t *testing.T) {
	store := memory.NewStore()
	svc := service.NewHousehold(store)
	alice := newUser(t, store, "alice")

	for _, name := range []string{"", "   ", strings.Repeat("x", 101)} {
		if _, err := svc.Create(context.Background(), alice, name); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("name %q: want ErrInvalid, got %v", name, err)
		}
	}
}

func TestGetIsMembersOnly(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	svc := service.NewHousehold(store)
	alice, bob, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "eve")

	m, _ := svc.Create(ctx, alice, "Home")
	store.AddMember(m.Household.ID, bob.ID, domain.RoleMember)

	d, err := svc.Get(ctx, bob, m.Household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Role != domain.RoleMember || len(d.Members) != 2 {
		t.Fatalf("got %+v", d)
	}

	if _, err := svc.Get(ctx, eve, m.Household.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	if list, _ := svc.List(ctx, eve); len(list) != 0 {
		t.Fatalf("non-member sees %d households", len(list))
	}
}

type placeWeather struct{ known string }

func (placeWeather) ForecastAt(context.Context, domain.Coords, time.Time) (domain.Forecast, error) {
	return domain.Forecast{}, nil
}

func (w placeWeather) Forecast(_ context.Context, place string, _ time.Time) (domain.Forecast, error) {
	if place != w.known {
		return domain.Forecast{}, domain.ErrNotFound
	}
	return domain.Forecast{}, nil
}

func TestHouseholdSetPlace(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	svc := service.NewHousehold(store).WithWeather(placeWeather{known: "Berlin"})
	m, err := svc.Create(ctx, alice, "Home")
	if err != nil {
		t.Fatal(err)
	}
	id := m.Household.ID
	d, err := svc.SetPlace(ctx, alice, id, "  Berlin ")
	if err != nil || d.Household.Place != "Berlin" {
		t.Fatalf("set: %+v, %v", d, err)
	}
	if _, err := svc.SetPlace(ctx, alice, id, "Nowhereville"); !errors.Is(err, domain.ErrUnknownPlace) {
		t.Fatalf("unknown place: %v", err)
	}
	if _, err := svc.SetPlace(ctx, eve, id, "Berlin"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: %v", err)
	}
	if d, err = svc.SetPlace(ctx, alice, id, ""); err != nil || d.Household.Place != "" {
		t.Fatalf("clear: %+v, %v", d, err)
	}
}
