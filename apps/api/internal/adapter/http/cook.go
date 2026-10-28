package http

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) SuggestRecipes(ctx context.Context, req oapi.SuggestRecipesRequestObject) (oapi.SuggestRecipesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	s, err := h.cook.Suggest(ctx, u, req.HouseholdId, req.Body.Mood, i18n.FromContext(ctx).String())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SuggestRecipes404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SuggestRecipes400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxMood))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SuggestRecipes503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.SuggestRecipes200JSONResponse{Chef: s.Chef, Suggestions: make([]oapi.Suggestion, len(s.Recipes))}
	for i, r := range s.Recipes {
		out.Suggestions[i] = oapi.Suggestion{Recipe: toRecipeSummary(r.Recipe), Reason: optional(r.Reason)}
	}
	return out, nil
}

func (h *handlers) GetRecipe(ctx context.Context, req oapi.GetRecipeRequestObject) (oapi.GetRecipeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	r, err := h.cook.Recipe(ctx, u, req.HouseholdId, req.Slug)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetRecipe404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetRecipe503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	sum := toRecipeSummary(r.RecipeSummary)
	out := oapi.GetRecipe200JSONResponse{
		Slug: sum.Slug, Name: sum.Name, Description: sum.Description, TotalTime: sum.TotalTime, Tags: sum.Tags,
		Servings: optional(r.Servings), MealieUrl: optional(r.URL),
		Ingredients: make([]oapi.Ingredient, len(r.Ingredients)), Steps: r.Steps,
	}
	for i, in := range r.Ingredients {
		out.Ingredients[i] = oapi.Ingredient{Id: in.ID, Text: in.Text}
	}
	if out.Steps == nil {
		out.Steps = []string{}
	}
	return out, nil
}

func (h *handlers) AddToShoppingList(ctx context.Context, req oapi.AddToShoppingListRequestObject) (oapi.AddToShoppingListResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	added, err := h.cook.AddToShoppingList(ctx, u, req.HouseholdId, req.Body.Recipe, req.Body.IngredientIds)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddToShoppingList404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddToShoppingList400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_ingredients", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.AddToShoppingList503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddToShoppingList200JSONResponse{Added: added, List: i18n.T(i18n.FromContext(ctx), "shopping.list")}, nil
}

func toRecipeSummary(r domain.RecipeSummary) oapi.RecipeSummary {
	tags := r.Tags
	if tags == nil {
		tags = []string{}
	}
	return oapi.RecipeSummary{Slug: r.Slug, Name: r.Name, Description: optional(r.Description), TotalTime: optional(r.TotalTime), Tags: tags}
}

// cookingUnavailable answers when Mealie is down or not set up; the cause is logged, not sent.
func cookingUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "cooking unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "cooking_unavailable", ""))
}
