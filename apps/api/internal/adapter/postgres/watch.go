package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveWatchConnection(ctx context.Context, c domain.WatchConnection) error {
	return s.q.UpsertWatchConnection(ctx, sqlcgen.UpsertWatchConnectionParams{UserID: c.UserID, Token: c.Token})
}

func (s *Store) WatchConnection(ctx context.Context, userID uuid.UUID) (domain.WatchConnection, error) {
	c, err := s.q.GetWatchConnection(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.WatchConnection{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.WatchConnection{}, err
	}
	return domain.WatchConnection{UserID: c.UserID, Token: c.Token, Hidden: c.HiddenLibraries}, nil
}

func (s *Store) DeleteWatchConnection(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeleteWatchConnection(ctx, userID)
}

func (s *Store) SetWatchLibraryHidden(ctx context.Context, userID uuid.UUID, libraryID string, hidden bool) error {
	n, err := s.q.SetWatchLibraryHidden(ctx, sqlcgen.SetWatchLibraryHiddenParams{UserID: userID, LibraryID: libraryID, Hidden: hidden})
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (s *Store) SaveTonight(ctx context.Context, p domain.TonightPick) error {
	return s.q.UpsertTonight(ctx, sqlcgen.UpsertTonightParams{
		HouseholdID: p.HouseholdID, TitleID: p.TitleID, Kind: string(p.Kind), Name: p.Name,
		Year: int32(p.Year), Url: p.URL, PickedBy: p.PickedBy, PickedAt: p.PickedAt, //nolint:gosec // a year fits
	})
}

func (s *Store) Tonight(ctx context.Context, householdID uuid.UUID) (domain.TonightPick, error) {
	p, err := s.q.GetTonight(ctx, householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TonightPick{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TonightPick{}, err
	}
	return domain.TonightPick{
		HouseholdID: p.HouseholdID, TitleID: p.TitleID, Kind: domain.MediaKind(p.Kind), Name: p.Name,
		Year: int(p.Year), URL: p.Url, PickedBy: p.PickedBy, PickedAt: p.PickedAt,
	}, nil
}

func (s *Store) DeleteTonight(ctx context.Context, householdID uuid.UUID) error {
	return s.q.DeleteTonight(ctx, householdID)
}
