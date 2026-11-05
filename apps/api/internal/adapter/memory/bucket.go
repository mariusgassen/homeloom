package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) BucketItems(_ context.Context, householdID uuid.UUID, kind domain.BucketKind, limit int) ([]domain.BucketItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.BucketItem{}
	for _, w := range s.bucket {
		if w.HouseholdID == householdID && (kind == "" || w.Kind == kind) {
			out = append(out, w)
		}
	}
	// Open bucket oldest first, then ticked ones, most recently ticked first.
	slices.SortStableFunc(out, func(a, b domain.BucketItem) int {
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

func (s *Store) AddBucketItem(_ context.Context, w domain.BucketItem) (domain.BucketItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.ID, w.Created, w.DoneAt = uuid.New(), time.Now(), nil
	s.bucket = append(s.bucket, w)
	return w, nil
}

func (s *Store) SetBucketItemDone(_ context.Context, householdID, id uuid.UUID, done bool) (domain.BucketItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, w := range s.bucket {
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
		s.bucket[i] = w
		return w, nil
	}
	return domain.BucketItem{}, domain.ErrNotFound
}

func (s *Store) DeleteBucketItem(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.bucket)
	s.bucket = slices.DeleteFunc(s.bucket, func(w domain.BucketItem) bool { return w.ID == id && w.HouseholdID == householdID })
	if len(s.bucket) == n {
		return domain.ErrNotFound
	}
	return nil
}
