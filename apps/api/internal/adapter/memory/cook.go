package memory

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// RecipeBook is a stand-in for Mealie in tests. Set Down to make every call unavailable.
type RecipeBook struct {
	mu      sync.Mutex
	recipes []domain.Recipe
	Down    bool
}

func NewRecipeBook(recipes ...domain.Recipe) *RecipeBook {
	return &RecipeBook{recipes: recipes}
}

func (b *RecipeBook) Recipes(context.Context) ([]domain.RecipeSummary, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Down {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.RecipeSummary, len(b.recipes))
	for i, r := range b.recipes {
		out[i] = r.RecipeSummary
	}
	return out, nil
}

func (b *RecipeBook) Recipe(_ context.Context, slug string) (domain.Recipe, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Down {
		return domain.Recipe{}, domain.ErrUnavailable
	}
	for _, r := range b.recipes {
		if r.Slug == slug {
			return r, nil
		}
	}
	return domain.Recipe{}, domain.ErrNotFound
}

// Image is a tiny stand-in picture for recipes with an Image version.
func (b *RecipeBook) Image(ctx context.Context, slug string) (domain.RecipeImage, error) {
	r, err := b.Recipe(ctx, slug)
	if err != nil {
		return domain.RecipeImage{}, err
	}
	if r.Image == "" {
		return domain.RecipeImage{}, domain.ErrNotFound
	}
	return domain.RecipeImage{ContentType: "image/webp", Body: []byte("RIFF-" + slug)}, nil
}

// Chef is a stand-in for the LLM: it picks the candidates whose name contains a word of the
// mood, and is unavailable for the mood "down". It also picks "made-up", which doesn't exist.
type Chef struct{}

func (Chef) Pick(_ context.Context, mood, lang string, candidates []domain.RecipeSummary, n int) ([]domain.Pick, error) {
	if mood == "down" {
		return nil, domain.ErrUnavailable
	}
	picks := []domain.Pick{{Slug: "made-up", Reason: "invented"}}
	for _, c := range candidates {
		for _, w := range strings.Fields(strings.ToLower(mood)) {
			if strings.Contains(strings.ToLower(c.Name), w) && len(picks) <= n {
				picks = append(picks, domain.Pick{Slug: c.Slug, Reason: lang + ": fits " + w})
				break
			}
		}
	}
	return picks, nil
}

// Plan picks the candidates whose name contains a word of the wish or of what is on hand, then
// fills up in order; it is unavailable for the wish "down" and also picks "made-up".
func (Chef) Plan(_ context.Context, wish, lang string, onHand, _ []string, candidates []domain.RecipeSummary, n int) ([]domain.Pick, error) {
	if wish == "down" {
		return nil, domain.ErrUnavailable
	}
	words := strings.Fields(strings.ToLower(wish + " " + strings.Join(onHand, " ")))
	picks := []domain.Pick{{Slug: "made-up", Reason: "invented"}}
	for _, c := range candidates {
		for _, w := range words {
			if strings.Contains(strings.ToLower(c.Name), w) && len(picks) <= n {
				picks = append(picks, domain.Pick{Slug: c.Slug, Reason: lang + ": uses " + w})
				break
			}
		}
	}
	return picks, nil
}

func (s *Store) SaveMeal(_ context.Context, m domain.PlannedMeal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meals = slices.DeleteFunc(s.meals, func(o domain.PlannedMeal) bool { return o.HouseholdID == m.HouseholdID && o.Day.Equal(m.Day) })
	s.meals = append(s.meals, m)
	return nil
}

func (s *Store) Meals(_ context.Context, householdID uuid.UUID, from, to time.Time) ([]domain.PlannedMeal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.PlannedMeal{}
	for _, m := range s.meals {
		if m.HouseholdID == householdID && !m.Day.Before(from) && m.Day.Before(to) {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b domain.PlannedMeal) int { return a.Day.Compare(b.Day) })
	return out, nil
}

func (s *Store) DeleteMeal(_ context.Context, householdID uuid.UUID, day time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meals = slices.DeleteFunc(s.meals, func(o domain.PlannedMeal) bool { return o.HouseholdID == householdID && o.Day.Equal(day) })
	return nil
}
