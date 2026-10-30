package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// MealPlan: one dinner per household and day, saving replaces it, ranges are half-open and by day.
func MealPlan(t *testing.T, s interface {
	Store
	port.MealPlanRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Other", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hid := home.Household.ID
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	meal := func(hid uuid.UUID, day time.Time, slug string) domain.PlannedMeal {
		return domain.PlannedMeal{HouseholdID: hid, Day: day, PlannedBy: alice.ID,
			Recipe: domain.RecipeSummary{Slug: slug, Name: "Name of " + slug, Image: "v1", TotalTime: "30 min"}}
	}
	for _, m := range []domain.PlannedMeal{
		meal(hid, monday.AddDate(0, 0, 2), "soup"),
		meal(hid, monday, "pasta"),
		meal(hid, monday, "curry"),
		meal(hid, monday.AddDate(0, 0, 7), "salad"),
		meal(other.Household.ID, monday, "pizza"),
	} {
		if err := s.SaveMeal(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Meals(ctx, hid, monday, monday.AddDate(0, 0, 7))
	if err != nil || len(got) != 2 {
		t.Fatalf("week: %+v, %v", got, err)
	}
	if r := got[0].Recipe; r.Slug != "curry" || r.Name != "Name of curry" || r.Image != "v1" || r.TotalTime != "30 min" || !got[0].Day.Equal(monday) || got[0].PlannedBy != alice.ID || got[1].Recipe.Slug != "soup" {
		t.Fatalf("week: %+v", got)
	}
	for range 2 {
		if err := s.DeleteMeal(ctx, hid, monday); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.Meals(ctx, hid, monday, monday.AddDate(0, 0, 1)); err != nil || len(got) != 0 {
		t.Fatalf("after delete: %+v, %v", got, err)
	}
}
