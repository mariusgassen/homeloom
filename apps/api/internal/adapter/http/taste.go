package http

import (
	"context"
	"errors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func toMealRating(r domain.MealRating) oapi.MealRating {
	return oapi.MealRating{Slug: r.Slug, Name: r.Name, UserId: r.UserID, Rating: oapi.Rating(r.Rating), RatedAt: r.RatedAt}
}

func (h *handlers) RateRecipe(ctx context.Context, req oapi.RateRecipeRequestObject) (oapi.RateRecipeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	r, err := h.cook.Rate(ctx, u, req.HouseholdId, req.Slug, domain.Rating(req.Body.Rating))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.RateRecipe404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.RateRecipe400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.RateRecipe503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.RateRecipe200JSONResponse(toMealRating(r)), nil
}

func (h *handlers) UnrateRecipe(ctx context.Context, req oapi.UnrateRecipeRequestObject) (oapi.UnrateRecipeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.cook.Unrate(ctx, u, req.HouseholdId, req.Slug)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UnrateRecipe404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.UnrateRecipe503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UnrateRecipe204Response{}, nil
}

func (h *handlers) GetTaste(ctx context.Context, req oapi.GetTasteRequestObject) (oapi.GetTasteResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := h.cook.Ratings(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetTaste404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetTaste503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.GetTaste200JSONResponse{Ratings: make([]oapi.MealRating, len(rs))}
	for i, r := range rs {
		out.Ratings[i] = toMealRating(r)
	}
	return out, nil
}

func toCookedDishes(ds []domain.CookedDish) []oapi.CookedDish {
	out := make([]oapi.CookedDish, len(ds))
	for i, d := range ds {
		out[i] = oapi.CookedDish{Slug: d.Slug, Name: d.Name, Times: d.Times, Last: openapi_types.Date{Time: d.Last}}
	}
	return out
}

func (h *handlers) GetMealRecap(ctx context.Context, req oapi.GetMealRecapRequestObject) (oapi.GetMealRecapResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	year := 0
	if req.Params.Year != nil {
		year = *req.Params.Year
	}
	r, err := h.cook.Recap(ctx, u, req.HouseholdId, year)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetMealRecap404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.GetMealRecap400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetMealRecap503JSONResponse{UnavailableJSONResponse: cookingUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.GetMealRecap200JSONResponse{
		Year: r.Year, Memories: r.Memories, Dishes: toCookedDishes(r.Dishes), Favourites: toCookedDishes(r.Favourites), Photos: toMoments(r.Photos),
	}, nil
}
