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

func (s *Store) CreateMoment(ctx context.Context, m domain.Moment) (domain.Moment, error) {
	row, err := s.q.CreateMoment(ctx, sqlcgen.CreateMomentParams{
		HouseholdID: m.HouseholdID, AuthorID: m.AuthorID, Kind: string(m.Kind), Day: m.Day,
		Visibility: string(m.Visibility), RevealAt: m.RevealAt, Text: m.Text,
	})
	if err != nil {
		return domain.Moment{}, err
	}
	return toMoment(row), nil
}

func (s *Store) SaveGratitude(ctx context.Context, m domain.Moment) (domain.Moment, error) {
	row, err := s.q.SaveGratitude(ctx, sqlcgen.SaveGratitudeParams{
		HouseholdID: m.HouseholdID, AuthorID: m.AuthorID, Day: m.Day, Visibility: string(m.Visibility), Text: m.Text,
	})
	if err != nil {
		return domain.Moment{}, err
	}
	return toMoment(row), nil
}

func (s *Store) GetMoment(ctx context.Context, id uuid.UUID) (domain.Moment, error) {
	row, err := s.q.GetMoment(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Moment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Moment{}, err
	}
	return toMoment(row), nil
}

func (s *Store) UpdateMoment(ctx context.Context, m domain.Moment) (domain.Moment, error) {
	row, err := s.q.UpdateMoment(ctx, sqlcgen.UpdateMomentParams{
		ID: m.ID, Day: m.Day, Visibility: string(m.Visibility), RevealAt: m.RevealAt, Text: m.Text,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Moment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Moment{}, err
	}
	return toMoment(row), nil
}

func (s *Store) DeleteMoment(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteMoment(ctx, id)
}

func (s *Store) ListMoments(ctx context.Context, householdID, viewer uuid.UUID, from, to time.Time, limit int) ([]domain.Moment, error) {
	rows, err := s.q.ListMoments(ctx, sqlcgen.ListMomentsParams{
		HouseholdID: householdID, Viewer: viewer, FromDay: from, ToDay: to, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	return toMoments(rows), nil
}

func (s *Store) LatestMomentDay(ctx context.Context, householdID, viewer uuid.UUID, before time.Time) (time.Time, error) {
	day, err := s.q.LatestMomentDay(ctx, sqlcgen.LatestMomentDayParams{HouseholdID: householdID, Viewer: viewer, BeforeDay: before})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, domain.ErrNotFound
	}
	return day, err
}

func (s *Store) MomentsOnDay(ctx context.Context, householdID, viewer uuid.UUID, today time.Time, limit int) ([]domain.Moment, error) {
	rows, err := s.q.MomentsOnDay(ctx, sqlcgen.MomentsOnDayParams{
		HouseholdID: householdID, Viewer: viewer, Today: today, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	return toMoments(rows), nil
}

func (s *Store) RandomGratitude(ctx context.Context, householdID, viewer uuid.UUID, before time.Time) (domain.Moment, error) {
	row, err := s.q.RandomGratitude(ctx, sqlcgen.RandomGratitudeParams{HouseholdID: householdID, Viewer: viewer, BeforeDay: before})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Moment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Moment{}, err
	}
	return toMoment(row), nil
}

func (s *Store) SetMomentPhoto(ctx context.Context, id uuid.UUID, p domain.Photo) error {
	return s.q.SetMomentPhoto(ctx, sqlcgen.SetMomentPhotoParams{MomentID: id, ContentType: p.ContentType, Data: p.Data})
}

func (s *Store) MarkMomentPhoto(ctx context.Context, id uuid.UUID) error {
	return s.q.MarkMomentPhoto(ctx, id)
}

func (s *Store) MomentPhoto(ctx context.Context, id uuid.UUID) (domain.Photo, error) {
	row, err := s.q.GetMomentPhoto(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Photo{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Photo{}, err
	}
	return domain.Photo{ContentType: row.ContentType, Data: row.Data}, nil
}

func (s *Store) DeleteMomentPhoto(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteMomentPhoto(ctx, id)
}

func (s *Store) HouseholdIDs(ctx context.Context) ([]uuid.UUID, error) {
	return s.q.ListHouseholdIDs(ctx)
}

func (s *Store) ClaimJobRun(ctx context.Context, job string, day time.Time) (bool, error) {
	n, err := s.q.ClaimJobRun(ctx, sqlcgen.ClaimJobRunParams{Job: job, Day: day})
	return n == 1, err
}

func toMoments(rows []sqlcgen.Moment) []domain.Moment {
	out := make([]domain.Moment, len(rows))
	for i, r := range rows {
		out[i] = toMoment(r)
	}
	return out
}

func toMoment(r sqlcgen.Moment) domain.Moment {
	return domain.Moment{
		ID: r.ID, HouseholdID: r.HouseholdID, AuthorID: r.AuthorID, Kind: domain.MomentKind(r.Kind),
		MomentFields: domain.MomentFields{Day: r.Day, Visibility: domain.Visibility(r.Visibility), RevealAt: r.RevealAt, Text: r.Text},
		PhotoAt:      r.PhotoAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
