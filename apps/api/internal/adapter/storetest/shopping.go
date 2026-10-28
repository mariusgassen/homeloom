package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// ShoppingItems: one list per household, open items before ticked ones, and no reaching into
// another household's list by id.
func ShoppingItems(t *testing.T, s interface {
	Store
	port.ShoppingRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hh := home.Household.ID

	added, err := s.AddShoppingItems(ctx, []domain.ShoppingItem{
		{HouseholdID: hh, Text: "Milk", AddedBy: &alice.ID},
		{HouseholdID: hh, Text: "Leek", Recipe: "Soup", AddedBy: &alice.ID},
		{HouseholdID: hh, Text: "Bread"},
	})
	if err != nil || len(added) != 3 || added[1].Recipe != "Soup" || added[0].ID == uuid.Nil || added[0].DoneAt != nil {
		t.Fatalf("add: %+v, %v", added, err)
	}
	if _, err := s.AddShoppingItems(ctx, []domain.ShoppingItem{{HouseholdID: other.Household.ID, Text: "Tea"}}); err != nil {
		t.Fatal(err)
	}
	milk, leek, bread := added[0].ID, added[1].ID, added[2].ID

	if _, err := s.SetShoppingItemDone(ctx, other.Household.ID, milk, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tick in another household: want ErrNotFound, got %v", err)
	}
	if it, err := s.SetShoppingItemDone(ctx, hh, milk, true); err != nil || it.DoneAt == nil {
		t.Fatalf("tick: %+v, %v", it, err)
	}
	texts := func() []string {
		t.Helper()
		items, err := s.ShoppingItems(ctx, hh, 100)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.Text
		}
		return out
	}
	if got := texts(); len(got) != 3 || got[0] != "Leek" || got[1] != "Bread" || got[2] != "Milk" {
		t.Fatalf("order: %v", got)
	}
	if it, err := s.SetShoppingItemDone(ctx, hh, milk, false); err != nil || it.DoneAt != nil {
		t.Fatalf("untick: %+v, %v", it, err)
	}

	if err := s.DeleteShoppingItem(ctx, other.Household.ID, leek); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete in another household: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteShoppingItem(ctx, hh, leek); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetShoppingItemDone(ctx, hh, bread, true); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearDoneShoppingItems(ctx, hh); err != nil {
		t.Fatal(err)
	}
	if got := texts(); len(got) != 1 || got[0] != "Milk" {
		t.Fatalf("after clearing: %v", got)
	}
	if items, err := s.ShoppingItems(ctx, other.Household.ID, 100); err != nil || len(items) != 1 {
		t.Fatalf("other household touched: %+v, %v", items, err)
	}
}
