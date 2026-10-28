package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SavePushSubscription(ctx context.Context, sub domain.PushSubscription) error {
	return s.q.UpsertPushSubscription(ctx, sqlcgen.UpsertPushSubscriptionParams{
		Endpoint: sub.Endpoint, UserID: sub.UserID, P256dh: sub.P256dh, Auth: sub.Auth, Lang: sub.Lang,
	})
}

func (s *Store) PushSubscriptions(ctx context.Context, userIDs []uuid.UUID) ([]domain.PushSubscription, error) {
	rows, err := s.q.ListPushSubscriptions(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PushSubscription, len(rows))
	for i, r := range rows {
		out[i] = domain.PushSubscription{UserID: r.UserID, Endpoint: r.Endpoint, P256dh: r.P256dh, Auth: r.Auth, Lang: r.Lang}
	}
	return out, nil
}

func (s *Store) DeletePushSubscription(ctx context.Context, userID uuid.UUID, endpoint string) error {
	return s.q.DeletePushSubscription(ctx, sqlcgen.DeletePushSubscriptionParams{Endpoint: endpoint, UserID: userID})
}

func (s *Store) DeletePushEndpoint(ctx context.Context, endpoint string) error {
	return s.q.DeletePushEndpoint(ctx, endpoint)
}
