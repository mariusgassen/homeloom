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

func (s *Store) MutedPushTopics(ctx context.Context, userID uuid.UUID) ([]domain.PushTopic, error) {
	rows, err := s.q.ListMutedPushTopics(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PushTopic, len(rows))
	for i, r := range rows {
		out[i] = domain.PushTopic(r)
	}
	return out, nil
}

func (s *Store) UsersMutingPushTopic(ctx context.Context, userIDs []uuid.UUID, topic domain.PushTopic) ([]uuid.UUID, error) {
	return s.q.ListUsersMutingPushTopic(ctx, sqlcgen.ListUsersMutingPushTopicParams{Topic: string(topic), UserIds: userIDs})
}

func (s *Store) SetPushTopicMuted(ctx context.Context, userID uuid.UUID, topic domain.PushTopic, muted bool) error {
	if muted {
		return s.q.MutePushTopic(ctx, sqlcgen.MutePushTopicParams{UserID: userID, Topic: string(topic)})
	}
	return s.q.UnmutePushTopic(ctx, sqlcgen.UnmutePushTopicParams{UserID: userID, Topic: string(topic)})
}
