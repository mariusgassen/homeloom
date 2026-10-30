package http

import (
	"context"
	"errors"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (h *handlers) ListExpenses(ctx context.Context, req oapi.ListExpensesRequestObject) (oapi.ListExpensesResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var month time.Time
	if req.Params.Month != nil {
		if month, err = time.Parse(monthLayout, *req.Params.Month); err != nil {
			return oapi.ListExpenses400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(errorBody(ctx, "invalid", err.Error()))}, nil
		}
	}
	em, err := h.budget.Month(ctx, u, req.HouseholdId, month)
	if errors.Is(err, domain.ErrNotFound) {
		return oapi.ListExpenses404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	}
	if err != nil {
		return nil, err
	}
	out := oapi.ListExpenses200JSONResponse{Month: em.Month.Format(monthLayout), Expenses: make([]oapi.Expense, len(em.Expenses))}
	for i, e := range em.Expenses {
		out.Expenses[i] = toExpense(e)
	}
	if !em.Earlier.IsZero() {
		e := em.Earlier.Format(monthLayout)
		out.Earlier = &e
	}
	return out, nil
}

func (h *handlers) CreateExpense(ctx context.Context, req oapi.CreateExpenseRequestObject) (oapi.CreateExpenseResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	e, err := h.budget.Add(ctx, u, req.HouseholdId, toExpenseFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.CreateExpense404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.CreateExpense400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidExpense(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.CreateExpense201JSONResponse(toExpense(e)), nil
}

func (h *handlers) UpdateExpense(ctx context.Context, req oapi.UpdateExpenseRequestObject) (oapi.UpdateExpenseResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	e, err := h.budget.Update(ctx, u, req.HouseholdId, req.ExpenseId, toExpenseFields(*req.Body))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.UpdateExpense404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrSettlement):
		return oapi.UpdateExpense409JSONResponse(errorBody(ctx, "settlement_fixed", err.Error())), nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.UpdateExpense400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidExpense(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.UpdateExpense200JSONResponse(toExpense(e)), nil
}

func (h *handlers) DeleteExpense(ctx context.Context, req oapi.DeleteExpenseRequestObject) (oapi.DeleteExpenseResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	err = h.budget.Delete(ctx, u, req.HouseholdId, req.ExpenseId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.DeleteExpense404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.DeleteExpense204Response{}, nil
}

func (h *handlers) GetBalance(ctx context.Context, req oapi.GetBalanceRequestObject) (oapi.GetBalanceResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b, err := h.budget.Balance(ctx, u, req.HouseholdId)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.GetBalance404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case err != nil:
		return nil, err
	}
	out := oapi.GetBalance200JSONResponse{Members: make([]oapi.MemberBalance, len(b.Members)), Transfers: make([]oapi.Transfer, len(b.Transfers))}
	for i, n := range b.Members {
		out.Members[i] = oapi.MemberBalance{UserId: n.UserID, Cents: n.Cents}
	}
	for i, t := range b.Transfers {
		out.Transfers[i] = oapi.Transfer{From: t.From, To: t.To, Cents: t.Cents}
	}
	return out, nil
}

func (h *handlers) SettleUp(ctx context.Context, req oapi.SettleUpRequestObject) (oapi.SettleUpResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	b := req.Body
	var day time.Time
	if b.Day != nil {
		day = b.Day.Time
	}
	e, err := h.budget.Settle(ctx, u, req.HouseholdId, b.From, b.To, b.Cents, day)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return oapi.SettleUp404JSONResponse{NotFoundJSONResponse: notFound(ctx)}, nil
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SettleUp400JSONResponse{BadRequestJSONResponse: oapi.BadRequestJSONResponse(invalidExpense(ctx, err))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SettleUp201JSONResponse(toExpense(e)), nil
}

func invalidExpense(ctx context.Context, err error) oapi.Error {
	var te domain.TextError
	switch {
	case errors.Is(err, domain.ErrAmount):
		return errorBody(ctx, "invalid_amount", err.Error(), "max", domain.MaxExpenseCents/100)
	case errors.Is(err, domain.ErrShares):
		return errorBody(ctx, "invalid_shares", err.Error())
	case errors.Is(err, domain.ErrPayer):
		return errorBody(ctx, "invalid_payer", err.Error())
	case errors.As(err, &te):
		return errorBody(ctx, "invalid_text", err.Error(), "max", te.Max)
	}
	return errorBody(ctx, "invalid", err.Error())
}

func toExpenseFields(b oapi.ExpenseFields) domain.ExpenseFields {
	f := domain.ExpenseFields{Cents: b.Cents, Category: domain.ExpenseCategory(b.Category), PaidBy: b.PaidBy}
	if b.Note != nil {
		f.Note = *b.Note
	}
	if b.Day != nil {
		f.Day = b.Day.Time
	}
	if b.Shares != nil {
		for _, sh := range *b.Shares {
			f.Shares = append(f.Shares, domain.Share{UserID: sh.UserId, Cents: sh.Cents})
		}
	}
	return f
}

func toExpense(e domain.Expense) oapi.Expense {
	out := oapi.Expense{
		Id: e.ID, Kind: oapi.ExpenseKind(e.Kind), Cents: e.Cents, Category: oapi.ExpenseCategory(e.Category),
		Note: optional(e.Note), PaidBy: e.PaidBy, Day: openapi_types.Date{Time: e.Day}, AddedBy: e.AddedBy,
		Shares: make([]oapi.ExpenseShare, len(e.Shares)), CreatedAt: e.Created, UpdatedAt: e.Updated,
	}
	for i, sh := range e.Shares {
		out.Shares[i] = oapi.ExpenseShare{UserId: sh.UserID, Cents: sh.Cents}
	}
	return out
}
