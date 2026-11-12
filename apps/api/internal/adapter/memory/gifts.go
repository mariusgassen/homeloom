package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) SetBirthday(_ context.Context, userID uuid.UUID, b *domain.Birthday) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.birthdays == nil {
		s.birthdays = map[uuid.UUID]domain.Birthday{}
	}
	if b == nil {
		delete(s.birthdays, userID)
		return nil
	}
	s.birthdays[userID] = *b
	return nil
}

func (s *Store) Birthdays(_ context.Context, householdID uuid.UUID) (map[uuid.UUID]domain.Birthday, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[uuid.UUID]domain.Birthday{}
	for _, m := range s.memberships {
		if b, ok := s.birthdays[m.userID]; ok && m.householdID == householdID {
			out[m.userID] = b
		}
	}
	return out, nil
}

func (s *Store) GiftIdeas(_ context.Context, householdID, authorID uuid.UUID, limit int) ([]domain.GiftIdea, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.GiftIdea{}
	for _, i := range s.giftIdeas {
		if i.HouseholdID == householdID && i.AuthorID == authorID {
			out = append(out, i)
		}
	}
	// Open ideas first, newest first.
	slices.SortStableFunc(out, func(a, b domain.GiftIdea) int {
		if (a.GivenAt == nil) != (b.GivenAt == nil) {
			if a.GivenAt == nil {
				return -1
			}
			return 1
		}
		return b.Created.Compare(a.Created)
	})
	return out[:min(len(out), limit)], nil
}

func (s *Store) AddGiftIdea(_ context.Context, idea domain.GiftIdea) (domain.GiftIdea, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idea.ID, idea.Created, idea.GivenAt = uuid.New(), time.Now(), nil
	s.giftIdeas = append(s.giftIdeas, idea)
	return idea, nil
}

func (s *Store) giftIdea(householdID, authorID, id uuid.UUID) int {
	return slices.IndexFunc(s.giftIdeas, func(i domain.GiftIdea) bool {
		return i.ID == id && i.HouseholdID == householdID && i.AuthorID == authorID
	})
}

func (s *Store) SetGiftIdeaGiven(_ context.Context, householdID, authorID, id uuid.UUID, given bool) (domain.GiftIdea, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.giftIdea(householdID, authorID, id)
	if i < 0 {
		return domain.GiftIdea{}, domain.ErrNotFound
	}
	switch {
	case !given:
		s.giftIdeas[i].GivenAt = nil
	case s.giftIdeas[i].GivenAt == nil:
		now := time.Now()
		s.giftIdeas[i].GivenAt = &now
	}
	return s.giftIdeas[i], nil
}

func (s *Store) DeleteGiftIdea(_ context.Context, householdID, authorID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.giftIdea(householdID, authorID, id)
	if i < 0 {
		return domain.ErrNotFound
	}
	s.giftIdeas = slices.Delete(s.giftIdeas, i, i+1)
	return nil
}

func (s *Store) Occasions(_ context.Context, householdID uuid.UUID) ([]domain.Occasion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Occasion{}
	for _, o := range s.occasions {
		if o.HouseholdID == householdID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (s *Store) AddOccasion(_ context.Context, o domain.Occasion) (domain.Occasion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o.ID, o.Created = uuid.New(), time.Now()
	s.occasions = append(s.occasions, o)
	return o, nil
}

func (s *Store) DeleteOccasion(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.occasions, func(o domain.Occasion) bool { return o.ID == id && o.HouseholdID == householdID })
	if i < 0 {
		return domain.ErrNotFound
	}
	s.occasions = slices.Delete(s.occasions, i, i+1)
	for j := range s.giftIdeas {
		if s.giftIdeas[j].OccasionID != nil && *s.giftIdeas[j].OccasionID == id {
			s.giftIdeas[j].OccasionID = nil
		}
	}
	return nil
}
