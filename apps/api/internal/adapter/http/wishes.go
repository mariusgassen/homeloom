package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListWishes(ctx context.Context, req oapi.ListWishesRequestObject) (oapi.ListWishesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var kind domain.WishKind
	if req.Params.Kind != nil {
		kind = domain.WishKind(*req.Params.Kind)
	}
	ws, err := h.wishes.List(ctx, u, req.HouseholdId, kind)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListWishes404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListWishes200JSONResponse, len(ws))
	for i, w := range ws {
		out[i] = toWish(w)
	}
	return out, nil
}

func (h *handlers) AddWish(ctx context.Context, req oapi.AddWishRequestObject) (oapi.AddWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.Add(ctx, u, req.HouseholdId, domain.WishKind(req.Body.Kind), req.Body.Text)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddWish400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxWishText))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddWish201JSONResponse(toWish(w)), nil
}

func (h *handlers) DeleteWish(ctx context.Context, req oapi.DeleteWishRequestObject) (oapi.DeleteWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.wishes.Delete(ctx, u, req.HouseholdId, req.WishId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteWish204Response{}, nil
}

func (h *handlers) TickWish(ctx context.Context, req oapi.TickWishRequestObject) (oapi.TickWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.SetDone(ctx, u, req.HouseholdId, req.WishId, true)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.TickWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.TickWish200JSONResponse(toWish(w)), nil
}

func (h *handlers) UntickWish(ctx context.Context, req oapi.UntickWishRequestObject) (oapi.UntickWishResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.wishes.SetDone(ctx, u, req.HouseholdId, req.WishId, false)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UntickWish404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UntickWish200JSONResponse(toWish(w)), nil
}

func toWish(w domain.Wish) oapi.Wish {
	return oapi.Wish{Id: w.ID, Kind: oapi.WishKind(w.Kind), Text: w.Text, AddedBy: w.AddedBy, Done: w.DoneAt != nil, DoneAt: w.DoneAt, CreatedAt: w.Created}
}
