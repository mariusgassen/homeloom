package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) Expenses(_ context.Context, householdID uuid.UUID, from, to time.Time, limit int) ([]domain.Expense, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Expense{}
	for _, e := range s.expenses {
		if e.HouseholdID == householdID && !e.Day.Before(from) && e.Day.Before(to) {
			out = append(out, cloneExpense(e))
		}
	}
	// Newest day first, then newest added.
	slices.SortStableFunc(out, func(a, b domain.Expense) int {
		if c := b.Day.Compare(a.Day); c != 0 {
			return c
		}
		return b.Created.Compare(a.Created)
	})
	return out[:min(len(out), limit)], nil
}

func (s *Store) LatestExpenseDay(_ context.Context, householdID uuid.UUID, before time.Time) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest time.Time
	for _, e := range s.expenses {
		if e.HouseholdID == householdID && e.Day.Before(before) && e.Day.After(latest) {
			latest = e.Day
		}
	}
	if latest.IsZero() {
		return time.Time{}, domain.ErrNotFound
	}
	return latest, nil
}

func (s *Store) Expense(_ context.Context, householdID, id uuid.UUID) (domain.Expense, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.expenses {
		if e.ID == id && e.HouseholdID == householdID {
			return cloneExpense(e), nil
		}
	}
	return domain.Expense{}, domain.ErrNotFound
}

func (s *Store) CreateExpense(_ context.Context, e domain.Expense) (domain.Expense, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ID, e.Created = uuid.New(), time.Now()
	e.Updated = e.Created
	e = cloneExpense(e)
	s.expenses = append(s.expenses, e)
	return cloneExpense(e), nil
}

func (s *Store) UpdateExpense(_ context.Context, e domain.Expense) (domain.Expense, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, old := range s.expenses {
		if old.ID == e.ID && old.HouseholdID == e.HouseholdID {
			e.Kind, e.AddedBy, e.Created, e.Updated = old.Kind, old.AddedBy, old.Created, time.Now()
			s.expenses[i] = cloneExpense(e)
			return cloneExpense(e), nil
		}
	}
	return domain.Expense{}, domain.ErrNotFound
}

func (s *Store) DeleteExpense(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.expenses)
	s.expenses = slices.DeleteFunc(s.expenses, func(e domain.Expense) bool { return e.ID == id && e.HouseholdID == householdID })
	if len(s.expenses) == n {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) Nets(_ context.Context, householdID uuid.UUID) ([]domain.Net, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	nets := map[uuid.UUID]int64{}
	for _, e := range s.expenses {
		if e.HouseholdID != householdID {
			continue
		}
		nets[e.PaidBy] += e.Cents
		for _, sh := range e.Shares {
			nets[sh.UserID] -= sh.Cents
		}
	}
	out := make([]domain.Net, 0, len(nets))
	for u, c := range nets {
		out = append(out, domain.Net{UserID: u, Cents: c})
	}
	return out, nil
}

func cloneExpense(e domain.Expense) domain.Expense {
	e.Shares = slices.Clone(e.Shares)
	return e
}
