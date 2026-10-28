package memory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SaveHomeConnection(_ context.Context, c domain.HomeConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.homeConnections == nil {
		s.homeConnections = map[uuid.UUID]domain.HomeConnection{}
	}
	s.homeConnections[c.UserID] = c
	return nil
}

func (s *Store) HomeConnection(_ context.Context, userID uuid.UUID) (domain.HomeConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.homeConnections[userID]
	if !ok {
		return domain.HomeConnection{}, domain.ErrNotFound
	}
	return c, nil
}

func (s *Store) DeleteHomeConnection(_ context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.homeConnections, userID)
	return nil
}
