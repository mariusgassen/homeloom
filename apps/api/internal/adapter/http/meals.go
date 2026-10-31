package http

import (
	"context"
	"errors"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) GetMealPlan(ctx context.Context, req oapi.GetMealPlanRequestObject) (oapi.GetMealPlanResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	meals, err := h.cook.Plan(ctx, u, req.HouseholdId, req.Params.From.Time, days(req.Params.Days))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetMealPlan404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.GetMealPlan400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.GetMealPlan200JSONResponse, len(meals))
	for i, m := range meals {
		out[i] = toPlannedMeal(req.HouseholdId, m)
	}
	return out, nil
}

func (h *handlers) PlanMeal(ctx context.Context, req oapi.PlanMealRequestObject) (oapi.PlanMealResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.cook.PlanMeal(ctx, u, req.HouseholdId, req.Day.Time, req.Body.Recipe)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.PlanMeal404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.PlanMeal503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.PlanMeal200JSONResponse(toPlannedMeal(req.HouseholdId, m)), nil
}

func (h *handlers) UnplanMeal(ctx context.Context, req oapi.UnplanMealRequestObject) (oapi.UnplanMealResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.cook.UnplanMeal(ctx, u, req.HouseholdId, req.Day.Time)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UnplanMeal404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UnplanMeal204Response{}, nil
}

func (h *handlers) SuggestMealPlan(ctx context.Context, req oapi.SuggestMealPlanRequestObject) (oapi.SuggestMealPlanResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	wish := ""
	if req.Body.Wish != nil {
		wish = *req.Body.Wish
	}
	ideas, err := h.cook.SuggestPlan(ctx, u, req.HouseholdId, req.Body.From.Time, days(req.Body.Days), wish, i18n.FromContext(ctx).String())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SuggestMealPlan404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SuggestMealPlan400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SuggestMealPlan503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.SuggestMealPlan200JSONResponse{Chef: ideas.Chef, Meals: make([]oapi.MealIdea, len(ideas.Meals))}
	for i, m := range ideas.Meals {
		out.Meals[i] = oapi.MealIdea{Day: openapi_types.Date{Time: m.Day}, Recipe: toRecipeSummary(req.HouseholdId, m.Recipe), Reason: optional(m.Reason)}
	}
	return out, nil
}

func (h *handlers) AddMealPlanToShoppingList(ctx context.Context, req oapi.AddMealPlanToShoppingListRequestObject) (oapi.AddMealPlanToShoppingListResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	recipes := make([]domain.RecipeIngredients, len(req.Body.Recipes))
	for i, r := range req.Body.Recipes {
		recipes[i] = domain.RecipeIngredients{Slug: r.Recipe, IngredientIDs: r.IngredientIds}
	}
	added, err := h.cook.AddMealsToShoppingList(ctx, u, req.HouseholdId, recipes)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddMealPlanToShoppingList404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddMealPlanToShoppingList400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_ingredients", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.AddMealPlanToShoppingList503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddMealPlanToShoppingList200JSONResponse{Added: added, List: i18n.T(i18n.FromContext(ctx), "shopping.list")}, nil
}

// days is the number of days asked for, a week when not given.
func days(n *int) int {
	if n == nil {
		return 7
	}
	return *n
}

func toPlannedMeal(householdID uuid.UUID, m domain.PlannedMeal) oapi.PlannedMeal {
	return oapi.PlannedMeal{Day: openapi_types.Date{Time: m.Day}, Recipe: toRecipeSummary(householdID, m.Recipe), PlannedBy: m.PlannedBy}
}
