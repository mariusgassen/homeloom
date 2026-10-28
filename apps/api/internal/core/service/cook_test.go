package service_test

import (
	"context"
	"errors"
	"testing"

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
	shopping := service.NewShopping(store, store)
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
