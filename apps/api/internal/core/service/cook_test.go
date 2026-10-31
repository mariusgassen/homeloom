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

func recipes() []domain.Recipe {
	mk := func(slug, name string) domain.Recipe {
		return domain.Recipe{
			RecipeSummary: domain.RecipeSummary{Slug: slug, Name: name},
			Ingredients:   []domain.Ingredient{{ID: slug + "-1", Text: "1 onion"}, {ID: slug + "-2", Text: "salt"}},
		}
	}
	return []domain.Recipe{mk("mushroom-soup", "Mushroom soup"), mk("curry", "Green curry"), mk("pasta", "Pasta"), mk("salad", "Salad")}
}

func TestSuggestUsesTheChefAndDropsInventedRecipes(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	cook := service.NewCook(store, memory.NewRecipeBook(recipes()...), memory.Chef{}, nil)

	s, err := cook.Suggest(ctx, alice, m.Household.ID, "  something with mushroom ", "de")
	if err != nil || !s.Chef || len(s.Recipes) != 1 || s.Recipes[0].Recipe.Slug != "mushroom-soup" || s.Recipes[0].Reason != "de: fits mushroom" {
		t.Fatalf("suggest: %+v, %v", s, err)
	}
}

func TestSuggestFallsBackToRandomWithoutTheChef(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")

	for name, cook := range map[string]*service.Cook{
		"chef down":    service.NewCook(store, memory.NewRecipeBook(recipes()...), memory.Chef{}, nil),
		"no chef":      service.NewCook(store, memory.NewRecipeBook(recipes()...), nil, nil),
		"nothing fits": service.NewCook(store, memory.NewRecipeBook(recipes()...), memory.Chef{}, nil),
	} {
		mood := map[string]string{"chef down": "down", "no chef": "cozy", "nothing fits": "sushi"}[name]
		s, err := cook.Suggest(ctx, alice, m.Household.ID, mood, "en")
		if err != nil || s.Chef || len(s.Recipes) != domain.SuggestionCount {
			t.Fatalf("%s: %+v, %v", name, s, err)
		}
		seen := map[string]bool{}
		for _, r := range s.Recipes {
			if seen[r.Recipe.Slug] || r.Reason != "" {
				t.Fatalf("%s: duplicate or reasoned random pick %+v", name, s)
			}
			seen[r.Recipe.Slug] = true
		}
	}
}

func TestCookingNeedsMembershipAndMealie(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	book := memory.NewRecipeBook(recipes()...)
	shopping := service.NewShopping(store, store, nil)
	cook := service.NewCook(store, book, memory.Chef{}, shopping)

	if _, err := cook.Recipe(ctx, eve, m.Household.ID, "curry"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	if _, err := service.NewCook(store, nil, nil, nil).Recipe(ctx, eve, m.Household.ID, "curry"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member without Mealie: want ErrNotFound, got %v", err)
	}
	if _, err := service.NewCook(store, nil, nil, nil).Suggest(ctx, alice, m.Household.ID, "cozy", "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("no Mealie: want ErrUnavailable, got %v", err)
	}
	if _, err := cook.Suggest(ctx, alice, m.Household.ID, " ", "en"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty mood: want ErrInvalid, got %v", err)
	}
	book.Down = true
	if _, err := cook.Suggest(ctx, alice, m.Household.ID, "cozy", "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Mealie down: want ErrUnavailable, got %v", err)
	}
	book.Down = false

	added, err := cook.AddToShoppingList(ctx, alice, m.Household.ID, "curry", []string{"curry-1", "pasta-1"})
	items, _ := shopping.Items(ctx, alice, m.Household.ID)
	if err != nil || added != 1 || len(items) != 1 || items[0].Text != "1 onion" || items[0].Recipe != "Green curry" {
		t.Fatalf("add: %d, %v, %+v", added, err, items)
	}
	if _, err := cook.AddToShoppingList(ctx, alice, m.Household.ID, "curry", nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("no ingredients: want ErrInvalid, got %v", err)
	}
	if _, err := cook.AddToShoppingList(ctx, eve, m.Household.ID, "curry", []string{"curry-1"}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
}

func TestMealPlan(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hid := m.Household.ID
	book := memory.NewRecipeBook(recipes()...)
	shopping := service.NewShopping(store, store, nil)
	cook := service.NewCook(store, book, memory.Chef{}, shopping).WithPlan(store)
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	// The day counts, not the time of day or its zone.
	berlin, _ := time.LoadLocation("Europe/Berlin")
	planned, err := cook.PlanMeal(ctx, alice, hid, time.Date(2026, 11, 3, 23, 30, 0, 0, berlin), "curry")
	if err != nil || !planned.Day.Equal(monday.AddDate(0, 0, 1)) || planned.Recipe.Name != "Green curry" || planned.PlannedBy != alice.ID {
		t.Fatalf("plan: %+v, %v", planned, err)
	}
	if _, err := cook.PlanMeal(ctx, alice, hid, monday, "made-up"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown recipe: %v", err)
	}
	if _, err := cook.PlanMeal(ctx, eve, hid, monday, "curry"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stranger plans: %v", err)
	}
	if _, err := cook.Plan(ctx, eve, hid, monday, 7); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stranger reads: %v", err)
	}
	if _, err := cook.Plan(ctx, alice, hid, monday, 15); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("too many days: %v", err)
	}

	// While Mealie is down the plan can still be read and changed, but not filled.
	book.Down = true
	if week, err := cook.Plan(ctx, alice, hid, monday, 7); err != nil || len(week) != 1 || week[0].Recipe.Slug != "curry" {
		t.Fatalf("week without Mealie: %+v, %v", week, err)
	}
	if _, err := cook.SuggestPlan(ctx, alice, hid, monday, 7, "", "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("ideas without Mealie: %v", err)
	}
	book.Down = false

	if err := cook.UnplanMeal(ctx, eve, hid, monday.AddDate(0, 0, 1)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stranger unplans: %v", err)
	}
	for range 2 {
		if err := cook.UnplanMeal(ctx, alice, hid, monday.AddDate(0, 0, 1)); err != nil {
			t.Fatal(err)
		}
	}
	if week, err := cook.Plan(ctx, alice, hid, monday, 7); err != nil || len(week) != 0 {
		t.Fatalf("after unplanning: %+v, %v", week, err)
	}
}

