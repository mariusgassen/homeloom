package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Wishes(_ context.Context, householdID uuid.UUID, limit int) ([]domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.Wish
	for _, w := range s.wishes {
		if w.HouseholdID == householdID {
			out = append(out, copyWish(w))
		}
	}
	// Newest first, as the database lists them.
	slices.Reverse(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) CreateWish(_ context.Context, w domain.Wish) (domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.ID, w.CreatedAt, w.Votes = uuid.New(), time.Now(), nil
	s.wishes = append(s.wishes, w)
	return copyWish(w), nil
}

func (s *Store) GetWish(_ context.Context, householdID, id uuid.UUID) (domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.wish(householdID, id); i >= 0 {
		return copyWish(s.wishes[i]), nil
	}
	return domain.Wish{}, domain.ErrNotFound
}

func (s *Store) UpdateWish(_ context.Context, w domain.Wish) (domain.Wish, error) {
	return s.changeWish(w.HouseholdID, w.ID, func(c *domain.Wish) { c.WishFields = w.WishFields })
}

func (s *Store) DeleteWish(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.wish(householdID, id)
	if i < 0 {
		return domain.ErrNotFound
	}
	s.wishes = slices.Delete(s.wishes, i, i+1)
	return nil
}

func (s *Store) VoteWish(_ context.Context, householdID, id, userID uuid.UUID, hearts int) (domain.Wish, error) {
	return s.changeWish(householdID, id, func(c *domain.Wish) {
		c.Votes = slices.DeleteFunc(c.Votes, func(v domain.WishVote) bool { return v.UserID == userID })
		if hearts > 0 {
			c.Votes = append(c.Votes, domain.WishVote{UserID: userID, Hearts: hearts})
		}
	})
}

func (s *Store) SetWishGot(_ context.Context, householdID, id uuid.UUID, at *time.Time) (domain.Wish, error) {
	return s.changeWish(householdID, id, func(c *domain.Wish) { c.GotAt = at })
}

func (s *Store) SetWishPhoto(_ context.Context, householdID, id uuid.UUID, at *time.Time) (domain.Wish, error) {
	return s.changeWish(householdID, id, func(c *domain.Wish) { c.PhotoAt = at })
}

func (s *Store) changeWish(householdID, id uuid.UUID, change func(*domain.Wish)) (domain.Wish, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.wish(householdID, id)
	if i < 0 {
		return domain.Wish{}, domain.ErrNotFound
	}
	change(&s.wishes[i])
	return copyWish(s.wishes[i]), nil
}

// wish returns the index of the household's wish, or -1.
func (s *Store) wish(householdID, id uuid.UUID) int {
	return slices.IndexFunc(s.wishes, func(w domain.Wish) bool { return w.ID == id && w.HouseholdID == householdID })
}

func copyWish(w domain.Wish) domain.Wish {
	w.Votes = slices.Clone(w.Votes)
	return w
}
