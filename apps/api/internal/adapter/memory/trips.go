package memory

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// tripCopy returns the trip with its own lists, so callers can't change what the store holds.
func tripCopy(t domain.Trip) domain.Trip {
	t.Options = slices.Clone(t.Options)
	for i := range t.Options {
		t.Options[i].Photos = slices.Clone(t.Options[i].Photos)
	}
	t.Packing = slices.Clone(t.Packing)
	t.Votes = slices.Clone(t.Votes)
	return t
}

func (s *Store) trip(householdID, id uuid.UUID) int {
	return slices.IndexFunc(s.trips, func(t domain.Trip) bool { return t.ID == id && t.HouseholdID == householdID })
}

func (s *Store) Trips(_ context.Context, householdID uuid.UUID, limit int) ([]domain.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Trip{}
	for _, t := range s.trips {
		if t.HouseholdID == householdID {
			out = append(out, tripCopy(t))
		}
	}
	// Soonest start first, trips without one last.
	slices.SortStableFunc(out, func(a, b domain.Trip) int {
		switch {
		case a.StartOn == nil && b.StartOn == nil:
			return 0
		case a.StartOn == nil:
			return 1
		case b.StartOn == nil:
			return -1
		}
		return a.StartOn.Compare(*b.StartOn)
	})
	return out[:min(len(out), limit)], nil
}

func (s *Store) Trip(_ context.Context, householdID, id uuid.UUID) (domain.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.trip(householdID, id); i >= 0 {
		return tripCopy(s.trips[i]), nil
	}
	return domain.Trip{}, domain.ErrNotFound
}

func (s *Store) CreateTrip(_ context.Context, t domain.Trip) (domain.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t.ID, t.CreatedAt = uuid.New(), time.Now()
	t.Options, t.Packing, t.Votes = []domain.TripOption{}, []domain.PackingItem{}, []domain.TripVote{}
	s.trips = append(s.trips, t)
	return tripCopy(t), nil
}

func (s *Store) UpdateTrip(_ context.Context, t domain.Trip) (domain.Trip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.trip(t.HouseholdID, t.ID)
	if i < 0 {
		return domain.Trip{}, domain.ErrNotFound
	}
	s.trips[i].TripFields = t.TripFields
	return tripCopy(s.trips[i]), nil
}

func (s *Store) DeleteTrip(_ context.Context, householdID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.trip(householdID, id)
	if i < 0 {
		return domain.ErrNotFound
	}
	s.trips = slices.Delete(s.trips, i, i+1)
	return nil
}

// tripByID finds a trip whatever its household; callers have checked the household already.
func (s *Store) tripByID(id uuid.UUID) *domain.Trip {
	if i := slices.IndexFunc(s.trips, func(t domain.Trip) bool { return t.ID == id }); i >= 0 {
		return &s.trips[i]
	}
	return nil
}

func (s *Store) CreateTripOption(_ context.Context, o domain.TripOption) (domain.TripOption, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tripByID(o.TripID)
	if t == nil {
		return domain.TripOption{}, domain.ErrNotFound
	}
	o.ID, o.CreatedAt, o.Photos = uuid.New(), time.Now(), []domain.TripPhoto{}
	t.Options = append(t.Options, o)
	return o, nil
}

func (s *Store) UpdateTripOption(_ context.Context, tripID uuid.UUID, o domain.TripOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tripByID(tripID); t != nil {
		for i := range t.Options {
			if t.Options[i].ID == o.ID {
				t.Options[i].TripOptionFields = o.TripOptionFields
				return nil
			}
		}
	}
	return domain.ErrNotFound
}

func (s *Store) DeleteTripOption(_ context.Context, tripID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tripByID(tripID)
	if t == nil {
		return domain.ErrNotFound
	}
	i := slices.IndexFunc(t.Options, func(o domain.TripOption) bool { return o.ID == id })
	if i < 0 {
		return domain.ErrNotFound
	}
	t.Options = slices.Delete(slices.Clone(t.Options), i, i+1)
	t.Votes = slices.DeleteFunc(slices.Clone(t.Votes), func(v domain.TripVote) bool { return v.OptionID == id })
	if t.ChosenOptionID != nil && *t.ChosenOptionID == id {
		t.ChosenOptionID = nil
	}
	return nil
}

func (s *Store) SetTripVote(_ context.Context, tripID, userID uuid.UUID, optionID *uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tripByID(tripID)
	if t == nil {
		return domain.ErrNotFound
	}
	t.Votes = slices.DeleteFunc(slices.Clone(t.Votes), func(v domain.TripVote) bool { return v.UserID == userID })
	if optionID != nil {
		t.Votes = append(t.Votes, domain.TripVote{UserID: userID, OptionID: *optionID})
	}
	return nil
}

func (s *Store) CreateTripPhoto(_ context.Context, p domain.TripPhoto) (domain.TripPhoto, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.ID, p.CreatedAt = uuid.New(), time.Now()
	for ti := range s.trips {
		for oi := range s.trips[ti].Options {
			if o := &s.trips[ti].Options[oi]; o.ID == p.OptionID {
				o.Photos = append(o.Photos, p)
				return p, nil
			}
		}
	}
	return domain.TripPhoto{}, domain.ErrNotFound
}

func (s *Store) DeleteTripPhoto(_ context.Context, optionID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ti := range s.trips {
		for oi := range s.trips[ti].Options {
			o := &s.trips[ti].Options[oi]
			if o.ID != optionID {
				continue
			}
			i := slices.IndexFunc(o.Photos, func(p domain.TripPhoto) bool { return p.ID == id })
			if i < 0 {
				return domain.ErrNotFound
			}
			o.Photos = slices.Delete(slices.Clone(o.Photos), i, i+1)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (s *Store) CreatePackingItem(_ context.Context, tripID uuid.UUID, text string) (domain.PackingItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tripByID(tripID)
	if t == nil {
		return domain.PackingItem{}, domain.ErrNotFound
	}
	it := domain.PackingItem{ID: uuid.New(), TripID: tripID, Text: text, CreatedAt: time.Now()}
	t.Packing = append(t.Packing, it)
	return it, nil
}

func (s *Store) UpdatePackingItem(_ context.Context, tripID, id uuid.UUID, f domain.PackingFields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.tripByID(tripID); t != nil {
		for i := range t.Packing {
			if t.Packing[i].ID == id {
				t.Packing[i].Text, t.Packing[i].Done = f.Text, f.Done
				return nil
			}
		}
	}
	return domain.ErrNotFound
}

func (s *Store) DeletePackingItem(_ context.Context, tripID, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tripByID(tripID)
	if t == nil {
		return domain.ErrNotFound
	}
	i := slices.IndexFunc(t.Packing, func(p domain.PackingItem) bool { return p.ID == id })
	if i < 0 {
		return domain.ErrNotFound
	}
	t.Packing = slices.Delete(slices.Clone(t.Packing), i, i+1)
	return nil
}
