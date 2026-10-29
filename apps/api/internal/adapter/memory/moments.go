package memory

import (
	"context"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateMoment(_ context.Context, m domain.Moment) (domain.Moment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	m.ID, m.CreatedAt, m.UpdatedAt, m.PhotoAt = uuid.New(), now, now, nil
	s.moments = append(s.moments, m)
	return m, nil
}

func (s *Store) SaveGratitude(ctx context.Context, m domain.Moment) (domain.Moment, error) {
	s.mu.Lock()
	for i, old := range s.moments {
		if old.Kind == domain.KindGratitude && old.HouseholdID == m.HouseholdID && old.AuthorID == m.AuthorID && old.Day.Equal(m.Day) {
			old.Visibility, old.Text, old.UpdatedAt = m.Visibility, m.Text, time.Now()
			s.moments[i] = old
			s.mu.Unlock()
			return old, nil
		}
	}
	s.mu.Unlock()
	m.Kind, m.RevealAt = domain.KindGratitude, nil
	return s.CreateMoment(ctx, m)
}

func (s *Store) GetMoment(_ context.Context, id uuid.UUID) (domain.Moment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.moments {
		if m.ID == id {
			return m, nil
		}
	}
	return domain.Moment{}, domain.ErrNotFound
}

func (s *Store) UpdateMoment(_ context.Context, m domain.Moment) (domain.Moment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, old := range s.moments {
		if old.ID == m.ID {
			old.MomentFields, old.UpdatedAt = m.MomentFields, time.Now()
			s.moments[i] = old
			return old, nil
		}
	}
	return domain.Moment{}, domain.ErrNotFound
}

func (s *Store) DeleteMoment(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.moments = slices.DeleteFunc(s.moments, func(m domain.Moment) bool { return m.ID == id })
	delete(s.momentPhotos, id)
	return nil
}

// seen returns the household's moments the viewer can see that match keep, newest day first.
func (s *Store) seen(householdID, viewer uuid.UUID, keep func(domain.Moment) bool) []domain.Moment {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Moment{}
	for _, m := range s.moments {
		if m.HouseholdID == householdID && (m.AuthorID == viewer || m.Visibility != domain.VisibilityPrivate) && keep(m) {
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(a, b domain.Moment) int {
		if c := b.Day.Compare(a.Day); c != 0 {
			return c
		}
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return out
}

func (s *Store) ListMoments(_ context.Context, householdID, viewer uuid.UUID, from, to time.Time, limit int) ([]domain.Moment, error) {
	out := s.seen(householdID, viewer, func(m domain.Moment) bool { return !m.Day.Before(from) && m.Day.Before(to) })
	return out[:min(len(out), limit)], nil
}

func (s *Store) LatestMomentDay(_ context.Context, householdID, viewer uuid.UUID, before time.Time) (time.Time, error) {
	out := s.seen(householdID, viewer, func(m domain.Moment) bool { return m.Day.Before(before) })
	if len(out) == 0 {
		return time.Time{}, domain.ErrNotFound
	}
	return out[0].Day, nil
}

func (s *Store) MomentsOnDay(_ context.Context, householdID, viewer uuid.UUID, today time.Time, limit int) ([]domain.Moment, error) {
	out := s.seen(householdID, viewer, func(m domain.Moment) bool {
		return m.Day.Month() == today.Month() && m.Day.Day() == today.Day() && m.Day.Before(today)
	})
	return out[:min(len(out), limit)], nil
}

func (s *Store) RandomGratitude(_ context.Context, householdID, viewer uuid.UUID, before time.Time) (domain.Moment, error) {
	out := s.seen(householdID, viewer, func(m domain.Moment) bool { return m.Kind == domain.KindGratitude && m.Day.Before(before) })
	if len(out) == 0 {
		return domain.Moment{}, domain.ErrNotFound
	}
	return out[rand.IntN(len(out))], nil //nolint:gosec // picking a memory, not a secret
}

func (s *Store) SetMomentPhoto(_ context.Context, id uuid.UUID, p domain.Photo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.moments {
		if m.ID == id {
			now := time.Now()
			m.PhotoAt, m.UpdatedAt = &now, now
			s.moments[i] = m
			s.momentPhotos[id] = p
			return nil
		}
	}
	return domain.ErrNotFound
}

func (s *Store) MomentPhoto(_ context.Context, id uuid.UUID) (domain.Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.momentPhotos[id]
	if !ok {
		return domain.Photo{}, domain.ErrNotFound
	}
	return p, nil
}

func (s *Store) DeleteMomentPhoto(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.momentPhotos, id)
	for i, m := range s.moments {
		if m.ID == id {
			m.PhotoAt, m.UpdatedAt = nil, time.Now()
			s.moments[i] = m
		}
	}
	return nil
}

func (s *Store) HouseholdIDs(context.Context) ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]uuid.UUID, 0, len(s.households))
	for id := range s.households {
		out = append(out, id)
	}
	return out, nil
}

func (s *Store) ClaimJobRun(_ context.Context, job string, day time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := job + "/" + day.Format(time.DateOnly)
	if s.jobRuns[key] {
		return false, nil
	}
	s.jobRuns[key] = true
	return true, nil
}
