package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) SetLocation(ctx context.Context, req oapi.SetLocationRequestObject) (oapi.SetLocationResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.location.Set(ctx, u, domain.Coords{Latitude: req.Body.Latitude, Longitude: req.Body.Longitude}, i18n.FromContext(ctx).String())
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SetLocation400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetLocation204Response{}, nil
}

func (h *handlers) DeleteLocation(ctx context.Context, _ oapi.DeleteLocationRequestObject) (oapi.DeleteLocationResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.location.Clear(ctx, u); err != nil {
		return nil, err
	}
	return oapi.DeleteLocation204Response{}, nil
}
