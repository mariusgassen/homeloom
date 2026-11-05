package http

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) SearchPlaces(ctx context.Context, req oapi.SearchPlacesRequestObject) (oapi.SearchPlacesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	found, err := h.places.Search(ctx, u, req.Params.Q, i18n.FromContext(ctx).String())
	if errors.Is(err, domain.ErrUnavailable) {
		slog.WarnContext(ctx, "place search unavailable", "err", err)
		return oapi.SearchPlaces503JSONResponse{UnavailableJSONResponse: oapi.UnavailableJSONResponse(errorBody(ctx, "places_unavailable", ""))}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(oapi.SearchPlaces200JSONResponse, len(found))
	for i, p := range found {
		out[i] = oapi.PlaceMatch{Label: p.Label}
	}
	return out, nil
}
