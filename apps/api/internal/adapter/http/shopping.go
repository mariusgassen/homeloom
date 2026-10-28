package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListShoppingItems(ctx context.Context, req oapi.ListShoppingItemsRequestObject) (oapi.ListShoppingItemsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.shopping.Items(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListShoppingItems404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListShoppingItems200JSONResponse, len(items))
	for i, it := range items {
		out[i] = toShoppingItem(it)
	}
	return out, nil
}

func (h *handlers) AddShoppingItem(ctx context.Context, req oapi.AddShoppingItemRequestObject) (oapi.AddShoppingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.shopping.Add(ctx, u, req.HouseholdId, []string{req.Body.Text}, "")
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddShoppingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddShoppingItem400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxShoppingText))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddShoppingItem201JSONResponse(toShoppingItem(items[0])), nil
}

func (h *handlers) DeleteShoppingItem(ctx context.Context, req oapi.DeleteShoppingItemRequestObject) (oapi.DeleteShoppingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.shopping.Delete(ctx, u, req.HouseholdId, req.ItemId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteShoppingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteShoppingItem204Response{}, nil
}

func (h *handlers) TickShoppingItem(ctx context.Context, req oapi.TickShoppingItemRequestObject) (oapi.TickShoppingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	it, err := h.shopping.SetDone(ctx, u, req.HouseholdId, req.ItemId, true)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.TickShoppingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.TickShoppingItem200JSONResponse(toShoppingItem(it)), nil
}

func (h *handlers) UntickShoppingItem(ctx context.Context, req oapi.UntickShoppingItemRequestObject) (oapi.UntickShoppingItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	it, err := h.shopping.SetDone(ctx, u, req.HouseholdId, req.ItemId, false)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UntickShoppingItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UntickShoppingItem200JSONResponse(toShoppingItem(it)), nil
}

func (h *handlers) ClearShoppingItems(ctx context.Context, req oapi.ClearShoppingItemsRequestObject) (oapi.ClearShoppingItemsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.shopping.ClearDone(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ClearShoppingItems404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.ClearShoppingItems204Response{}, nil
}

func toShoppingItem(it domain.ShoppingItem) oapi.ShoppingItem {
	return oapi.ShoppingItem{
		Id: it.ID, Text: it.Text, Recipe: optional(it.Recipe), AddedBy: it.AddedBy,
		Done: it.DoneAt != nil, DoneAt: it.DoneAt, CreatedAt: it.Created,
	}
}
