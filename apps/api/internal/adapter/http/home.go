package http

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) GetHome(ctx context.Context, req oapi.GetHomeRequestObject) (oapi.GetHomeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	home, err := h.home.Get(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetHome404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetHome503JSONResponse{UnavailableJSONResponse: homeUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.GetHome200JSONResponse{
		Scenes: make([]oapi.Scene, len(home.Scenes)), Lights: make([]oapi.Light, len(home.Lights)),
	}
	for i, s := range home.Scenes {
		out.Scenes[i] = oapi.Scene{Id: s.ID, Name: s.Name}
	}
	for i, l := range home.Lights {
		out.Lights[i] = toLight(l)
	}
	return out, nil
}

func (h *handlers) ActivateScene(ctx context.Context, req oapi.ActivateSceneRequestObject) (oapi.ActivateSceneResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.home.ActivateScene(ctx, u, req.HouseholdId, req.SceneId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ActivateScene404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.ActivateScene503JSONResponse{UnavailableJSONResponse: homeUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.ActivateScene204Response{}, nil
}

func (h *handlers) ToggleLight(ctx context.Context, req oapi.ToggleLightRequestObject) (oapi.ToggleLightResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	l, err := h.home.ToggleLight(ctx, u, req.HouseholdId, req.LightId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ToggleLight404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.ToggleLight503JSONResponse{UnavailableJSONResponse: homeUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.ToggleLight200JSONResponse(toLight(l)), nil
}

func toLight(l domain.Light) oapi.Light {
	return oapi.Light{Id: l.ID, Name: l.Name, On: l.On}
}

// homeUnavailable answers when Home Assistant is down or not set up; the cause is logged, not sent.
func homeUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "home unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "home_unavailable", ""))
}
