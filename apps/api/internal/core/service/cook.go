package service

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Cook suggests recipes from Mealie and puts missing ingredients on the household's shopping list.
// Without Mealie (book is nil) cooking is unavailable; without an LLM (chef is nil or down)
// suggestions are random, so the feature degrades instead of breaking.
type Cook struct {
	households port.HouseholdRepository
	book       port.RecipeBook
	chef       port.Chef
	shopping   port.ShoppingService
	meals      port.MealPlanRepository // nil: no meal plan
}

func NewCook(households port.HouseholdRepository, book port.RecipeBook, chef port.Chef, shopping port.ShoppingService) *Cook {
	return &Cook{households: households, book: book, chef: chef, shopping: shopping}
}

// WithPlan keeps the household's meal plan in meals.
func (s *Cook) WithPlan(meals port.MealPlanRepository) *Cook {
	s.meals = meals
	return s
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
		if out := chosen(all, picks, domain.SuggestionCount); len(out) > 0 {
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

func (s *Cook) Image(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string) (domain.RecipeImage, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return domain.RecipeImage{}, err
	}
	return s.book.Image(ctx, slug)
}

func (s *Cook) AddToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, slug string, ingredientIDs []string) (int, error) {
	return s.AddMealsToShoppingList(ctx, actor, householdID, []domain.RecipeIngredients{{Slug: slug, IngredientIDs: ingredientIDs}})
}

func (s *Cook) AddMealsToShoppingList(ctx context.Context, actor domain.User, householdID uuid.UUID, recipes []domain.RecipeIngredients) (int, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return 0, err
	}
	if len(recipes) == 0 || len(recipes) > domain.MaxPlanDays {
		return 0, fmt.Errorf("%w: add from 1-%d recipes", domain.ErrInvalid, domain.MaxPlanDays)
	}
	var items []domain.ShoppingItem
	for _, ri := range recipes {
		r, err := s.book.Recipe(ctx, ri.Slug)
		if err != nil {
			return 0, err
		}
		want := make(map[string]bool, len(ri.IngredientIDs))
		for _, id := range ri.IngredientIDs {
			want[id] = true
		}
		n := len(items)
		for _, in := range r.Ingredients {
			if want[in.ID] {
				items = append(items, domain.ShoppingItem{Text: in.Text, Recipe: r.Name})
			}
		}
		if len(items) == n {
			return 0, fmt.Errorf("%w: none of the ingredients belong to %s", domain.ErrInvalid, ri.Slug)
		}
	}
	added, err := s.shopping.AddItems(ctx, actor, householdID, items)
	return len(added), err
}

func (s *Cook) Find(ctx context.Context, actor domain.User, householdID uuid.UUID, query string, n int) ([]domain.RecipeSummary, error) {
	if err := s.ready(ctx, actor, householdID); err != nil {
		return nil, err
	}
	all, err := s.book.Recipes(ctx)
	if err != nil {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		out := []domain.RecipeSummary{}
		for _, r := range random(all, n) {
			out = append(out, r.Recipe)
		}
		return out, nil
	}
	type scored struct {
		r     domain.RecipeSummary
		score int
	}
	var hits []scored
	for _, r := range all {
		name := strings.ToLower(r.Name)
		rest := strings.ToLower(strings.Join(r.Tags, " ") + " " + r.Description)
		score := 0
		for _, w := range words {
			// A word in the name counts more than one in the tags or description.
			if strings.Contains(name, w) {
				score += 2
			} else if strings.Contains(rest, w) {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, scored{r, score})
		}
	}
	slices.SortStableFunc(hits, func(a, b scored) int { return b.score - a.score })
	out := []domain.RecipeSummary{}
	for _, h := range hits[:min(n, len(hits))] {
		out = append(out, h.r)
	}
	return out, nil
}

func (s *Cook) Plan(ctx context.Context, actor domain.User, householdID uuid.UUID, from time.Time, days int) ([]domain.PlannedMeal, error) {
	if err := s.planReady(ctx, actor, householdID, false); err != nil {
		return nil, err
	}
	if days < 1 || days > domain.MaxPlanDays {
		return nil, fmt.Errorf("%w: days must be 1-%d", domain.ErrInvalid, domain.MaxPlanDays)
	}
	from = date(from)
	return s.meals.Meals(ctx, householdID, from, from.AddDate(0, 0, days))
}

func (s *Cook) PlanMeal(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time, slug string) (domain.PlannedMeal, error) {
	if err := s.planReady(ctx, actor, householdID, true); err != nil {
		return domain.PlannedMeal{}, err
	}
	r, err := s.book.Recipe(ctx, slug)
	if err != nil {
		return domain.PlannedMeal{}, err
	}
	m := domain.PlannedMeal{HouseholdID: householdID, Day: date(day), Recipe: r.RecipeSummary, PlannedBy: actor.ID}
	if err := s.meals.SaveMeal(ctx, m); err != nil {
		return domain.PlannedMeal{}, err
	}
	return m, nil
}

