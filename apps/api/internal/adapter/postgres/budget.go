package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Expenses(ctx context.Context, householdID uuid.UUID, from, to time.Time, limit int) ([]domain.Expense, error) {
	rows, err := s.q.ListExpenses(ctx, sqlcgen.ListExpensesParams{
		HouseholdID: householdID, FromDay: from, ToDay: to, Lim: int32(min(limit, domain.MaxExpensesPerMonth)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	return withShares(ctx, s.q, rows)
}

func (s *Store) LatestExpenseDay(ctx context.Context, householdID uuid.UUID, before time.Time) (time.Time, error) {
	day, err := s.q.LatestExpenseDay(ctx, sqlcgen.LatestExpenseDayParams{HouseholdID: householdID, BeforeDay: before})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, domain.ErrNotFound
	}
	return day, err
}

func (s *Store) Expense(ctx context.Context, householdID, id uuid.UUID) (domain.Expense, error) {
	row, err := s.q.GetExpense(ctx, sqlcgen.GetExpenseParams{ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Expense{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Expense{}, err
	}
	es, err := withShares(ctx, s.q, []sqlcgen.Expense{row})
	if err != nil {
		return domain.Expense{}, err
	}
	return es[0], nil
}

func (s *Store) CreateExpense(ctx context.Context, e domain.Expense) (domain.Expense, error) {
	var out domain.Expense
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		row, err := q.CreateExpense(ctx, sqlcgen.CreateExpenseParams{
			HouseholdID: e.HouseholdID, Kind: string(e.Kind), Cents: e.Cents, Category: string(e.Category),
			Note: nullable(e.Note), PaidBy: e.PaidBy, Day: e.Day, AddedBy: e.AddedBy,
		})
		if err != nil {
			return err
		}
		out, err = saveShares(ctx, q, row, e.Shares)
		return err
	})
	return out, err
}

func (s *Store) UpdateExpense(ctx context.Context, e domain.Expense) (domain.Expense, error) {
	var out domain.Expense
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		row, err := q.UpdateExpense(ctx, sqlcgen.UpdateExpenseParams{
			Cents: e.Cents, Category: string(e.Category), Note: nullable(e.Note), PaidBy: e.PaidBy, Day: e.Day,
			ID: e.ID, HouseholdID: e.HouseholdID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := q.ClearExpenseShares(ctx, row.ID); err != nil {
			return err
		}
		out, err = saveShares(ctx, q, row, e.Shares)
		return err
	})
	return out, err
}

func (s *Store) DeleteExpense(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteExpense(ctx, sqlcgen.DeleteExpenseParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) Nets(ctx context.Context, householdID uuid.UUID) ([]domain.Net, error) {
	rows, err := s.q.BudgetNets(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Net, len(rows))
	for i, r := range rows {
		out[i] = domain.Net{UserID: r.UserID, Cents: r.Cents}
	}
	return out, nil
}

func saveShares(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.Expense, shares []domain.Share) (domain.Expense, error) {
	for _, sh := range shares {
		if err := q.AddExpenseShare(ctx, sqlcgen.AddExpenseShareParams{ExpenseID: row.ID, UserID: sh.UserID, Cents: sh.Cents}); err != nil {
			return domain.Expense{}, err
		}
	}
	e := toExpense(row)
	e.Shares = shares
	return e, nil
}

// withShares reads the shares of all rows in one query.
func withShares(ctx context.Context, q *sqlcgen.Queries, rows []sqlcgen.Expense) ([]domain.Expense, error) {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	shares, err := q.ExpenseShares(ctx, ids)
	if err != nil {
		return nil, err
	}
	byExpense := make(map[uuid.UUID][]domain.Share, len(rows))
	for _, sh := range shares {
		byExpense[sh.ExpenseID] = append(byExpense[sh.ExpenseID], domain.Share{UserID: sh.UserID, Cents: sh.Cents})
	}
	out := make([]domain.Expense, len(rows))
	for i, r := range rows {
		out[i] = toExpense(r)
		out[i].Shares = byExpense[r.ID]
	}
	return out, nil
}

func toExpense(r sqlcgen.Expense) domain.Expense {
	e := domain.Expense{
		ID: r.ID, HouseholdID: r.HouseholdID, Kind: domain.ExpenseKind(r.Kind), Cents: r.Cents,
		Category: domain.ExpenseCategory(r.Category), PaidBy: r.PaidBy, Day: r.Day, AddedBy: r.AddedBy,
		Created: r.CreatedAt, Updated: r.UpdatedAt,
	}
	if r.Note != nil {
		e.Note = *r.Note
	}
	return e
}
