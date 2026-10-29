package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) CreateEntry(ctx context.Context, e domain.Entry) (domain.Entry, error) {
	row, err := s.q.CreateEntry(ctx, sqlcgen.CreateEntryParams{
		HouseholdID: e.HouseholdID, AuthorID: e.AuthorID, Kind: string(e.Kind), Visibility: string(e.Visibility),
		RevealAt: e.RevealAt, Text: e.Text, DueAt: e.DueAt, StartsAt: e.StartsAt, EndsAt: e.EndsAt, AllDay: e.AllDay,
		AlertMinutes: alertMinutes(e.AlertMinutes), Repeat: string(e.Repeat), AssigneeID: e.AssigneeID, NotifiedAt: e.NotifiedAt,
		IcalUid: nullable(e.ICalUID), IcalName: nullable(e.ICalName),
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return toEntry(row), nil
}

func (s *Store) EntryByICalName(ctx context.Context, householdID uuid.UUID, name string) (domain.Entry, error) {
	row, err := s.q.GetEntryByICalName(ctx, sqlcgen.GetEntryByICalNameParams{HouseholdID: householdID, IcalName: &name})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Entry{}, err
	}
	return toEntry(row), nil
}

func (s *Store) GetEntry(ctx context.Context, id uuid.UUID) (domain.Entry, error) {
	row, err := s.q.GetEntry(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Entry{}, err
	}
	return toEntry(row), nil
}

func (s *Store) RenameEntry(ctx context.Context, id uuid.UUID, name string) error {
	n, err := s.q.RenameEntry(ctx, sqlcgen.RenameEntryParams{ID: id, IcalName: &name})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) ListEntries(ctx context.Context, householdID, viewer uuid.UUID, kind domain.EntryKind, since time.Time, limit int) ([]domain.Entry, error) {
	var k *string
	if kind != "" {
		k = (*string)(&kind)
	}
	rows, err := s.q.ListEntries(ctx, sqlcgen.ListEntriesParams{
		HouseholdID: householdID, Viewer: viewer, Kind: k, Since: since, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	return toEntries(rows), nil
}

func (s *Store) UpdateEntry(ctx context.Context, e domain.Entry) (domain.Entry, error) {
	row, err := s.q.UpdateEntry(ctx, sqlcgen.UpdateEntryParams{
		ID: e.ID, Visibility: string(e.Visibility), RevealAt: e.RevealAt, Text: e.Text, DueAt: e.DueAt,
		StartsAt: e.StartsAt, EndsAt: e.EndsAt, AllDay: e.AllDay, AlertMinutes: alertMinutes(e.AlertMinutes),
		Repeat: string(e.Repeat), AssigneeID: e.AssigneeID, DoneAt: e.DoneAt, DoneBy: e.DoneBy, NotifiedAt: e.NotifiedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Entry{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Entry{}, err
	}
	return toEntry(row), nil
}

func (s *Store) DeleteEntry(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteEntry(ctx, id)
}

func (s *Store) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.Entry, error) {
	rows, err := s.q.ClaimDue(ctx, sqlcgen.ClaimDueParams{Now: now, Lim: int32(min(limit, 1000))}) //nolint:gosec // bounded above
	if err != nil {
		return nil, err
	}
	return toEntries(rows), nil
}

func toEntries(rows []sqlcgen.Entry) []domain.Entry {
	out := make([]domain.Entry, len(rows))
	for i, r := range rows {
		out[i] = toEntry(r)
	}
	return out
}

func toEntry(r sqlcgen.Entry) domain.Entry {
	e := domain.Entry{
		ID: r.ID, HouseholdID: r.HouseholdID, AuthorID: r.AuthorID, Kind: domain.EntryKind(r.Kind),
		EntryFields: domain.EntryFields{
			Visibility: domain.Visibility(r.Visibility), RevealAt: r.RevealAt, Text: r.Text, DueAt: r.DueAt,
			StartsAt: r.StartsAt, EndsAt: r.EndsAt, AllDay: r.AllDay, Repeat: domain.Repeat(r.Repeat), AssigneeID: r.AssigneeID,
		},
		DoneAt: r.DoneAt, DoneBy: r.DoneBy, NotifiedAt: r.NotifiedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		ICalUID: deref(r.IcalUid), ICalName: deref(r.IcalName),
	}
	if r.AlertMinutes != nil {
		m := int(*r.AlertMinutes)
		e.AlertMinutes = &m
	}
	return e
}

// alertMinutes fits the minutes into the column; the service keeps them to a week.
func alertMinutes(m *int) *int32 {
	if m == nil {
		return nil
	}
	v := int32(min(max(*m, 0), domain.MaxAlertMinutes)) //nolint:gosec // clamped above
	return &v
}

func (s *Store) SaveCalendarFeed(ctx context.Context, tokenHash []byte, f domain.CalendarFeed) error {
	return s.q.SaveCalendarFeed(ctx, sqlcgen.SaveCalendarFeedParams{TokenHash: tokenHash, UserID: f.UserID, HouseholdID: f.HouseholdID})
}

func (s *Store) CalendarFeed(ctx context.Context, tokenHash []byte) (domain.CalendarFeed, error) {
	row, err := s.q.GetCalendarFeed(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CalendarFeed{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.CalendarFeed{}, err
	}
	return domain.CalendarFeed{UserID: row.UserID, HouseholdID: row.HouseholdID}, nil
}

func (s *Store) HasCalendarFeed(ctx context.Context, f domain.CalendarFeed) (bool, error) {
	return s.q.HasCalendarFeed(ctx, sqlcgen.HasCalendarFeedParams{UserID: f.UserID, HouseholdID: f.HouseholdID})
}

func (s *Store) DeleteCalendarFeed(ctx context.Context, f domain.CalendarFeed) error {
	return s.q.DeleteCalendarFeed(ctx, sqlcgen.DeleteCalendarFeedParams{UserID: f.UserID, HouseholdID: f.HouseholdID})
}
