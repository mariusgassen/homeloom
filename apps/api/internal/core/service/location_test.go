package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestLocation(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	svc := service.NewLocation(store).WithNamer(fakeNamer{})

	if err := svc.Set(ctx, alice, domain.Coords{Latitude: 91}, "en"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bad position: %v", err)
	}
	if err := svc.Set(ctx, alice, domain.Coords{Latitude: 53.551234, Longitude: 9.993456}, "en"); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Locations(ctx, []uuid.UUID{alice.ID})
	if l := got[alice.ID]; l.Latitude != 53.55 || l.Longitude != 9.99 || l.Place != "Hamburg" {
		t.Fatalf("kept %+v", l)
	}
	if err := svc.Clear(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Locations(ctx, []uuid.UUID{alice.ID}); len(got) != 0 {
		t.Fatalf("still kept %+v", got)
	}
}
