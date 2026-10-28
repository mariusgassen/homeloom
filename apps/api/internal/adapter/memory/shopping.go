package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) ShoppingItems(_ context.Context, householdID uuid.UUID, limit int) ([]domain.ShoppingItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.ShoppingItem{}
	for _, it := range s.shopping {
		if it.HouseholdID == householdID {
			out = append(out, it)
		}
	}
	// Open items oldest first, then ticked ones, most recently ticked first.
	slices.SortStableFunc(out, func(a, b domain.ShoppingItem) int {
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

func (s *Store) AddShoppingItems(_ context.Context, items []domain.ShoppingItem) ([]domain.ShoppingItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.ShoppingItem, len(items))
	for i, it := range items {
		it.ID, it.Created, it.DoneAt = uuid.New(), time.Now(), nil
		s.shopping = append(s.shopping, it)
		out[i] = it
	}
	return out, nil
}

func (s *Store) SetShoppingItemDone(_ context.Context, householdID, id uuid.UUID, done bool) (domain.ShoppingItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, it := range s.shopping {
		if it.ID != id || it.HouseholdID != householdID {
			continue
		}
		switch {
		case !done:
			it.DoneAt = nil
		case it.DoneAt == nil:
			now := time.Now()
			it.DoneAt = &now
		}
		s.shopping[i] = it
		return it, nil
	}
	return domain.ShoppingItem{}, domain.ErrNotFound
}

func (s *Store) DeleteShoppingItem(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.shopping)
	s.shopping = slices.DeleteFunc(s.shopping, func(it domain.ShoppingItem) bool { return it.ID == id && it.HouseholdID == householdID })
	if len(s.shopping) == n {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ClearDoneShoppingItems(_ context.Context, householdID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shopping = slices.DeleteFunc(s.shopping, func(it domain.ShoppingItem) bool { return it.HouseholdID == householdID && it.DoneAt != nil })
	return nil
}