func TestSuggestPlanFillsOpenDaysWithFreshRecipes(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hid := m.Household.ID
	shopping := service.NewShopping(store, store, nil)
	cook := service.NewCook(store, memory.NewRecipeBook(recipes()...), memory.Chef{}, shopping).WithPlan(store)
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	// Pasta was last week, the salad is planned for Tuesday, and mushrooms were just bought.
	for day, slug := range map[time.Time]string{monday.AddDate(0, 0, -5): "pasta", monday.AddDate(0, 0, 1): "salad"} {
		if _, err := cook.PlanMeal(ctx, alice, hid, day, slug); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := shopping.Add(ctx, alice, hid, []string{"Mushroom"}, "")
	if _, err := shopping.SetDone(ctx, alice, hid, items[0].ID, true); err != nil {
		t.Fatal(err)
	}

	ideas, err := cook.SuggestPlan(ctx, alice, hid, monday, 3, " ", "de")
	if err != nil || !ideas.Chef || len(ideas.Meals) != 2 {
		t.Fatalf("ideas: %+v, %v", ideas, err)
	}
	first, second := ideas.Meals[0], ideas.Meals[1]
	if !first.Day.Equal(monday) || first.Recipe.Slug != "mushroom-soup" || first.Reason != "de: uses mushroom" {
		t.Fatalf("monday: %+v", first)
	}
	// The chef had nothing more; Wednesday gets the one recipe neither cooked lately nor picked.
	if !second.Day.Equal(monday.AddDate(0, 0, 2)) || second.Recipe.Slug != "curry" || second.Reason != "" {
		t.Fatalf("wednesday: %+v", second)
	}

	ideas, err = cook.SuggestPlan(ctx, alice, hid, monday, 3, "down", "en")
	if err != nil || ideas.Chef || len(ideas.Meals) != 2 {
		t.Fatalf("chef down: %+v, %v", ideas, err)
	}
	if ideas, err := cook.SuggestPlan(ctx, alice, hid, monday.AddDate(0, 0, 1), 1, "", "en"); err != nil || len(ideas.Meals) != 0 {
		t.Fatalf("all planned: %+v, %v", ideas, err)
	}
}

func TestAddMealsToShoppingList(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hid := m.Household.ID
	shopping := service.NewShopping(store, store, nil)
	cook := service.NewCook(store, memory.NewRecipeBook(recipes()...), nil, shopping)

	n, err := cook.AddMealsToShoppingList(ctx, alice, hid, []domain.RecipeIngredients{
		{Slug: "curry", IngredientIDs: []string{"curry-1", "curry-2"}},
		{Slug: "pasta", IngredientIDs: []string{"pasta-1", "unknown"}},
	})
	if err != nil || n != 3 {
		t.Fatalf("add: %d, %v", n, err)
	}
	items, _ := shopping.Items(ctx, alice, hid)
	got := map[string]int{}
	for _, it := range items {
		got[it.Recipe]++
	}
	if got["Green curry"] != 2 || got["Pasta"] != 1 {
		t.Fatalf("items by recipe: %v", got)
	}
	if _, err := cook.AddMealsToShoppingList(ctx, alice, hid, []domain.RecipeIngredients{{Slug: "curry", IngredientIDs: []string{"curry-1"}}, {Slug: "salad", IngredientIDs: []string{"x"}}}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("a recipe with none of its ingredients: %v", err)
	}
	if items, _ := shopping.Items(ctx, alice, hid); len(items) != 3 {
		t.Fatalf("all or nothing: %d items", len(items))
	}
}
