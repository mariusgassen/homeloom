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
	return domain.WatchConnection{UserID: c.UserID, Token: c.Token}, nil
}

func (s *Store) DeleteWatchConnection(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeleteWatchConnection(ctx, userID)
}
