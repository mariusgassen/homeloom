package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) GetPushConfig(ctx context.Context, _ oapi.GetPushConfigRequestObject) (oapi.GetPushConfigResponseObject, error) {
	key, err := h.push.PublicKey(ctx)
	switch {
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.GetPushConfig503JSONResponse{UnavailableJSONResponse: pushUnavailable(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.GetPushConfig200JSONResponse{PublicKey: key}, nil
}

func (h *handlers) SubscribePush(ctx context.Context, req oapi.SubscribePushRequestObject) (oapi.SubscribePushResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.push.Subscribe(ctx, u, domain.PushSubscription{
		Endpoint: req.Body.Endpoint,
		P256dh:   req.Body.Keys.P256dh,
		Auth:     req.Body.Keys.Auth,
		Lang:     i18n.FromContext(ctx).String(),
	})
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SubscribePush400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SubscribePush503JSONResponse{UnavailableJSONResponse: pushUnavailable(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SubscribePush204Response{}, nil
}

func (h *handlers) UnsubscribePush(ctx context.Context, req oapi.UnsubscribePushRequestObject) (oapi.UnsubscribePushResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.push.Unsubscribe(ctx, u, req.Params.Endpoint); err != nil {
		return nil, err
	}
	return oapi.UnsubscribePush204Response{}, nil
}

func (h *handlers) ListPushTopics(ctx context.Context, _ oapi.ListPushTopicsRequestObject) (oapi.ListPushTopicsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	topics, err := h.push.Topics(ctx, u)
	if err != nil {
		return nil, err
	}
	out := make([]oapi.PushTopic, len(topics))
	for i, t := range topics {
		out[i] = oapi.PushTopic{Topic: oapi.PushTopicName(t.Topic), Enabled: t.Enabled}
	}
	return oapi.ListPushTopics200JSONResponse(out), nil
}

func (h *handlers) SetPushTopic(ctx context.Context, req oapi.SetPushTopicRequestObject) (oapi.SetPushTopicResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.push.SetTopic(ctx, u, domain.PushTopic(req.Topic), req.Body.Enabled)
	if errors.Is(err, domain.ErrInvalid) {
		return oapi.SetPushTopic400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.SetPushTopic204Response{}, nil
}

func pushUnavailable(ctx context.Context) oapi.UnavailableJSONResponse {
	return oapi.UnavailableJSONResponse(errorBody(ctx, "push_unavailable", ""))
}
