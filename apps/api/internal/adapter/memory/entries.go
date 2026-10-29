package memory

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateEntry(_ context.Context, e domain.Entry) (domain.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	e.ID, e.CreatedAt, e.UpdatedAt = uuid.New(), now, now
	s.entries = append(s.entries, e)
	return e, nil
}

func (s *Store) GetEntry(_ context.Context, id uuid.UUID) (domain.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := s.findEntry(id); i >= 0 {
		return s.entries[i], nil
	}
	return domain.Entry{}, domain.ErrNotFound
}

func (s *Store) EntryByICalName(_ context.Context, householdID uuid.UUID, name string) (domain.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.HouseholdID == householdID && e.ICalName != "" && e.ICalName == name {
			return e, nil
		}
	}
	return domain.Entry{}, domain.ErrNotFound
}

func (s *Store) RenameEntry(_ context.Context, id uuid.UUID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.findEntry(id)
	if i < 0 {
		return domain.ErrNotFound
	}
	s.entries[i].ICalName, s.entries[i].UpdatedAt = name, time.Now()
	return nil
}

func (s *Store) ListEntries(_ context.Context, householdID, viewer uuid.UUID, kind domain.EntryKind, since time.Time, limit int) ([]domain.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().AddDate(0, 0, -domain.DoneReminderDays)
	out := []domain.Entry{}
	for _, e := range s.entries {
		if e.HouseholdID != householdID || (e.AuthorID != viewer && e.Visibility == domain.VisibilityPrivate) ||
			(kind != "" && e.Kind != kind) || (e.DoneAt != nil && e.DoneAt.Before(cutoff)) || ended(e, since) {
			continue
		}
		out = append(out, e)
	}
	// Same order as the Postgres query: kind, open before done, by start or due time, newest first.
	slices.SortStableFunc(out, func(a, b domain.Entry) int {
		return cmp.Or(
			cmp.Compare(a.Kind, b.Kind),
			cmpBool(a.DoneAt != nil, b.DoneAt != nil),
			cmpDue(cmp.Or(a.StartsAt, a.DueAt), cmp.Or(b.StartsAt, b.DueAt)),
			b.CreatedAt.Compare(a.CreatedAt),
		)
	})
	return out[:min(limit, len(out))], nil
}

func (s *Store) UpdateEntry(_ context.Context, e domain.Entry) (domain.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.findEntry(e.ID)
	if i < 0 {
		return domain.Entry{}, domain.ErrNotFound
	}
	stored := s.entries[i]
	stored.EntryFields, stored.DoneAt, stored.DoneBy, stored.NotifiedAt = e.EntryFields, e.DoneAt, e.DoneBy, e.NotifiedAt
	stored.UpdatedAt = time.Now()
	s.entries[i] = stored
	return stored, nil
}

func (s *Store) DeleteEntry(_ context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = slices.DeleteFunc(s.entries, func(e domain.Entry) bool { return e.ID == id })
	s.entryPhotos = slices.DeleteFunc(s.entryPhotos, func(p entryPhoto) bool { return p.entryID == id })
	return nil
}

func (s *Store) ClaimDue(_ context.Context, now time.Time, limit int) ([]domain.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []domain.Entry{}
	for i, e := range s.entries {
		if len(out) == limit {
			break
		}
		if e.DoneAt != nil || e.DueAt == nil || e.DueAt.After(now) ||
			(e.NotifiedAt != nil && !e.NotifiedAt.Before(*e.DueAt)) {
			continue
		}
		e.NotifiedAt = &now
		s.entries[i] = e
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) SaveCalendarFeed(_ context.Context, tokenHash []byte, f domain.CalendarFeed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.DeleteFunc(s.calendarFeeds, func(_ string, g domain.CalendarFeed) bool { return g == f })
	s.calendarFeeds[string(tokenHash)] = f
	return nil
}

func (s *Store) CalendarFeed(_ context.Context, tokenHash []byte) (domain.CalendarFeed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.calendarFeeds[string(tokenHash)]
	if !ok {
		return domain.CalendarFeed{}, domain.ErrNotFound
	}
	return f, nil
}

func (s *Store) HasCalendarFeed(_ context.Context, f domain.CalendarFeed) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range s.calendarFeeds {
		if g == f {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) DeleteCalendarFeed(_ context.Context, f domain.CalendarFeed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.DeleteFunc(s.calendarFeeds, func(_ string, g domain.CalendarFeed) bool { return g == f })
	return nil
}

// ended is true for an event that ended before since and doesn't repeat.
func ended(e domain.Entry, since time.Time) bool {
	if e.Kind != domain.KindEvent || e.Repeat != domain.RepeatNever {
		return false
	}
	end := *e.StartsAt
	if e.EndsAt != nil {
		end = *e.EndsAt
	}
	return end.Before(since)
}

func (s *Store) findEntry(id uuid.UUID) int {
	return slices.IndexFunc(s.entries, func(e domain.Entry) bool { return e.ID == id })
}

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// cmpDue sorts by due time with entries that have none last.
func cmpDue(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return a.Compare(*b)
}

type entryPhoto struct {
	id, entryID uuid.UUID
	domain.Photo
}

func (s *Store) AddEntryPhoto(_ context.Context, entryID uuid.UUID, p domain.Photo) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !slices.ContainsFunc(s.entries, func(e domain.Entry) bool { return e.ID == entryID }) {
		return uuid.Nil, domain.ErrNotFound
	}
	id := uuid.New()
	s.entryPhotos = append(s.entryPhotos, entryPhoto{id: id, entryID: entryID, Photo: p})
	return id, nil
}

func (s *Store) EntryPhoto(_ context.Context, entryID, photoID uuid.UUID) (domain.Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.entryPhotos {
		if p.id == photoID && p.entryID == entryID {
			return p.Photo, nil
		}
	}
	return domain.Photo{}, domain.ErrNotFound
}

func (s *Store) EntryPhotoIDs(_ context.Context, entryID uuid.UUID) ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := []uuid.UUID{}
	for _, p := range s.entryPhotos {
		if p.entryID == entryID {
			ids = append(ids, p.id)
		}
	}
	return ids, nil
}

func (s *Store) DeleteEntryPhotos(_ context.Context, entryID uuid.UUID, ids []uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entryPhotos = slices.DeleteFunc(s.entryPhotos, func(p entryPhoto) bool { return p.entryID == entryID && slices.Contains(ids, p.id) })
	return nil
}
