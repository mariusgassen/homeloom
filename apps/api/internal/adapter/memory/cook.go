package memory

import (
	"context"
	"strings"
	"sync"

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
