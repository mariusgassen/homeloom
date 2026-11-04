package storetest

import (
	"context"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Taste: one rating per member and dish, rating again replaces it, and a household sees only its own.
func Taste(t *testing.T, s interface {
	Store
	port.TasteRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Other", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hid := home.Household.ID
	for _, r := range []domain.MealRating{
		{HouseholdID: hid, UserID: alice.ID, Slug: "soup", Name: "Soup", Rating: domain.RatingNope},
		{HouseholdID: hid, UserID: alice.ID, Slug: "soup", Name: "Mushroom soup", Rating: domain.RatingLove},
		{HouseholdID: hid, UserID: bob.ID, Slug: "soup", Name: "Mushroom soup", Rating: domain.RatingFine},
		{HouseholdID: hid, UserID: alice.ID, Slug: "curry", Name: "Curry", Rating: domain.RatingNope},
		{HouseholdID: other.Household.ID, UserID: alice.ID, Slug: "pizza", Name: "Pizza", Rating: domain.RatingLove},
	} {
		if err := s.SaveRating(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Ratings(ctx, hid)
	if err != nil || len(got) != 3 {
		t.Fatalf("ratings: %+v, %v", got, err)
	}
	by := map[string]domain.MealRating{}
	for _, r := range got {
		by[r.Slug+"/"+r.UserID.String()] = r
	}
	if r := by["soup/"+alice.ID.String()]; r.Rating != domain.RatingLove || r.Name != "Mushroom soup" || r.RatedAt.IsZero() {
		t.Fatalf("alice's soup: %+v", r)
	}
	if by["soup/"+bob.ID.String()].Rating != domain.RatingFine {
		t.Fatalf("bob's soup: %+v", got)
	}
	for range 2 {
		if err := s.DeleteRating(ctx, hid, alice.ID, "curry"); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.Ratings(ctx, hid); err != nil || len(got) != 2 {
		t.Fatalf("after delete: %+v, %v", got, err)
	}
}
