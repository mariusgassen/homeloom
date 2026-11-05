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

func (s *Store) Wishes(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.Wish, error) {
	rows, err := s.q.ListWishes(ctx, sqlcgen.ListWishesParams{
		HouseholdID: householdID, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	votes, err := s.wishVotes(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Wish, len(rows))
	for i, r := range rows {
		out[i] = toWish(r, votes[r.ID])
	}
	return out, nil
}

func (s *Store) CreateWish(ctx context.Context, w domain.Wish) (domain.Wish, error) {
	row, err := s.q.CreateWish(ctx, sqlcgen.CreateWishParams{
		HouseholdID: w.HouseholdID, Title: w.Title, Note: w.Note, Url: w.URL, PriceCents: w.PriceCents, CreatedBy: w.CreatedBy,
	})
	if err != nil {
		return domain.Wish{}, err
	}
	return toWish(row, nil), nil
}

func (s *Store) GetWish(ctx context.Context, householdID, id uuid.UUID) (domain.Wish, error) {
	row, err := s.q.GetWish(ctx, sqlcgen.GetWishParams{ID: id, HouseholdID: householdID})
	return s.wishResult(ctx, row, err)
}

func (s *Store) UpdateWish(ctx context.Context, w domain.Wish) (domain.Wish, error) {
	row, err := s.q.UpdateWish(ctx, sqlcgen.UpdateWishParams{
		Title: w.Title, Note: w.Note, Url: w.URL, PriceCents: w.PriceCents, ID: w.ID, HouseholdID: w.HouseholdID,
	})
	return s.wishResult(ctx, row, err)
}

func (s *Store) DeleteWish(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteWish(ctx, sqlcgen.DeleteWishParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) VoteWish(ctx context.Context, householdID, id, userID uuid.UUID, hearts int) (domain.Wish, error) {
	if hearts == 0 {
		// Only the household's own wish may be touched.
		if _, err := s.q.GetWish(ctx, sqlcgen.GetWishParams{ID: id, HouseholdID: householdID}); errors.Is(err, pgx.ErrNoRows) {
			return domain.Wish{}, domain.ErrNotFound
		} else if err != nil {
			return domain.Wish{}, err
		}
		if err := s.q.UnvoteWish(ctx, sqlcgen.UnvoteWishParams{ID: id, UserID: userID}); err != nil {
			return domain.Wish{}, err
		}
	} else {
		n, err := s.q.VoteWish(ctx, sqlcgen.VoteWishParams{ID: id, HouseholdID: householdID, UserID: userID, Hearts: int32(hearts)}) //nolint:gosec // 1-3
		if err != nil {
			return domain.Wish{}, err
		}
		if n == 0 {
			return domain.Wish{}, domain.ErrNotFound
		}
	}
	return s.GetWish(ctx, householdID, id)
}

func (s *Store) SetWishGot(ctx context.Context, householdID, id uuid.UUID, at *time.Time) (domain.Wish, error) {
	row, err := s.q.SetWishGot(ctx, sqlcgen.SetWishGotParams{GotAt: at, ID: id, HouseholdID: householdID})
	return s.wishResult(ctx, row, err)
}

func (s *Store) SetWishPhoto(ctx context.Context, householdID, id uuid.UUID, at *time.Time) (domain.Wish, error) {
	row, err := s.q.SetWishPhoto(ctx, sqlcgen.SetWishPhotoParams{PhotoAt: at, ID: id, HouseholdID: householdID})
	return s.wishResult(ctx, row, err)
}

// wishResult is the wish with its votes, or the error of the query that returned it.
func (s *Store) wishResult(ctx context.Context, row sqlcgen.Wish, err error) (domain.Wish, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wish{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Wish{}, err
	}
	votes, err := s.wishVotes(ctx, []uuid.UUID{row.ID})
	if err != nil {
		return domain.Wish{}, err
	}
	return toWish(row, votes[row.ID]), nil
}

func (s *Store) wishVotes(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]domain.WishVote, error) {
	out := map[uuid.UUID][]domain.WishVote{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.WishVotes(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.WishID] = append(out[r.WishID], domain.WishVote{UserID: r.UserID, Hearts: int(r.Hearts)})
	}
	return out, nil
}

func toWish(r sqlcgen.Wish, votes []domain.WishVote) domain.Wish {
	return domain.Wish{
		ID: r.ID, HouseholdID: r.HouseholdID, PhotoAt: r.PhotoAt, GotAt: r.GotAt, Votes: votes, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		WishFields: domain.WishFields{Title: r.Title, Note: r.Note, URL: r.Url, PriceCents: r.PriceCents},
	}
}
