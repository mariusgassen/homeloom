package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListMcpTokens(ctx context.Context, req oapi.ListMcpTokensRequestObject) (oapi.ListMcpTokensResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ts, err := h.agents.Tokens(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListMcpTokens404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListMcpTokens200JSONResponse, len(ts))
	for i, t := range ts {
		out[i] = toMcpToken(t)
	}
	return out, nil
}

func (h *handlers) CreateMcpToken(ctx context.Context, req oapi.CreateMcpTokenRequestObject) (oapi.CreateMcpTokenResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	access := domain.AccessTools
	if req.Body.Access != nil {
		access = domain.TokenAccess(*req.Body.Access)
	}
	t, secret, err := h.agents.CreateToken(ctx, u, req.HouseholdId, req.Body.Name, access)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateMcpToken404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateMcpToken400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxMCPTokenName))}, nil
	case err != nil:
		return nil, err
	}
	sum := toMcpToken(t)
	return oapi.CreateMcpToken201JSONResponse{Id: sum.Id, Name: sum.Name, Access: sum.Access, CreatedAt: sum.CreatedAt, Secret: secret}, nil
}

func (h *handlers) DeleteMcpToken(ctx context.Context, req oapi.DeleteMcpTokenRequestObject) (oapi.DeleteMcpTokenResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.agents.DeleteToken(ctx, u, req.TokenId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteMcpToken404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteMcpToken204Response{}, nil
}

func toMcpToken(t domain.MCPToken) oapi.McpToken {
	return oapi.McpToken{Id: t.ID, Name: t.Name, Access: oapi.TokenAccess(t.Access), CreatedAt: t.CreatedAt}
}
