package memory

import (
	"context"
	"sync"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Photos is an in-memory port.PhotoStore.
type Photos struct {
	mu     sync.Mutex
	photos map[string]domain.Photo
}

func NewPhotos() *Photos { return &Photos{photos: map[string]domain.Photo{}} }

func (s *Photos) PutPhoto(_ context.Context, key string, p domain.Photo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.photos[key] = p
	return nil
}

func (s *Photos) GetPhoto(_ context.Context, key string) (domain.Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.photos[key]
	if !ok {
		return domain.Photo{}, domain.ErrNotFound
	}
	return p, nil
}

func (s *Photos) DeletePhoto(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.photos, key)
	return nil
}

// Len is how many photos it holds.
func (s *Photos) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.photos)
}
