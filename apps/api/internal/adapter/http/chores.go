package http

import (
	"context"
	"errors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListChores(ctx context.Context, req oapi.ListChoresRequestObject) (oapi.ListChoresResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := h.chores.List(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListChores404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListChores200JSONResponse, len(cs))
	for i, c := range cs {
		out[i] = toChore(c)
	}
	return out, nil
}

func (h *handlers) CreateChore(ctx context.Context, req oapi.CreateChoreRequestObject) (oapi.CreateChoreResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.chores.Create(ctx, u, req.HouseholdId, toChoreFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateChore404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateChore400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidChore(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateChore201JSONResponse(toChore(c)), nil
}

func (h *handlers) UpdateChore(ctx context.Context, req oapi.UpdateChoreRequestObject) (oapi.UpdateChoreResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.chores.Update(ctx, u, req.HouseholdId, req.ChoreId, toChoreFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateChore404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateChore400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidChore(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateChore200JSONResponse(toChore(c)), nil
}

func (h *handlers) DeleteChore(ctx context.Context, req oapi.DeleteChoreRequestObject) (oapi.DeleteChoreResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.chores.Delete(ctx, u, req.HouseholdId, req.ChoreId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteChore404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteChore204Response{}, nil
}

func (h *handlers) CompleteChore(ctx context.Context, req oapi.CompleteChoreRequestObject) (oapi.CompleteChoreResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	d, err := h.chores.Complete(ctx, u, req.HouseholdId, req.ChoreId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CompleteChore404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.CompleteChore409JSONResponse(errorBody(ctx, "chore_done", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.CompleteChore201JSONResponse(toChoreCompletion(d)), nil
}

func (h *handlers) ListChoreCompletions(ctx context.Context, req oapi.ListChoreCompletionsRequestObject) (oapi.ListChoreCompletionsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	ds, err := h.chores.History(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ListChoreCompletions404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := make(oapi.ListChoreCompletions200JSONResponse, len(ds))
	for i, d := range ds {
		out[i] = toChoreCompletion(d)
	}
	return out, nil
}

func (h *handlers) UndoChoreCompletion(ctx context.Context, req oapi.UndoChoreCompletionRequestObject) (oapi.UndoChoreCompletionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.chores.Undo(ctx, u, req.HouseholdId, req.CompletionId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UndoChoreCompletion404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrForbidden):
		return oapi.UndoChoreCompletion403JSONResponse(errorBody(ctx, "not_yours", err.Error())), nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.UndoChoreCompletion409JSONResponse(errorBody(ctx, "chore_not_latest", err.Error())), nil
	case err != nil:
		return nil, err
	}
	return oapi.UndoChoreCompletion204Response{}, nil
}

func (h *handlers) ThankChoreCompletion(ctx context.Context, req oapi.ThankChoreCompletionRequestObject) (oapi.ThankChoreCompletionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var line string
	if req.Body != nil && req.Body.Gratitude != nil {
		line = *req.Body.Gratitude
	}
	d, m, err := h.chores.Thank(ctx, u, req.HouseholdId, req.CompletionId, line)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.ThankChoreCompletion404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.ThankChoreCompletion400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidChore(ctx, err))}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.ThankChoreCompletion409JSONResponse(errorBody(ctx, "own_chore", err.Error())), nil
	case err != nil:
		return nil, err
	}
	out := oapi.ThankChoreCompletion200JSONResponse{Completion: toChoreCompletion(d)}
	if m != nil {
		g := toMoment(*m)
		out.Gratitude = &g
	}
	return out, nil
}

func (h *handlers) GetChoreFairness(ctx context.Context, req oapi.GetChoreFairnessRequestObject) (oapi.GetChoreFairnessResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	f, err := h.chores.Fairness(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetChoreFairness404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.GetChoreFairness200JSONResponse{Week: toChoreTally(f.Week), Month: toChoreTally(f.Month)}, nil
}

// invalidChore names what is wrong with a chore, so the app can say it in the user's language.
func invalidChore(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	switch {
	case errors.As(err, &te):
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	case errors.Is(err, domain.ErrChoreAssignee):
		return errorBody(ctx, "invalid_chore_assignee", err.Error())
	}
	return errorBody(ctx, "invalid_chore", err.Error())
}

func toChoreFields(b oapi.ChoreFields) domain.ChoreFields {
	f := domain.ChoreFields{Title: b.Title, Points: b.Points, Rotate: b.Rotate != nil && *b.Rotate, AssigneeID: b.AssigneeId}
	if b.Repeat != nil {
		f.Repeat = domain.Repeat(*b.Repeat)
	}
	if b.DueOn != nil {
		f.DueOn = b.DueOn.Time
	}
	return f
}

func toChore(c domain.Chore) oapi.Chore {
	return oapi.Chore{
		Id: c.ID, Title: c.Title, Points: c.Points, Repeat: oapi.Repeat(c.Repeat), Rotate: c.Rotate, AssigneeId: c.AssigneeID,
		DueOn: openapi_types.Date{Time: c.DueOn}, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
	}
}

func toChoreCompletion(d domain.ChoreCompletion) oapi.ChoreCompletion {
	return oapi.ChoreCompletion{
		Id: d.ID, ChoreId: d.ChoreID, Title: d.Title, Points: d.Points, UserId: d.UserID,
		Day: openapi_types.Date{Time: d.Day}, DoneAt: d.DoneAt, ThankedBy: append([]openapi_types.UUID{}, d.ThankedBy...),
	}
}

func toChoreTally(t domain.ChoreTally) oapi.ChoreTally {
	out := oapi.ChoreTally{From: openapi_types.Date{Time: t.From}, Members: make([]oapi.ChoreShare, len(t.Members))}
	for i, m := range t.Members {
		out.Members[i] = oapi.ChoreShare{UserId: m.UserID, Points: m.Points, Count: m.Count}
	}
	return out
}
