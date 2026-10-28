package http

import (
	"context"
	"errors"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) CreateInvite(ctx context.Context, req oapi.CreateInviteRequestObject) (oapi.CreateInviteResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := h.households.Invite(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.CreateInvite404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.CreateInvite201JSONResponse{Code: inv.Code, ExpiresAt: inv.ExpiresAt}, nil
}

func (h *handlers) AcceptInvite(ctx context.Context, req oapi.AcceptInviteRequestObject) (oapi.AcceptInviteResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	m, err := h.households.Join(ctx, u, req.Code)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.AcceptInvite404JSONResponse(errorBody(ctx, "invite_invalid", "")), nil
	}
	if err != nil {
		return nil, err
	}
	return oapi.AcceptInvite200JSONResponse(toHousehold(m)), nil
}

func (h *handlers) ListQuestions(ctx context.Context, req oapi.ListQuestionsRequestObject) (oapi.ListQuestionsResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	qs, err := h.questions.List(ctx, u, req.HouseholdId)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.ListQuestions404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(oapi.ListQuestions200JSONResponse, len(qs))
	for i, q := range qs {
		out[i] = toQuestion(q)
	}
	return out, nil
}

func (h *handlers) AskQuestion(ctx context.Context, req oapi.AskQuestionRequestObject) (oapi.AskQuestionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	q, err := h.questions.Ask(ctx, u, req.HouseholdId, req.Body.Text)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AskQuestion404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AskQuestion400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxQuestionText))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.AskQuestion201JSONResponse(toQuestion(q)), nil
}

func (h *handlers) AnswerQuestion(ctx context.Context, req oapi.AnswerQuestionRequestObject) (oapi.AnswerQuestionResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	q, err := h.questions.Answer(ctx, u, req.QuestionId, req.Body.Text)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.AnswerQuestion404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.AnswerQuestion400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid_text", err.Error(), "max", domain.MaxAnswerText))}, nil
	case errors.Is(err, domain.ErrConflict):
		return oapi.AnswerQuestion409JSONResponse(errorBody(ctx, "already_revealed", "")), nil
	case err != nil:
		return nil, err
	}
	return oapi.AnswerQuestion200JSONResponse(toQuestion(q)), nil
}

func toQuestion(q domain.Question) oapi.Question {
	answers := make([]oapi.Answer, len(q.Answers))
	for i, a := range q.Answers {
		answers[i] = oapi.Answer{UserId: a.UserID, AnsweredAt: a.AnsweredAt}
		if !a.Sealed {
			answers[i].Text = &a.Text
		}
	}
	return oapi.Question{
		Id: q.ID, Text: q.Text, AskedBy: q.AskedBy, CreatedAt: q.CreatedAt,
		Revealed: q.Revealed(), RevealedAt: q.RevealedAt, Answers: answers,
	}
}

func notFound(ctx context.Context) oapi.NotFoundJSONResponse {
	return oapi.NotFoundJSONResponse(errorBody(ctx, "not_found", ""))
}
