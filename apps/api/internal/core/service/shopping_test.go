package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

// grocer puts milk in dairy and everything else in produce, and counts what it was asked.
type grocer struct {
	asked []string
	down  bool
}

func (g *grocer) Aisles(_ context.Context, texts []string) ([]domain.Aisle, error) {
	if g.down {
		return nil, domain.ErrUnavailable
	}
	g.asked = append(g.asked, texts...)
	out := make([]domain.Aisle, len(texts))
	for i, t := range texts {
		out[i] = domain.AisleProduce
		if strings.Contains(t, "milch") {
			out[i] = domain.AisleDairy
		}
	}
	return out, nil
}

func TestShoppingSortsIntoAislesAndRemembersMoves(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	g := &grocer{}
	shopping := service.NewShopping(store, store, g)

	items, err := shopping.Add(ctx, alice, hh, []string{"Hafermilch", "Lauch", "lauch "}, "")
	if err != nil || items[0].Aisle != domain.AisleDairy || items[1].Aisle != domain.AisleProduce || items[2].Aisle != domain.AisleProduce {
		t.Fatalf("add: %+v, %v", items, err)
	}
	if len(g.asked) != 2 {
		t.Fatalf("asked the grocer %v; want each new thing once", g.asked)
	}

	if _, err := shopping.SetAisle(ctx, eve, hh, items[0].ID, domain.AisleDrinks); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	if _, err := shopping.SetAisle(ctx, alice, hh, items[0].ID, "garden"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown aisle: want ErrInvalid, got %v", err)
	}
	if it, err := shopping.SetAisle(ctx, alice, hh, items[0].ID, domain.AisleDrinks); err != nil || it.Aisle != domain.AisleDrinks {
		t.Fatalf("move: %+v, %v", it, err)
	}

	// What the household knows wins over the grocer, and isn't asked again.
	g.asked = nil
	items, err = shopping.Add(ctx, alice, hh, []string{"HAFERMILCH", "Lauch"}, "")
	if err != nil || items[0].Aisle != domain.AisleDrinks || items[1].Aisle != domain.AisleProduce || len(g.asked) != 0 {
		t.Fatalf("again: %+v, %v, asked %v", items, err, g.asked)
	}

	// Without the grocer new things still go on the list, under "other".
	g.down = true
	if items, err = shopping.Add(ctx, alice, hh, []string{"Seife"}, ""); err != nil || items[0].Aisle != domain.AisleOther {
		t.Fatalf("grocer down: %+v, %v", items, err)
	}
}
