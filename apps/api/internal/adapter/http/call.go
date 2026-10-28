package http

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) JoinCall(ctx context.Context, req oapi.JoinCallRequestObject) (oapi.JoinCallResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.calls.Join(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.JoinCall404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		slog.WarnContext(ctx, "calls unavailable", "err", err)
		return oapi.JoinCall503JSONResponse{UnavailableJSONResponse: oapi.UnavailableJSONResponse(errorBody(ctx, "calls_unavailable", ""))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.JoinCall200JSONResponse{Url: c.URL, Token: c.Token, Room: c.Room}, nil
}
