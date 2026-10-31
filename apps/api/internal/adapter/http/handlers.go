package http

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

// handlers translates between the generated API types and the core. It holds no business rules.
type handlers struct {
	version    string
	publicURL  string
	households port.HouseholdService
	avatars    port.AvatarService
	questions  port.QuestionService
	entries    port.EntryService
	moments    port.MomentService
	cook       port.CookService
	shopping   port.ShoppingService
	chores     port.ChoreService
	budget     port.BudgetService
	home       port.HomeService
	watch      port.WatchService
	push       port.PushService
	calls      port.CallService
	voice      port.VoiceService
	assistant  port.AssistantService
	agents     port.AgentAccessService
	login      port.LoginService
	cookies    cookies
	checks     []port.HealthCheck
}

var _ oapi.StrictServerInterface = (*handlers)(nil)

func (h *handlers) GetHealth(context.Context, oapi.GetHealthRequestObject) (oapi.GetHealthResponseObject, error) {
	return oapi.GetHealth200JSONResponse{Status: "ok", Version: h.version}, nil
}

func (h *handlers) GetReadiness(ctx context.Context, _ oapi.GetReadinessRequestObject) (oapi.GetReadinessResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	res := oapi.Readiness{Status: oapi.Ok, Checks: map[string]string{}}
	for _, c := range h.checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := "ok"
			if err := c.Check(ctx); err != nil {
				status = "down"
			}
			mu.Lock()
			defer mu.Unlock()
			res.Checks[c.Name()] = status
			if status != "ok" {
				res.Status = oapi.Degraded
			}
		}()
	}
	wg.Wait()
	if res.Status != oapi.Ok {
		return oapi.GetReadiness503JSONResponse(res), nil
	}
	return oapi.GetReadiness200JSONResponse(res), nil
}

func (h *handlers) GetMe(ctx context.Context, _ oapi.GetMeRequestObject) (oapi.GetMeResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	return oapi.GetMe200JSONResponse(toUser(u)), nil
}

func (h *handlers) ListHouseholds(ctx context.Context, _ oapi.ListHouseholdsRequestObject) (oapi.ListHouseholdsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ms, err := h.households.List(ctx, u)
	if err != nil {
		return nil, err
	}
	out := make(oapi.ListHouseholds200JSONResponse, len(ms))
	for i, m := range ms {
		out[i] = toHousehold(m)
	}
	return out, nil
}

func (h *handlers) CreateHousehold(ctx context.Context, req oapi.CreateHouseholdRequestObject) (oapi.CreateHouseholdResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.households.Create(ctx, u, req.Body.Name)
	if errors.Is(err, domain.ErrInvalid) {
		return oapi.CreateHousehold400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_household_name", err.Error(), "max", domain.MaxHouseholdName))}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.CreateHousehold201JSONResponse(toHousehold(m)), nil
}

func (h *handlers) GetHousehold(ctx context.Context, req oapi.GetHouseholdRequestObject) (oapi.GetHouseholdResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.households.Get(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetHousehold404JSONResponse{NotFoundJSONResponse: oapi.NotFoundJSONResponse(errorBody(ctx, "not_found", ""))}, nil
	}
	if err != nil {
		return nil, err
	}
	members := make([]oapi.Member, len(d.Members))
	for i, m := range d.Members {
		members[i] = oapi.Member{User: toUser(m.User), Role: oapi.Role(m.Role)}
	}
	return oapi.GetHousehold200JSONResponse{
		Id:      d.Household.ID,
		Name:    d.Household.Name,
		Role:    oapi.Role(d.Role),
		Members: members,
	}, nil
}

func toUser(u domain.User) oapi.User {
	return oapi.User{Id: u.ID, Name: u.Name, Email: optional(u.Email), AvatarUrl: optional(avatarURL(u))}
}

func toHousehold(m domain.Membership) oapi.Household {
	return oapi.Household{Id: m.Household.ID, Name: m.Household.Name, Role: oapi.Role(m.Role)}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// errorBody builds an error in the request's language from the catalog key "error.<code>".
func errorBody(ctx context.Context, code, detail string, args ...any) oapi.Error {
	return oapi.Error{Code: code, Message: i18n.T(i18n.FromContext(ctx), "error."+code, args...), Detail: optional(detail)}
}
