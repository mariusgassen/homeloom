package http

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// calendarFeedPath is where calendar feeds are served; the token follows it.
const calendarFeedPath = "/v1/calendar-feeds/"

func (h *handlers) ListEntries(ctx context.Context, req oapi.ListEntriesRequestObject) (oapi.ListEntriesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var kind domain.EntryKind
	if req.Params.Kind != nil {
		kind = domain.EntryKind(*req.Params.Kind)
	}
	es, err := h.entries.List(ctx, u, req.HouseholdId, kind)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.ListEntries404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(oapi.ListEntries200JSONResponse, len(es))
	for i, e := range es {
		out[i] = toEntry(e)
	}
	return out, nil
}

func (h *handlers) ListReminderSuggestions(ctx context.Context, req oapi.ListReminderSuggestionsRequestObject) (oapi.ListReminderSuggestionsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ss, err := h.entries.Suggestions(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListReminderSuggestions404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.ListReminderSuggestions200JSONResponse(toTitleSuggestions(ss)), nil
}

func (h *handlers) CreateEntry(ctx context.Context, req oapi.CreateEntryRequestObject) (oapi.CreateEntryResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	e, err := h.entries.Create(ctx, u, req.HouseholdId, domain.EntryKind(b.Kind), toEntryFields(oapi.EntryFields{
		Text: b.Text, Visibility: b.Visibility, RevealAt: b.RevealAt, DueAt: b.DueAt, StartsAt: b.StartsAt, EndsAt: b.EndsAt,
		AllDay: b.AllDay, AlertMinutes: b.AlertMinutes, Repeat: b.Repeat, AssigneeId: b.AssigneeId,
	}))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateEntry404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateEntry400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidEntry(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateEntry201JSONResponse(toEntry(e)), nil
}

func (h *handlers) GetEntry(ctx context.Context, req oapi.GetEntryRequestObject) (oapi.GetEntryResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	e, err := h.entries.Get(ctx, u, req.EntryId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetEntry404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.GetEntry200JSONResponse(toEntry(e)), nil
}

func (h *handlers) UpdateEntry(ctx context.Context, req oapi.UpdateEntryRequestObject) (oapi.UpdateEntryResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	e, err := h.entries.Update(ctx, u, req.EntryId, toEntryFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateEntry404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.UpdateEntry403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateEntry400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidEntry(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateEntry200JSONResponse(toEntry(e)), nil
}

func (h *handlers) DeleteEntry(ctx context.Context, req oapi.DeleteEntryRequestObject) (oapi.DeleteEntryResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.entries.Delete(ctx, u, req.EntryId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteEntry404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.DeleteEntry403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteEntry204Response{}, nil
}

func (h *handlers) CompleteEntry(ctx context.Context, req oapi.CompleteEntryRequestObject) (oapi.CompleteEntryResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	e, err := h.entries.SetDone(ctx, u, req.EntryId, true)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CompleteEntry404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.CompleteEntry409JSONResponse(errorBody(ctx, "not_a_reminder", "")), nil
	case err != nil:
		return nil, err
	}
	return oapi.CompleteEntry200JSONResponse(toEntry(e)), nil
}

func (h *handlers) ReopenEntry(ctx context.Context, req oapi.ReopenEntryRequestObject) (oapi.ReopenEntryResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	e, err := h.entries.SetDone(ctx, u, req.EntryId, false)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ReopenEntry404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.ReopenEntry409JSONResponse(errorBody(ctx, "not_a_reminder", "")), nil
	case err != nil:
		return nil, err
	}
	return oapi.ReopenEntry200JSONResponse(toEntry(e)), nil
}

// invalidEntry names what is wrong with an entry, so the app can say it in the user's language.
func invalidEntry(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	switch {
	case errors.As(err, &te):
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	case errors.Is(err, domain.ErrRevealAt):
		return errorBody(ctx, "invalid_reveal", err.Error())
	case errors.Is(err, domain.ErrAssignee):
		return errorBody(ctx, "invalid_assignee", err.Error())
	case errors.Is(err, domain.ErrEventTime):
		return errorBody(ctx, "invalid_event_time", err.Error())
	}
	return errorBody(ctx, "invalid_entry", err.Error())
}

func toEntryFields(b oapi.EntryFields) domain.EntryFields {
	f := domain.EntryFields{
		Visibility: domain.Visibility(b.Visibility), RevealAt: b.RevealAt, Text: b.Text, DueAt: b.DueAt,
		StartsAt: b.StartsAt, EndsAt: b.EndsAt, AllDay: b.AllDay != nil && *b.AllDay, AlertMinutes: b.AlertMinutes, AssigneeID: b.AssigneeId,
	}
	if b.Repeat != nil {
		f.Repeat = domain.Repeat(*b.Repeat)
	}
	return f
}

func toEntry(e domain.Entry) oapi.Entry {
	out := oapi.Entry{
		Id: e.ID, Kind: oapi.EntryKind(e.Kind), AuthorId: e.AuthorID, Visibility: oapi.Visibility(e.Visibility),
		RevealAt: e.RevealAt, Sealed: e.Sealed, DueAt: e.DueAt, StartsAt: e.StartsAt, EndsAt: e.EndsAt, AllDay: e.AllDay,
		AlertMinutes: e.AlertMinutes, Repeat: oapi.Repeat(e.Repeat), AssigneeId: e.AssigneeID,
		DoneAt: e.DoneAt, DoneBy: e.DoneBy, CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
	if !e.Sealed {
		out.Text = &e.Text
	}
	return out
}

func (h *handlers) GetCalendarFeed(ctx context.Context, req oapi.GetCalendarFeedRequestObject) (oapi.GetCalendarFeedResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	active, err := h.entries.HasCalendarFeed(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetCalendarFeed404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.GetCalendarFeed200JSONResponse{Active: active}, nil
}

func (h *handlers) CreateCalendarFeed(ctx context.Context, req oapi.CreateCalendarFeedRequestObject) (oapi.CreateCalendarFeedResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	token, err := h.entries.CreateCalendarFeed(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.CreateCalendarFeed404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.CreateCalendarFeed201JSONResponse{Path: calendarFeedPath + token}, nil
}

func (h *handlers) DeleteCalendarFeed(ctx context.Context, req oapi.DeleteCalendarFeedRequestObject) (oapi.DeleteCalendarFeedResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.entries.DeleteCalendarFeed(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.DeleteCalendarFeed404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.DeleteCalendarFeed204Response{}, nil
}

func (h *handlers) GetCalendar(ctx context.Context, req oapi.GetCalendarRequestObject) (oapi.GetCalendarResponseObject, error) {
	es, loc, err := h.entries.CalendarFeed(ctx, req.Token)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.GetCalendar404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	body := icalendar(es, loc, time.Now())
	return oapi.GetCalendar200TextcalendarResponse{Body: bytes.NewReader(body), ContentLength: int64(len(body))}, nil
}
