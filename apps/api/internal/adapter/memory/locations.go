package memory

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveLocation(_ context.Context, userID uuid.UUID, l domain.MemberLocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.UpdatedAt.IsZero() {
		l.UpdatedAt = time.Now()
	}
	s.locations[userID] = l
	return nil
}

func (s *Store) DeleteLocation(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.locations, userID)
	return nil
}

func (s *Store) Locations(_ context.Context, userIDs []uuid.UUID) (map[uuid.UUID]domain.MemberLocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[uuid.UUID]domain.MemberLocation{}
	for _, id := range userIDs {
		if l, ok := s.locations[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}
