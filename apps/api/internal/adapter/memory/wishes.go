package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Wishes(_ context.Context, householdID uuid.UUID, kind domain.WishKind, limit int) ([]domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Wish{}
	for _, w := range s.wishes {
		if w.HouseholdID == householdID && (kind == "" || w.Kind == kind) {
			out = append(out, w)
		}
	}
	// Open wishes oldest first, then ticked ones, most recently ticked first.
	slices.SortStableFunc(out, func(a, b domain.Wish) int {
		switch {
		case (a.DoneAt == nil) != (b.DoneAt == nil):
			if a.DoneAt == nil {
				return -1
			}
			return 1
		case a.DoneAt != nil:
			return b.DoneAt.Compare(*a.DoneAt)
		}
		return a.Created.Compare(b.Created)
	})
	return out[:min(len(out), limit)], nil
}

func (s *Store) AddWish(_ context.Context, w domain.Wish) (domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.ID, w.Created, w.DoneAt = uuid.New(), time.Now(), nil
	s.wishes = append(s.wishes, w)
	return w, nil
}

func (s *Store) SetWishDone(_ context.Context, householdID, id uuid.UUID, done bool) (domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.wishes {
		if w.ID != id || w.HouseholdID != householdID {
			continue
		}
		switch {
		case !done:
			w.DoneAt = nil
		case w.DoneAt == nil:
			now := time.Now()
			w.DoneAt = &now
		}
		s.wishes[i] = w
		return w, nil
	}
	return domain.Wish{}, domain.ErrNotFound
}

func (s *Store) DeleteWish(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.wishes)
	s.wishes = slices.DeleteFunc(s.wishes, func(w domain.Wish) bool { return w.ID == id && w.HouseholdID == householdID })
	if len(s.wishes) == n {
		return domain.ErrNotFound
	}
	return nil
}
