package http

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) GetGifts(ctx context.Context, req oapi.GetGiftsRequestObject) (oapi.GetGiftsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	g, err := h.gifts.Overview(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetGifts404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.Gifts{MyBirthday: toBirthday(g.MyBirthday), Occasions: make([]oapi.Occasion, len(g.Occasions)), People: make([]oapi.GiftPerson, len(g.People)), Ideas: make([]oapi.GiftIdea, len(g.Ideas))}
	for i, p := range g.People {
		out.People[i] = oapi.GiftPerson{User: toUser(p.User), Birthday: toBirthday(p.Birthday), Ideas: p.Ideas}
		if p.Birthday != nil {
			next, days := openapi_types.Date{Time: p.Next}, p.Days
			out.People[i].Next, out.People[i].Days = &next, &days
		}
	}
	for i, o := range g.Occasions {
		out.Occasions[i] = toOccasion(o.Occasion, o.Next, o.Days)
	}
	for i, idea := range g.Ideas {
		out.Ideas[i] = toGiftIdea(idea)
	}
	return oapi.GetGifts200JSONResponse(out), nil
}

func (h *handlers) AddGiftIdea(ctx context.Context, req oapi.AddGiftIdeaRequestObject) (oapi.AddGiftIdeaResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	idea, err := h.gifts.Add(ctx, u, req.HouseholdId, req.Body.ForId, req.Body.OccasionId, req.Body.Text)
	var te domain.TextError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddGiftIdea404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.As(err, &te):
		return oapi.AddGiftIdea400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", te.Max))}, nil
	case errors.Is(err, domain.ErrOccasion):
		return oapi.AddGiftIdea400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_occasion", err.Error()))}, nil
	case errors.Is(err, domain.ErrGiftFor):
		return oapi.AddGiftIdea400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_gift_for", err.Error()))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AddGiftIdea201JSONResponse(toGiftIdea(idea)), nil
}

func (h *handlers) AddOccasion(ctx context.Context, req oapi.AddOccasionRequestObject) (oapi.AddOccasionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	date := domain.Birthday{Month: time.Month(req.Body.Month), Day: req.Body.Day, Year: derefInt(req.Body.Year)}
	o, err := h.gifts.AddOccasion(ctx, u, req.HouseholdId, req.Body.Name, date)
	var te domain.TextError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AddOccasion404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.As(err, &te):
		return oapi.AddOccasion400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", te.Max))}, nil
	case errors.Is(err, domain.ErrBirthday):
		return oapi.AddOccasion400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_birthday", err.Error()))}, nil
	case err != nil:
		return nil, err
	}
	next, days := o.Date.Next(time.Now())
	return oapi.AddOccasion201JSONResponse(toOccasion(o, next, days)), nil
}

func (h *handlers) DeleteOccasion(ctx context.Context, req oapi.DeleteOccasionRequestObject) (oapi.DeleteOccasionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.gifts.DeleteOccasion(ctx, u, req.HouseholdId, req.OccasionId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteOccasion404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteOccasion204Response{}, nil
}

func (h *handlers) UpdateGiftIdea(ctx context.Context, req oapi.UpdateGiftIdeaRequestObject) (oapi.UpdateGiftIdeaResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	idea, err := h.gifts.SetGiven(ctx, u, req.HouseholdId, req.IdeaId, req.Body.Given)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateGiftIdea404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateGiftIdea200JSONResponse(toGiftIdea(idea)), nil
}

func (h *handlers) DeleteGiftIdea(ctx context.Context, req oapi.DeleteGiftIdeaRequestObject) (oapi.DeleteGiftIdeaResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.gifts.Delete(ctx, u, req.HouseholdId, req.IdeaId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteGiftIdea404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteGiftIdea204Response{}, nil
}

func (h *handlers) SetBirthday(ctx context.Context, req oapi.SetBirthdayRequestObject) (oapi.SetBirthdayResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.gifts.SetBirthday(ctx, u, &domain.Birthday{Month: time.Month(req.Body.Month), Day: req.Body.Day, Year: derefInt(req.Body.Year)})
	switch {
	case errors.Is(err, domain.ErrBirthday):
		return oapi.SetBirthday400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_birthday", err.Error()))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SetBirthday204Response{}, nil
}

func (h *handlers) DeleteBirthday(ctx context.Context, _ oapi.DeleteBirthdayRequestObject) (oapi.DeleteBirthdayResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.gifts.SetBirthday(ctx, u, nil); err != nil {
		return nil, err
	}
	return oapi.DeleteBirthday204Response{}, nil
}

func toBirthday(b *domain.Birthday) *oapi.Birthday {
	if b == nil {
		return nil
	}
	out := &oapi.Birthday{Month: int(b.Month), Day: b.Day}
	if b.Year != 0 {
		out.Year = &b.Year
	}
	return out
}

func toOccasion(o domain.Occasion, next time.Time, days int) oapi.Occasion {
	out := oapi.Occasion{Id: o.ID, Name: o.Name, Month: int(o.Date.Month), Day: o.Date.Day, Next: openapi_types.Date{Time: next}, Days: days}
	if o.Date.Year != 0 {
		out.Year = &o.Date.Year
	}
	return out
}

func toGiftIdea(i domain.GiftIdea) oapi.GiftIdea {
	return oapi.GiftIdea{Id: i.ID, ForId: i.ForID, OccasionId: i.OccasionID, Text: i.Text, Given: i.GivenAt != nil, GivenAt: i.GivenAt, CreatedAt: i.Created}
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
