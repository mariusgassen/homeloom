package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveHomeConnection(ctx context.Context, c domain.HomeConnection) error {
	return s.q.UpsertHomeConnection(ctx, sqlcgen.UpsertHomeConnectionParams{UserID: c.UserID, RefreshToken: c.RefreshToken})
}

func (s *Store) HomeConnection(ctx context.Context, userID uuid.UUID) (domain.HomeConnection, error) {
	c, err := s.q.GetHomeConnection(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.HomeConnection{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.HomeConnection{}, err
	}
	return domain.HomeConnection{UserID: c.UserID, RefreshToken: c.RefreshToken}, nil
}

func (s *Store) DeleteHomeConnection(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeleteHomeConnection(ctx, userID)
}
