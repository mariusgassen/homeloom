package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Cook suggests recipes from Mealie. Without Mealie (book is nil) cooking is unavailable; without
// an LLM (chef is nil or down) suggestions are random, so the feature degrades instead of breaking.
type Cook struct {
	households port.HouseholdRepository
	book       port.RecipeBook
	chef       port.Chef
}

func NewCook(households port.HouseholdRepository, book port.RecipeBook, chef port.Chef) *Cook {
	return &Cook{households: households, book: book, chef: chef}
}

func (s *Cook) Suggest(ctx context.Context, actor domain.User, householdID uuid.UUID, mood, lang string) (domain.Suggestions, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Suggestions{}, err
	}
	mood = strings.TrimSpace(mood)
	if mood == "" || utf8.RuneCountInString(mood) > domain.MaxMood {
		return domain.Suggestions{}, fmt.Errorf("%w: mood must be 1-%d characters", domain.ErrInvalid, domain.MaxMood)
	}
	all, err := s.book.Recipes(ctx)
	if err != nil {
		return domain.Suggestions{}, err
	}
	if len(all) == 0 {
		return domain.Suggestions{Recipes: []domain.Suggestion{}}, nil
	}

	if s.chef != nil {
		picks, err := s.chef.Pick(ctx, mood, lang, all, domain.SuggestionCount)
		if err != nil {
			slog.WarnContext(ctx, "chef unavailable, picking at random", "err", err)
		}
		if out := chosen(all, picks); len(out) > 0 {
			return domain.Suggestions{Recipes: out, Chef: true}, nil
		}
	}
	return domain.Suggestions{Recipes: random(all, domain.SuggestionCount)}, nil
}

func (s *Cook) Recipe(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.Recipe, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.Recipe{}, err
	}
	return s.book.Recipe(ctx, slug)
}

func (s *Cook) AddToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string, ingredientIDs []string) (domain.ShoppingListAdded, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.ShoppingListAdded{}, err
	}
	if len(ingredientIDs) == 0 {
		return domain.ShoppingListAdded{}, fmt.Errorf("%w: no ingredients", domain.ErrInvalid)
	}
	return s.book.AddToShoppingList(ctx, slug, ingredientIDs)
}

// ready checks membership first, so non-members learn nothing about whether Mealie is set up.
func (s *Cook) ready(ctx context.Context, actor domain.User, householdID uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	if s.book == nil {
		return fmt.Errorf("%w: Mealie is not configured", domain.ErrUnavailable)
	}
	return nil
}

// chosen keeps the picks that name a real candidate, once each: an LLM can invent slugs.
func chosen(all []domain.RecipeSummary, picks []domain.Pick) []domain.Suggestion {
	bySlug := make(map[string]domain.RecipeSummary, len(all))
	for _, r := range all {
		bySlug[r.Slug] = r
	}
	out := []domain.Suggestion{}
	for _, p := range picks {
		r, ok := bySlug[p.Slug]
		if !ok || len(out) == domain.SuggestionCount {
			continue
		}
		delete(bySlug, p.Slug)
		out = append(out, domain.Suggestion{Recipe: r, Reason: p.Reason})
	}
	return out
}

func random(all []domain.RecipeSummary, n int) []domain.Suggestion {
	out := []domain.Suggestion{}
	for _, i := range rand.Perm(len(all)) { //nolint:gosec // picking dinner, not a secret
		if len(out) == n {
			break
		}
		out = append(out, domain.Suggestion{Recipe: all[i]})
	}
	return out
}
