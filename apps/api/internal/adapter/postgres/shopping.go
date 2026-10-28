package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) ShoppingItems(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.ShoppingItem, error) {
	rows, err := s.q.ListShoppingItems(ctx, sqlcgen.ListShoppingItemsParams{
		HouseholdID: householdID, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.ShoppingItem, len(rows))
	for i, r := range rows {
		out[i] = toShoppingItem(r)
	}
	return out, nil
}

func (s *Store) AddShoppingItems(ctx context.Context, items []domain.ShoppingItem) ([]domain.ShoppingItem, error) {
	out := make([]domain.ShoppingItem, len(items))
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		for i, it := range items {
			var recipe *string
			if it.Recipe != "" {
				recipe = &it.Recipe
			}
			row, err := q.AddShoppingItem(ctx, sqlcgen.AddShoppingItemParams{HouseholdID: it.HouseholdID, Text: it.Text, Recipe: recipe, AddedBy: it.AddedBy})
			if err != nil {
				return err
			}
			out[i] = toShoppingItem(row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SetShoppingItemDone(ctx context.Context, householdID, id uuid.UUID, done bool) (domain.ShoppingItem, error) {
	row, err := s.q.SetShoppingItemDone(ctx, sqlcgen.SetShoppingItemDoneParams{Done: done, ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ShoppingItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ShoppingItem{}, err
	}
	return toShoppingItem(row), nil
}

func (s *Store) DeleteShoppingItem(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteShoppingItem(ctx, sqlcgen.DeleteShoppingItemParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ClearDoneShoppingItems(ctx context.Context, householdID uuid.UUID) error {
	return s.q.ClearDoneShoppingItems(ctx, householdID)
}

func toShoppingItem(r sqlcgen.ShoppingItem) domain.ShoppingItem {
	it := domain.ShoppingItem{ID: r.ID, HouseholdID: r.HouseholdID, Text: r.Text, AddedBy: r.AddedBy, DoneAt: r.DoneAt, Created: r.CreatedAt}
	if r.Recipe != nil {
		it.Recipe = *r.Recipe
	}
	return it
}
