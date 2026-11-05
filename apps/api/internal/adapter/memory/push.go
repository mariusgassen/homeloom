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

func (s *Store) MutedPushTopics(_ context.Context, userID uuid.UUID) ([]domain.PushTopic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.PushTopic{}
	for _, m := range s.mutedPushTopics {
		if m.userID == userID {
			out = append(out, m.topic)
		}
	}
	return out, nil
}

func (s *Store) UsersMutingPushTopic(_ context.Context, userIDs []uuid.UUID, topic domain.PushTopic) ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []uuid.UUID{}
	for _, m := range s.mutedPushTopics {
		if m.topic == topic && slices.Contains(userIDs, m.userID) {
			out = append(out, m.userID)
		}
	}
	return out, nil
}

func (s *Store) SetPushTopicMuted(_ context.Context, userID uuid.UUID, topic domain.PushTopic, muted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := mutedPushTopic{userID, topic}
	s.mutedPushTopics = slices.DeleteFunc(s.mutedPushTopics, func(p mutedPushTopic) bool { return p == m })
	if muted {
		s.mutedPushTopics = append(s.mutedPushTopics, m)
	}
	return nil
}
