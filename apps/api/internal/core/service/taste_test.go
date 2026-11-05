package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestRatingsAreSharedAndReplaced(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	cook := service.NewCook(f.store, memory.NewRecipeBook(recipes()...), memory.Chef{}, nil).WithTaste(f.store, f.store)

	if _, err := cook.Rate(ctx, f.alice, f.household, "curry", "meh"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("unknown rating: want ErrInvalid, got %v", err)
	}
	if _, err := cook.Rate(ctx, f.alice, f.household, "invented", domain.RatingLove); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown dish: want ErrNotFound, got %v", err)
	}
	if _, err := cook.Rate(ctx, f.eve, f.household, "curry", domain.RatingLove); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	for _, step := range []struct {
		who    domain.User
		rating domain.Rating
	}{{f.alice, domain.RatingNope}, {f.alice, domain.RatingLove}, {f.bob, domain.RatingFine}} {
		if r, err := cook.Rate(ctx, step.who, f.household, "curry", step.rating); err != nil || r.Name != "Green curry" {
			t.Fatalf("rate: %+v, %v", r, err)
		}
	}
	got, err := cook.Ratings(ctx, f.bob, f.household)
	if err != nil || len(got) != 2 {
		t.Fatalf("ratings: %+v, %v", got, err)
	}
	if _, err := cook.Ratings(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	if err := cook.Unrate(ctx, f.bob, f.household, "curry"); err != nil {
		t.Fatal(err)
	}
	if got, _ := cook.Ratings(ctx, f.alice, f.household); len(got) != 1 || got[0].UserID != f.alice.ID || got[0].Rating != domain.RatingLove {
		t.Fatalf("after unrate: %+v", got)
	}
}

func TestNopeDishesLeaveTheIdeas(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	cook := service.NewCook(f.store, memory.NewRecipeBook(recipes()...), memory.Chef{}, nil).WithPlan(f.store).WithTaste(f.store, f.store)
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	if _, err := cook.Rate(ctx, f.bob, f.household, "curry", domain.RatingNope); err != nil {
		t.Fatal(err)
	}
	// Three of the four dishes are left, so they fill three days without the curry.
	ideas, err := cook.SuggestPlan(ctx, f.alice, f.household, monday, 3, "down", "en")
	if err != nil || len(ideas.Meals) != 3 {
		t.Fatalf("ideas: %+v, %v", ideas, err)
	}
	for _, m := range ideas.Meals {
		if m.Recipe.Slug == "curry" {
			t.Fatalf("curry was said no to: %+v", ideas.Meals)
		}
	}
	s, err := cook.Suggest(ctx, f.alice, f.household, "curry", "en")
	if err != nil || len(s.Recipes) != domain.SuggestionCount {
		t.Fatalf("suggest: %+v, %v", s, err)
	}
	for _, r := range s.Recipes {
		if r.Recipe.Slug == "curry" {
			t.Fatalf("curry was said no to: %+v", s.Recipes)
		}
	}
	// Four days need all four dishes, so the dislike gives way rather than leave a day empty.
	if ideas, err := cook.SuggestPlan(ctx, f.alice, f.household, monday, 4, "down", "en"); err != nil || len(ideas.Meals) != 4 {
		t.Fatalf("four days: %+v, %v", ideas, err)
	}
}

func TestRecapSumsUpMealMemories(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	cook := service.NewCook(f.store, memory.NewRecipeBook(recipes()...), memory.Chef{}, nil).WithTaste(f.store, f.store)
	meal := func(who domain.User, vis domain.Visibility, d, slug, name string) {
		f.create(t, who, domain.KindMoment, domain.MomentFields{
			Text: "Good", Visibility: vis, Day: day(d), RecipeSlug: slug, RecipeName: name,
		})
	}
	meal(f.alice, domain.VisibilityShared, "2026-03-01", "curry", "Green curry")
	meal(f.bob, domain.VisibilityShared, "2026-06-01", "curry", "Green curry")
	meal(f.alice, domain.VisibilityShared, "2026-07-01", "pasta", "Pasta")
	meal(f.alice, domain.VisibilityPrivate, "2026-08-01", "salad", "Salad")
	meal(f.alice, domain.VisibilityShared, "2025-12-31", "salad", "Salad")
	f.create(t, f.alice, domain.KindMoment, domain.MomentFields{Text: "Hike", Visibility: domain.VisibilityShared, Day: day("2026-05-01")})
	for _, who := range []domain.User{f.alice, f.bob} {
		if _, err := cook.Rate(ctx, who, f.household, "curry", domain.RatingLove); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cook.Rate(ctx, f.alice, f.household, "pasta", domain.RatingLove); err != nil {
		t.Fatal(err)
	}

	r, err := cook.Recap(ctx, f.bob, f.household, 2026)
	if err != nil || r.Year != 2026 || r.Memories != 3 {
		t.Fatalf("bob's recap: %+v, %v", r, err)
	}
	if len(r.Dishes) != 2 || r.Dishes[0].Slug != "curry" || r.Dishes[0].Times != 2 || !r.Dishes[0].Last.Equal(day("2026-06-01")) {
		t.Fatalf("dishes: %+v", r.Dishes)
	}
	// Only the curry was loved by both; the pasta has one rater.
	if len(r.Favourites) != 1 || r.Favourites[0].Slug != "curry" {
		t.Fatalf("favourites: %+v", r.Favourites)
	}
	// Alice also sees her own private salad.
	if r, err := cook.Recap(ctx, f.alice, f.household, 2026); err != nil || r.Memories != 4 {
		t.Fatalf("alice's recap: %+v, %v", r, err)
	}
	if _, err := cook.Recap(ctx, f.eve, f.household, 2026); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
}

func TestMealMemoryNeedsADishOnAMoment(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	shared := domain.MomentFields{Text: "Yum", Visibility: domain.VisibilityShared, Day: day("2026-10-01")}
	for name, c := range map[string]struct {
		kind       domain.MomentKind
		slug, name string
	}{
		"slug without name": {domain.KindMoment, "curry", ""},
		"name without slug": {domain.KindMoment, "", "Curry"},
		"on a gratitude":    {domain.KindGratitude, "curry", "Curry"},
	} {
		fields := shared
		fields.RecipeSlug, fields.RecipeName = c.slug, c.name
		if _, err := f.moments.Create(ctx, f.alice, f.household, c.kind, fields); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	shared.RecipeSlug, shared.RecipeName = " curry ", " Green curry "
	m := f.create(t, f.alice, domain.KindMoment, shared)
	if m.RecipeSlug != "curry" || m.RecipeName != "Green curry" {
		t.Fatalf("created: %+v", m)
	}
	// Editing without the dish makes it an ordinary moment again.
	shared.RecipeSlug, shared.RecipeName = "", ""
	if m, err := f.moments.Update(ctx, f.alice, m.ID, shared); err != nil || m.RecipeSlug != "" {
		t.Fatalf("updated: %+v, %v", m, err)
	}
}
