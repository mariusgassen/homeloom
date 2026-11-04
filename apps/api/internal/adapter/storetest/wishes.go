package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Wishes: one wishlist per household, filtered by kind, open wishes before ticked ones, and no
// reaching into another household's list by id.
func Wishes(t *testing.T, s interface {
	Store
	port.WishRepository
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

	add := func(household, kind, text string) domain.Wish {
		t.Helper()
		id := hh
		if household == "other" {
			id = other.Household.ID
		}
		w, err := s.AddWish(ctx, domain.Wish{HouseholdID: id, Kind: domain.WishKind(kind), Text: text, AddedBy: &alice.ID})
		if err != nil || w.ID.String() == "" || w.DoneAt != nil || w.Kind != domain.WishKind(kind) {
			t.Fatalf("add %q: %+v, %v", text, w, err)
		}
		return w
	}
	dune := add("home", "read", "Dune")
	alien := add("home", "watch", "Alien")
	lisbon := add("home", "visit", "Lisbon")
	add("other", "watch", "Tea ceremony")

	texts := func(kind domain.WishKind) []string {
		t.Helper()
		ws, err := s.Wishes(ctx, hh, kind, 100)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(ws))
		for i, w := range ws {
			out[i] = w.Text
		}
		return out
	}
	if got := texts(""); len(got) != 3 || got[0] != "Dune" || got[1] != "Alien" || got[2] != "Lisbon" {
		t.Fatalf("all: %v", got)
	}
	if got := texts(domain.WishWatch); len(got) != 1 || got[0] != "Alien" {
		t.Fatalf("watch: %v", got)
	}

	if _, err := s.SetWishDone(ctx, other.Household.ID, dune.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tick in another household: want ErrNotFound, got %v", err)
	}
	if w, err := s.SetWishDone(ctx, hh, dune.ID, true); err != nil || w.DoneAt == nil {
		t.Fatalf("tick: %+v, %v", w, err)
	}
	if got := texts(""); got[0] != "Alien" || got[2] != "Dune" {
		t.Fatalf("ticked goes last: %v", got)
	}
	if w, err := s.SetWishDone(ctx, hh, dune.ID, false); err != nil || w.DoneAt != nil {
		t.Fatalf("untick: %+v, %v", w, err)
	}

	if err := s.DeleteWish(ctx, other.Household.ID, alien.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete in another household: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteWish(ctx, hh, alien.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWish(ctx, hh, alien.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete twice: want ErrNotFound, got %v", err)
	}
	if got := texts(""); len(got) != 2 || got[1] != lisbon.Text {
		t.Fatalf("after delete: %v", got)
	}
	if ws, err := s.Wishes(ctx, other.Household.ID, "", 100); err != nil || len(ws) != 1 {
		t.Fatalf("other household touched: %+v, %v", ws, err)
	}
}
