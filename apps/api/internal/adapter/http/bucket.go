package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListBucket(ctx context.Context, req oapi.ListBucketRequestObject) (oapi.ListBucketResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var kind domain.BucketKind
	if req.Params.Kind != nil {
		kind = domain.BucketKind(*req.Params.Kind)
	}
	ws, err := h.bucket.List(ctx, u, req.HouseholdId, kind)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListBucket404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListBucket200JSONResponse, len(ws))
	for i, w := range ws {
		out[i] = toBucketItem(w)
	}
	return out, nil
}

func (h *handlers) AddBucketItem(ctx context.Context, req oapi.AddBucketItemRequestObject) (oapi.AddBucketItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.bucket.Add(ctx, u, req.HouseholdId, domain.BucketKind(req.Body.Kind), req.Body.Text)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddBucketItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AddBucketItem400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxBucketText))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddBucketItem201JSONResponse(toBucketItem(w)), nil
}

func (h *handlers) DeleteBucketItem(ctx context.Context, req oapi.DeleteBucketItemRequestObject) (oapi.DeleteBucketItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.bucket.Delete(ctx, u, req.HouseholdId, req.ItemId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteBucketItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteBucketItem204Response{}, nil
}

func (h *handlers) TickBucketItem(ctx context.Context, req oapi.TickBucketItemRequestObject) (oapi.TickBucketItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.bucket.SetDone(ctx, u, req.HouseholdId, req.ItemId, true)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.TickBucketItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.TickBucketItem200JSONResponse(toBucketItem(w)), nil
}

func (h *handlers) UntickBucketItem(ctx context.Context, req oapi.UntickBucketItemRequestObject) (oapi.UntickBucketItemResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	w, err := h.bucket.SetDone(ctx, u, req.HouseholdId, req.ItemId, false)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UntickBucketItem404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UntickBucketItem200JSONResponse(toBucketItem(w)), nil
}

func toBucketItem(w domain.BucketItem) oapi.BucketItem {
	return oapi.BucketItem{Id: w.ID, Kind: oapi.BucketKind(w.Kind), Text: w.Text, AddedBy: w.AddedBy, Done: w.DoneAt != nil, DoneAt: w.DoneAt, CreatedAt: w.Created}
}
