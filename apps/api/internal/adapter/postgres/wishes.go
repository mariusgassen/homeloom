package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Wishes(ctx context.Context, householdID uuid.UUID, kind domain.WishKind, limit int) ([]domain.Wish, error) {
	rows, err := s.q.ListWishes(ctx, sqlcgen.ListWishesParams{
		HouseholdID: householdID, Kind: string(kind), Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Wish, len(rows))
	for i, r := range rows {
		out[i] = toWish(r)
	}
	return out, nil
}

func (s *Store) AddWish(ctx context.Context, w domain.Wish) (domain.Wish, error) {
	row, err := s.q.AddWish(ctx, sqlcgen.AddWishParams{HouseholdID: w.HouseholdID, Kind: string(w.Kind), Text: w.Text, AddedBy: w.AddedBy})
	if err != nil {
		return domain.Wish{}, err
	}
	return toWish(row), nil
}

func (s *Store) SetWishDone(ctx context.Context, householdID, id uuid.UUID, done bool) (domain.Wish, error) {
	row, err := s.q.SetWishDone(ctx, sqlcgen.SetWishDoneParams{Done: done, ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wish{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Wish{}, err
	}
	return toWish(row), nil
}

func (s *Store) DeleteWish(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteWish(ctx, sqlcgen.DeleteWishParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toWish(r sqlcgen.Wish) domain.Wish {
	return domain.Wish{ID: r.ID, HouseholdID: r.HouseholdID, Kind: domain.WishKind(r.Kind), Text: r.Text, AddedBy: r.AddedBy, DoneAt: r.DoneAt, Created: r.CreatedAt}
}
