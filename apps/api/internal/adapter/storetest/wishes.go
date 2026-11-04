package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Wishes: newest first with their votes, one vote per member that a new one replaces, and no
// reaching into another household.
func Wishes(t *testing.T, s interface {
	Store
	port.WishRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	join(t, s, home.Household.ID, alice, bob)
	hh := home.Household.ID

	price := int64(4900)
	lamp, err := s.CreateWish(ctx, domain.Wish{HouseholdID: hh, CreatedBy: &alice.ID, WishFields: domain.WishFields{Title: "Lamp", Note: "warm", URL: "https://example.com", PriceCents: &price}})
	if err != nil || lamp.ID.String() == "" || lamp.CreatedAt.IsZero() || lamp.PriceCents == nil || *lamp.PriceCents != price {
		t.Fatalf("create: %+v, %v", lamp, err)
	}
	rug, _ := s.CreateWish(ctx, domain.Wish{HouseholdID: hh, WishFields: domain.WishFields{Title: "Rug"}})
	if _, err := s.CreateWish(ctx, domain.Wish{HouseholdID: other.Household.ID, WishFields: domain.WishFields{Title: "Theirs"}}); err != nil {
		t.Fatal(err)
	}
	if ws, err := s.Wishes(ctx, hh, 10); err != nil || len(ws) != 2 || ws[0].ID != rug.ID || ws[1].ID != lamp.ID || ws[0].PriceCents != nil {
		t.Fatalf("list: %+v, %v", ws, err)
	}
	if _, err := s.GetWish(ctx, other.Household.ID, lamp.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other household reads: %v", err)
	}

	if w, err := s.VoteWish(ctx, hh, lamp.ID, alice.ID, 2); err != nil || w.Hearts() != 2 {
		t.Fatalf("vote: %+v, %v", w, err)
	}
	if w, _ := s.VoteWish(ctx, hh, lamp.ID, bob.ID, 3); w.Hearts() != 5 || len(w.Votes) != 2 {
		t.Fatalf("second vote: %+v", w)
	}
	if w, _ := s.VoteWish(ctx, hh, lamp.ID, alice.ID, 1); w.Hearts() != 4 || len(w.Votes) != 2 {
		t.Fatalf("revote: %+v", w)
	}
	if ws, _ := s.Wishes(ctx, hh, 10); len(ws[1].Votes) != 2 || len(ws[0].Votes) != 0 {
		t.Fatalf("votes in list: %+v", ws)
	}
	if w, _ := s.VoteWish(ctx, hh, lamp.ID, alice.ID, 0); w.Hearts() != 3 || len(w.Votes) != 1 {
		t.Fatalf("unvote: %+v", w)
	}
	for _, hearts := range []int{0, 2} {
		if _, err := s.VoteWish(ctx, other.Household.ID, lamp.ID, bob.ID, hearts); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("other household votes (%d): %v", hearts, err)
		}
	}

	lamp.Title, lamp.PriceCents = "Floor lamp", nil
	if w, err := s.UpdateWish(ctx, lamp); err != nil || w.Title != "Floor lamp" || w.PriceCents != nil || w.Hearts() != 3 {
		t.Fatalf("update: %+v, %v", w, err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	if w, err := s.SetWishGot(ctx, hh, lamp.ID, &at); err != nil || w.GotAt == nil || !w.GotAt.Equal(at) {
		t.Fatalf("got: %+v, %v", w, err)
	}
	if w, _ := s.SetWishGot(ctx, hh, lamp.ID, nil); w.GotAt != nil {
		t.Fatalf("a wish again: %+v", w)
	}
	if w, err := s.SetWishPhoto(ctx, hh, lamp.ID, &at); err != nil || w.PhotoAt == nil {
		t.Fatalf("photo: %+v, %v", w, err)
	}
	if _, err := s.SetWishPhoto(ctx, other.Household.ID, lamp.ID, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other household photo: %v", err)
	}

	if err := s.DeleteWish(ctx, other.Household.ID, lamp.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other household deletes: %v", err)
	}
	if err := s.DeleteWish(ctx, hh, lamp.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetWish(ctx, hh, lamp.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
