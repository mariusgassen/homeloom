package http

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// guestCallPath is where guest links are redeemed; the token follows it.
const guestCallPath = "/v1/guest-calls/"

func (h *handlers) StartCall(ctx context.Context, req oapi.StartCallRequestObject) (oapi.StartCallResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.calls.Start(ctx, u, req.HouseholdId, req.Body.Invite)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.StartCall400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	case errors.Is(err, domain.ErrNotFound):
		return oapi.StartCall404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.StartCall503JSONResponse{UnavailableJSONResponse: callsUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.StartCall201JSONResponse(toCall(c)), nil
}

func (h *handlers) JoinCall(ctx context.Context, req oapi.JoinCallRequestObject) (oapi.JoinCallResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.calls.Join(ctx, u, req.HouseholdId, req.CallId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.JoinCall404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.JoinCall503JSONResponse{UnavailableJSONResponse: callsUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.JoinCall200JSONResponse(toCall(c)), nil
}

func (h *handlers) InviteToCall(ctx context.Context, req oapi.InviteToCallRequestObject) (oapi.InviteToCallResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.calls.Invite(ctx, u, req.HouseholdId, req.CallId, req.Body.Invite)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.InviteToCall400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	case errors.Is(err, domain.ErrNotFound):
		return oapi.InviteToCall404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.InviteToCall503JSONResponse{UnavailableJSONResponse: callsUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.InviteToCall204Response{}, nil
}

func (h *handlers) CreateGuestLink(ctx context.Context, req oapi.CreateGuestLinkRequestObject) (oapi.CreateGuestLinkResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	link, err := h.calls.GuestLink(ctx, u, req.HouseholdId, req.CallId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateGuestLink404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.CreateGuestLink503JSONResponse{UnavailableJSONResponse: callsUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateGuestLink201JSONResponse{Token: link.Token, ExpiresAt: link.ExpiresAt}, nil
}

func (h *handlers) GetGuestCall(ctx context.Context, req oapi.GetGuestCallRequestObject) (oapi.GetGuestCallResponseObject, error) {
	pass, err := h.calls.GuestPass(ctx, req.Token)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetGuestCall404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.GetGuestCall200JSONResponse{InvitedBy: pass.InvitedBy}, nil
}

func (h *handlers) JoinGuestCall(ctx context.Context, req oapi.JoinGuestCallRequestObject) (oapi.JoinGuestCallResponseObject, error) {
	c, err := h.calls.JoinAsGuest(ctx, req.Token, req.Body.Name)
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.JoinGuestCall400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	case errors.Is(err, domain.ErrNotFound):
		return oapi.JoinGuestCall404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.JoinGuestCall503JSONResponse{UnavailableJSONResponse: callsUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.JoinGuestCall200JSONResponse(toCall(c)), nil
}

func toCall(c domain.Call) oapi.Call {
	return oapi.Call{Id: c.ID, Url: c.URL, Token: c.Token, Room: c.Room}
}

func badRequest(ctx context.Context, err error) oapi.BadRequestJSONResponse {
	return oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))
}

func callsUnavailable(ctx context.Context, err error) oapi.UnavailableJSONResponse {
	slog.WarnContext(ctx, "calls unavailable", "err", err)
	return oapi.UnavailableJSONResponse(errorBody(ctx, "calls_unavailable", ""))
}
