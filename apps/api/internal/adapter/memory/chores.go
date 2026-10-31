package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Chores(_ context.Context, householdID uuid.UUID, limit int) ([]domain.Chore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Chore{}
	for _, c := range s.chores {
		if c.HouseholdID == householdID && c.DoneAt == nil {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.Chore) int { return a.DueOn.Compare(b.DueOn) })
	return out[:min(len(out), limit)], nil
}

func (s *Store) CreateChore(_ context.Context, c domain.Chore) (domain.Chore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.ID, c.CreatedAt, c.DoneAt = uuid.New(), time.Now(), nil
	s.chores = append(s.chores, c)
	return c, nil
}

func (s *Store) GetChore(_ context.Context, householdID, id uuid.UUID) (domain.Chore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.chore(householdID, id); i >= 0 {
		return s.chores[i], nil
	}
	return domain.Chore{}, domain.ErrNotFound
}

func (s *Store) UpdateChore(_ context.Context, c domain.Chore) (domain.Chore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.chore(c.HouseholdID, c.ID)
	if i < 0 {
		return domain.Chore{}, domain.ErrNotFound
	}
	s.chores[i].ChoreFields = c.ChoreFields
	return s.chores[i], nil
}

func (s *Store) DeleteChore(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.chore(householdID, id)
	if i < 0 {
		return domain.ErrNotFound
	}
	s.chores = slices.Delete(s.chores, i, i+1)
	for j, d := range s.choreCompletions {
		if d.ChoreID != nil && *d.ChoreID == id {
			s.choreCompletions[j].ChoreID = nil
		}
	}
	return nil
}

func (s *Store) CompleteChore(_ context.Context, before, after domain.Chore, done domain.ChoreCompletion) (domain.ChoreCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.chore(before.HouseholdID, before.ID)
	if i < 0 {
		return domain.ChoreCompletion{}, domain.ErrNotFound
	}
	c := s.chores[i]
	if c.DoneAt != nil || !c.DueOn.Equal(before.DueOn) || !sameID(c.AssigneeID, before.AssigneeID) {
		return domain.ChoreCompletion{}, domain.ErrConflict
	}
	c.DueOn, c.AssigneeID, c.DoneAt = after.DueOn, after.AssigneeID, after.DoneAt
	s.chores[i] = c
	done.ID, done.DoneAt, done.ThankedBy = uuid.New(), time.Now(), nil
	s.choreCompletions = append(s.choreCompletions, done)
	return done, nil
}

func (s *Store) LastChoreCompletion(_ context.Context, householdID, choreID uuid.UUID) (domain.ChoreCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.lastCompletion(householdID, choreID); i >= 0 {
		return s.completion(i), nil
	}
	return domain.ChoreCompletion{}, domain.ErrNotFound
}

func (s *Store) GetChoreCompletion(_ context.Context, householdID, id uuid.UUID) (domain.ChoreCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.choreCompletions {
		if d.ID == id && d.HouseholdID == householdID {
			return s.completion(i), nil
		}
	}
	return domain.ChoreCompletion{}, domain.ErrNotFound
}

func (s *Store) UndoChoreCompletion(_ context.Context, done domain.ChoreCompletion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := slices.IndexFunc(s.choreCompletions, func(d domain.ChoreCompletion) bool { return d.ID == done.ID && d.HouseholdID == done.HouseholdID })
	if k < 0 {
		return domain.ErrNotFound
	}
	if d := s.choreCompletions[k]; d.ChoreID != nil {
		if s.lastCompletion(d.HouseholdID, *d.ChoreID) != k {
			return domain.ErrConflict
		}
		if i := s.chore(d.HouseholdID, *d.ChoreID); i >= 0 {
			s.chores[i].DueOn, s.chores[i].AssigneeID, s.chores[i].DoneAt = d.PrevDueOn, d.PrevAssigneeID, nil
		}
	}
	s.choreCompletions = slices.Delete(s.choreCompletions, k, k+1)
	return nil
}

func (s *Store) ChoreCompletions(_ context.Context, householdID uuid.UUID, since time.Time, limit int) ([]domain.ChoreCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.ChoreCompletion{}
	for i := len(s.choreCompletions) - 1; i >= 0 && len(out) < limit; i-- {
		if d := s.choreCompletions[i]; d.HouseholdID == householdID && !d.Day.Before(since) {
			out = append(out, s.completion(i))
		}
	}
	return out, nil
}

func (s *Store) ThankChoreCompletion(_ context.Context, householdID, id, userID uuid.UUID) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.choreCompletions {
		if d.ID != id || d.HouseholdID != householdID {
			continue
		}
		if slices.Contains(d.ThankedBy, userID) {
			return false, nil
		}
		s.choreCompletions[i].ThankedBy = append(slices.Clone(d.ThankedBy), userID)
		return true, nil
	}
	return false, domain.ErrNotFound
}

func (s *Store) chore(householdID, id uuid.UUID) int {
	return slices.IndexFunc(s.chores, func(c domain.Chore) bool { return c.ID == id && c.HouseholdID == householdID })
}

// lastCompletion is the index of the chore's latest completion, or -1; completions are kept in
// the order they were made.
func (s *Store) lastCompletion(householdID, choreID uuid.UUID) int {
	for i := len(s.choreCompletions) - 1; i >= 0; i-- {
		if d := s.choreCompletions[i]; d.HouseholdID == householdID && d.ChoreID != nil && *d.ChoreID == choreID {
			return i
		}
	}
	return -1
}

func (s *Store) completion(i int) domain.ChoreCompletion {
	d := s.choreCompletions[i]
	d.ThankedBy = slices.Clone(d.ThankedBy)
	return d
}

func sameID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