func (s *Cook) UnplanMeal(ctx context.Context, actor domain.User, householdID uuid.UUID, day time.Time) error {
	if err := s.planReady(ctx, actor, householdID, false); err != nil {
		return err
	}
	return s.meals.DeleteMeal(ctx, householdID, date(day))
}

func (s *Cook) SuggestPlan(ctx context.Context, actor domain.User, householdID uuid.UUID, from time.Time, days int, wish, lang string) (domain.MealIdeas, error) {
	if err := s.planReady(ctx, actor, householdID, true); err != nil {
		return domain.MealIdeas{}, err
	}
	wish = strings.TrimSpace(wish)
	if days < 1 || days > domain.MaxPlanDays || utf8.RuneCountInString(wish) > domain.MaxMood {
		return domain.MealIdeas{}, fmt.Errorf("%w: days must be 1-%d, the wish at most %d characters", domain.ErrInvalid, domain.MaxPlanDays, domain.MaxMood)
	}
	from = date(from)
	to := from.AddDate(0, 0, days)
	past, err := s.meals.Meals(ctx, householdID, from.AddDate(0, 0, -domain.RecentMealDays), to)
	if err != nil {
		return domain.MealIdeas{}, err
	}
	planned := map[time.Time]bool{}
	cooked := map[string]bool{}
	var recent []string
	for _, m := range past {
		if !m.Day.Before(from) {
			planned[m.Day] = true
		}
		if !cooked[m.Recipe.Slug] {
			cooked[m.Recipe.Slug] = true
			recent = append(recent, m.Recipe.Name)
		}
	}
	var open []time.Time
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if !planned[d] {
			open = append(open, d)
		}
	}
	none := domain.MealIdeas{Meals: []domain.MealIdea{}}
	if len(open) == 0 {
		return none, nil
	}
	all, err := s.book.Recipes(ctx)
	if err != nil {
		return domain.MealIdeas{}, err
	}
	// Recipes cooked lately are left out, unless that leaves too few to fill the days.
	fresh := slices.DeleteFunc(slices.Clone(all), func(r domain.RecipeSummary) bool { return cooked[r.Slug] })
	if len(fresh) < len(open) {
		fresh = all
	}
	if len(fresh) == 0 {
		return none, nil
	}

	var picks []domain.Suggestion
	chef := false
	if s.chef != nil {
		ps, err := s.chef.Plan(ctx, wish, lang, s.onHand(ctx, actor, householdID), recent, fresh, len(open))
		if err != nil {
			slog.WarnContext(ctx, "chef unavailable, planning at random", "err", err)
		}
		picks = chosen(fresh, ps, len(open))
		chef = len(picks) > 0
	}
	// Days the chef left open get a random recipe it didn't pick.
	rest := slices.DeleteFunc(slices.Clone(fresh), func(r domain.RecipeSummary) bool {
		return slices.ContainsFunc(picks, func(p domain.Suggestion) bool { return p.Recipe.Slug == r.Slug })
	})
	picks = append(picks, random(rest, len(open)-len(picks))...)

	out := domain.MealIdeas{Meals: make([]domain.MealIdea, 0, len(picks)), Chef: chef}
	for i, p := range picks {
		out.Meals = append(out.Meals, domain.MealIdea{Day: open[i], Recipe: p.Recipe, Reason: p.Reason})
	}
	return out, nil
}

// onHand is what was ticked off the shopping list lately, so it's probably in the kitchen.
func (s *Cook) onHand(ctx context.Context, actor domain.User, householdID uuid.UUID) []string {
	if s.shopping == nil {
		return nil
	}
	items, err := s.shopping.Items(ctx, actor, householdID)
	if err != nil {
		slog.WarnContext(ctx, "reading the shopping list failed", "err", err)
		return nil
	}
	var out []string
	for _, it := range items {
		if it.DoneAt != nil && len(out) < 50 {
			out = append(out, it.Text)
		}
	}
	return out
}

// planReady is ready for the meal plan; reading and changing it works without Mealie.
func (s *Cook) planReady(ctx context.Context, actor domain.User, householdID uuid.UUID, mealie bool) error {
	var err error
	if mealie {
		err = s.ready(ctx, actor, householdID)
	} else {
		_, err = s.households.Role(ctx, householdID, actor.ID)
	}
	if err != nil {
		return err
	}
	if s.meals == nil {
		return fmt.Errorf("%w: no meal plan", domain.ErrUnavailable)
	}
	return nil
}

// date is the calendar date of t, at midnight UTC.
func date(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
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

// chosen keeps up to n picks that name a real candidate, once each: an LLM can invent slugs.
func chosen(all []domain.RecipeSummary, picks []domain.Pick, n int) []domain.Suggestion {
	bySlug := make(map[string]domain.RecipeSummary, len(all))
	for _, r := range all {
		bySlug[r.Slug] = r
	}
	out := []domain.Suggestion{}
	for _, p := range picks {
		r, ok := bySlug[p.Slug]
		if !ok || len(out) == n {
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
