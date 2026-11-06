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

func (s *Store) SetBirthday(ctx context.Context, userID uuid.UUID, b *domain.Birthday) error {
	if b == nil {
		return s.q.ClearBirthday(ctx, userID)
	}
	return s.q.SetBirthday(ctx, sqlcgen.SetBirthdayParams{UserID: userID, Month: int16(b.Month), Day: int16(b.Day), Year: yearArg(b.Year)}) //nolint:gosec // validated 1-12, 1-31 and 1900-now
}

func (s *Store) Birthdays(ctx context.Context, householdID uuid.UUID) (map[uuid.UUID]domain.Birthday, error) {
	rows, err := s.q.HouseholdBirthdays(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]domain.Birthday, len(rows))
	for _, r := range rows {
		out[r.UserID] = domain.Birthday{Month: time.Month(r.Month), Day: int(r.Day), Year: yearOf(r.Year)}
	}
	return out, nil
}

func (s *Store) GiftIdeas(ctx context.Context, householdID, authorID uuid.UUID, limit int) ([]domain.GiftIdea, error) {
	rows, err := s.q.ListGiftIdeas(ctx, sqlcgen.ListGiftIdeasParams{
		HouseholdID: householdID, AuthorID: authorID, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.GiftIdea, len(rows))
	for i, r := range rows {
		out[i] = toGiftIdea(r)
	}
	return out, nil
}

func (s *Store) AddGiftIdea(ctx context.Context, idea domain.GiftIdea) (domain.GiftIdea, error) {
	row, err := s.q.AddGiftIdea(ctx, sqlcgen.AddGiftIdeaParams{HouseholdID: idea.HouseholdID, AuthorID: idea.AuthorID, ForID: idea.ForID, Text: idea.Text})
	if err != nil {
		return domain.GiftIdea{}, err
	}
	return toGiftIdea(row), nil
}

func (s *Store) SetGiftIdeaGiven(ctx context.Context, householdID, authorID, id uuid.UUID, given bool) (domain.GiftIdea, error) {
	row, err := s.q.SetGiftIdeaGiven(ctx, sqlcgen.SetGiftIdeaGivenParams{Given: given, ID: id, HouseholdID: householdID, AuthorID: authorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.GiftIdea{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.GiftIdea{}, err
	}
	return toGiftIdea(row), nil
}

func (s *Store) DeleteGiftIdea(ctx context.Context, householdID, authorID, id uuid.UUID) error {
	n, err := s.q.DeleteGiftIdea(ctx, sqlcgen.DeleteGiftIdeaParams{ID: id, HouseholdID: householdID, AuthorID: authorID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func toGiftIdea(r sqlcgen.GiftIdea) domain.GiftIdea {
	return domain.GiftIdea{ID: r.ID, HouseholdID: r.HouseholdID, AuthorID: r.AuthorID, ForID: r.ForID, Text: r.Text, GivenAt: r.GivenAt, Created: r.CreatedAt}
}

// yearArg is the year of birth as a nullable column; 0 means not given.
func yearArg(year int) *int16 {
	if year == 0 {
		return nil
	}
	y := int16(year) //nolint:gosec // validated 1900-now
	return &y
}

func yearOf(y *int16) int {
	if y == nil {
		return 0
	}
	return int(*y)
}
