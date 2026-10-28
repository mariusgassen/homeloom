package memory

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SavePushSubscription(_ context.Context, sub domain.PushSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushSubscriptions = slices.DeleteFunc(s.pushSubscriptions, func(p domain.PushSubscription) bool { return p.Endpoint == sub.Endpoint })
	s.pushSubscriptions = append(s.pushSubscriptions, sub)
	return nil
}

func (s *Store) PushSubscriptions(_ context.Context, userIDs []uuid.UUID) ([]domain.PushSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.PushSubscription{}
	for _, p := range s.pushSubscriptions {
		if slices.Contains(userIDs, p.UserID) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Store) DeletePushSubscription(_ context.Context, userID uuid.UUID, endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushSubscriptions = slices.DeleteFunc(s.pushSubscriptions, func(p domain.PushSubscription) bool {
		return p.Endpoint == endpoint && p.UserID == userID
	})
	return nil
}

func (s *Store) DeletePushEndpoint(_ context.Context, endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pushSubscriptions = slices.DeleteFunc(s.pushSubscriptions, func(p domain.PushSubscription) bool { return p.Endpoint == endpoint })
	return nil
}
