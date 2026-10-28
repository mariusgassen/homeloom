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
		Connected: home.Connected, Scenes: make([]oapi.Scene, len(home.Scenes)), Lights: make([]oapi.Light, len(home.Lights)),
	}
	for i, s := range home.Scenes {
		out.Scenes[i] = oapi.Scene{Id: s.ID, Name: s.Name}
	}
	for i, l := range home.Lights {
		out.Lights[i] = toLight(l)
	}
	return out, nil
}

func (h *handlers) ConnectHome(ctx context.Context, req oapi.ConnectHomeRequestObject) (oapi.ConnectHomeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	url, err := h.home.Connect(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ConnectHome404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.ConnectHome503JSONResponse{UnavailableJSONResponse: homeUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.ConnectHome200JSONResponse{Url: url}, nil
}

func (h *handlers) CompleteHomeConnection(ctx context.Context, req oapi.CompleteHomeConnectionRequestObject) (oapi.CompleteHomeConnectionResponseObject, error) {
	back := func(code string) oapi.CompleteHomeConnection302Response {
		loc := "/home"
		if code != "" {
			loc += "?home_error=" + code
		}
		return oapi.CompleteHomeConnection302Response{Headers: oapi.CompleteHomeConnection302ResponseHeaders{Location: loc}}
	}
	if req.Params.Error != nil {
		return back("connect_failed"), nil
	}
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.home.CompleteConnect(ctx, u, deref(req.Params.State), deref(req.Params.Code))
	switch {
	case errors.Is(err, domain.ErrUnauthenticated):
		slog.InfoContext(ctx, "home assistant connection rejected", "err", err)
		return back("connect_failed"), nil
	case err != nil:
		slog.WarnContext(ctx, "home assistant connection failed", "err", err)
		return back("home_unavailable"), nil
	}
	return back(""), nil
}

func (h *handlers) DisconnectHome(ctx context.Context, req oapi.DisconnectHomeRequestObject) (oapi.DisconnectHomeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.home.Disconnect(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DisconnectHome404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.DisconnectHome503JSONResponse{UnavailableJSONResponse: homeUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DisconnectHome204Response{}, nil
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
