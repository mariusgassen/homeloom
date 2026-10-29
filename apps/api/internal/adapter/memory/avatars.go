package memory

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SetAvatar(_ context.Context, userID uuid.UUID, p domain.Photo) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	u, err := s.setAvatarAt(userID, &now)
	if err == nil {
		s.avatars[userID] = p
	}
	return u, err
}

func (s *Store) Avatar(_ context.Context, userID uuid.UUID) (domain.Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.avatars[userID]
	if !ok {
		return domain.Photo{}, domain.ErrNotFound
	}
	return p, nil
}

func (s *Store) DeleteAvatar(_ context.Context, userID uuid.UUID) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.avatars, userID)
	return s.setAvatarAt(userID, nil)
}

func (s *Store) SharesHousehold(_ context.Context, a, b uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ma := range s.memberships {
		for _, mb := range s.memberships {
			if ma.userID == a && mb.userID == b && ma.householdID == mb.householdID {
				return true, nil
			}
		}
	}
	return false, nil
}

// setAvatarAt updates the user in both indexes; the caller holds the lock.
func (s *Store) setAvatarAt(userID uuid.UUID, at *time.Time) (domain.User, error) {
	u, ok := s.users[userID]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	u.AvatarAt = at
	s.users[userID] = u
	for sub, v := range s.usersBySub {
		if v.ID == userID {
			s.usersBySub[sub] = u
		}
	}
	return u, nil
}
