package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) BucketItems(ctx context.Context, householdID uuid.UUID, kind domain.BucketKind, limit int) ([]domain.BucketItem, error) {
	rows, err := s.q.ListBucket(ctx, sqlcgen.ListBucketParams{
		HouseholdID: householdID, Kind: string(kind), Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.BucketItem, len(rows))
	for i, r := range rows {
		out[i] = toBucketItem(r)
	}
	return out, nil
}

func (s *Store) AddBucketItem(ctx context.Context, w domain.BucketItem) (domain.BucketItem, error) {
	row, err := s.q.AddBucketItem(ctx, sqlcgen.AddBucketItemParams{HouseholdID: w.HouseholdID, Kind: string(w.Kind), Text: w.Text, AddedBy: w.AddedBy})
	if err != nil {
		return domain.BucketItem{}, err
	}
	return toBucketItem(row), nil
}

func (s *Store) SetBucketItemDone(ctx context.Context, householdID, id uuid.UUID, done bool) (domain.BucketItem, error) {
	row, err := s.q.SetBucketItemDone(ctx, sqlcgen.SetBucketItemDoneParams{Done: done, ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.BucketItem{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.BucketItem{}, err
	}
	return toBucketItem(row), nil
}

func (s *Store) DeleteBucketItem(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteBucketItem(ctx, sqlcgen.DeleteBucketItemParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toBucketItem(r sqlcgen.BucketItem) domain.BucketItem {
	return domain.BucketItem{ID: r.ID, HouseholdID: r.HouseholdID, Kind: domain.BucketKind(r.Kind), Text: r.Text, AddedBy: r.AddedBy, DoneAt: r.DoneAt, Created: r.CreatedAt}
}
