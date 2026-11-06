package http

import (
	"context"
	"errors"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) SuggestDateNight(ctx context.Context, req oapi.SuggestDateNightRequestObject) (oapi.SuggestDateNightResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	q := domain.DateQuery{Place: deref(req.Body.Place), Mood: deref(req.Body.Mood), Constraints: deref(req.Body.Constraints)}
	if l := req.Body.Location; l != nil {
		q.Location = &domain.Coords{Latitude: l.Latitude, Longitude: l.Longitude}
	}
	if req.Body.Budget != nil {
		q.Budget = domain.DateBudget(*req.Body.Budget)
	}
	if d := req.Body.Day; d != nil {
		q.Day = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	}
	res, err := h.dateNight.Ideas(ctx, u, req.HouseholdId, q, i18n.FromContext(ctx).String())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SuggestDateNight404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SuggestDateNight400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_date_night", err.Error()))}, nil
	case errors.Is(err, domain.ErrUnavailable):
		return oapi.SuggestDateNight503JSONResponse{UnavailableJSONResponse: ideasUnavailable(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.SuggestDateNight200JSONResponse{Ideas: make([]oapi.DateIdea, len(res.Ideas))}
	for i, x := range res.Ideas {
		out.Ideas[i] = oapi.DateIdea{
			Title: x.Title, Reason: x.Reason, Where: optional(x.Where), Cost: optional(x.Cost),
			Indoor: x.Indoor, StartsAt: x.StartsAt,
		}
	}
	if f := res.Forecast; f != nil {
		out.Forecast = &oapi.DateForecast{Place: optional(f.Place), Sky: oapi.DateForecastSky(f.Sky), Condition: oapi.DateForecastCondition(f.Condition), HighC: float32(f.HighC), LowC: float32(f.LowC), RainPercent: f.RainPercent}
	}
	return out, nil
}
