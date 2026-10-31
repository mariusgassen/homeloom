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

func (s *Store) Chores(ctx context.Context, householdID uuid.UUID, limit int) ([]domain.Chore, error) {
	rows, err := s.q.ListChores(ctx, sqlcgen.ListChoresParams{
		HouseholdID: householdID, Lim: int32(min(limit, 1000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Chore, len(rows))
	for i, r := range rows {
		out[i] = toChore(r)
	}
	return out, nil
}

func (s *Store) CreateChore(ctx context.Context, c domain.Chore) (domain.Chore, error) {
	row, err := s.q.CreateChore(ctx, sqlcgen.CreateChoreParams{
		HouseholdID: c.HouseholdID, Title: c.Title, Points: int32(c.Points), Repeat: string(c.Repeat), //nolint:gosec // 1-3
		Rotate: c.Rotate, AssigneeID: c.AssigneeID, DueOn: c.DueOn, CreatedBy: c.CreatedBy,
	})
	if err != nil {
		return domain.Chore{}, err
	}
	return toChore(row), nil
}

func (s *Store) GetChore(ctx context.Context, householdID, id uuid.UUID) (domain.Chore, error) {
	row, err := s.q.GetChore(ctx, sqlcgen.GetChoreParams{ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Chore{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Chore{}, err
	}
	return toChore(row), nil
}

func (s *Store) UpdateChore(ctx context.Context, c domain.Chore) (domain.Chore, error) {
	row, err := s.q.UpdateChore(ctx, sqlcgen.UpdateChoreParams{
		Title: c.Title, Points: int32(c.Points), Repeat: string(c.Repeat), Rotate: c.Rotate, //nolint:gosec // 1-3
		AssigneeID: c.AssigneeID, DueOn: c.DueOn, ID: c.ID, HouseholdID: c.HouseholdID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Chore{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Chore{}, err
	}
	return toChore(row), nil
}

func (s *Store) DeleteChore(ctx context.Context, householdID, id uuid.UUID) error {
	n, err := s.q.DeleteChore(ctx, sqlcgen.DeleteChoreParams{ID: id, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) CompleteChore(ctx context.Context, before, after domain.Chore, done domain.ChoreCompletion) (domain.ChoreCompletion, error) {
	var out domain.ChoreCompletion
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		// The lock makes a second tap at the same moment wait, then find the chore moved on.
		c, err := q.LockChore(ctx, sqlcgen.LockChoreParams{ID: before.ID, HouseholdID: before.HouseholdID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if c.DoneAt != nil || !c.DueOn.Equal(before.DueOn) || !sameUUID(c.AssigneeID, before.AssigneeID) {
			return domain.ErrConflict
		}
		if err := q.SetChoreState(ctx, sqlcgen.SetChoreStateParams{DueOn: after.DueOn, AssigneeID: after.AssigneeID, DoneAt: after.DoneAt, ID: c.ID}); err != nil {
			return err
		}
		row, err := q.AddChoreCompletion(ctx, sqlcgen.AddChoreCompletionParams{
			HouseholdID: done.HouseholdID, ChoreID: done.ChoreID, Title: done.Title, Points: int32(done.Points), //nolint:gosec // 1-3
			UserID: done.UserID, Day: done.Day, PrevDueOn: done.PrevDueOn, PrevAssigneeID: done.PrevAssigneeID,
		})
		out = toChoreCompletion(row, nil)
		return err
	})
	if err != nil {
		return domain.ChoreCompletion{}, err
	}
	return out, nil
}

func (s *Store) LastChoreCompletion(ctx context.Context, householdID, choreID uuid.UUID) (domain.ChoreCompletion, error) {
	row, err := s.q.LastChoreCompletion(ctx, sqlcgen.LastChoreCompletionParams{HouseholdID: householdID, ChoreID: &choreID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChoreCompletion{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ChoreCompletion{}, err
	}
	return s.withThanks(ctx, row)
}

func (s *Store) GetChoreCompletion(ctx context.Context, householdID, id uuid.UUID) (domain.ChoreCompletion, error) {
	row, err := s.q.GetChoreCompletion(ctx, sqlcgen.GetChoreCompletionParams{ID: id, HouseholdID: householdID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ChoreCompletion{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ChoreCompletion{}, err
	}
	return s.withThanks(ctx, row)
}

func (s *Store) UndoChoreCompletion(ctx context.Context, done domain.ChoreCompletion) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		d, err := q.GetChoreCompletion(ctx, sqlcgen.GetChoreCompletionParams{ID: done.ID, HouseholdID: done.HouseholdID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if d.ChoreID != nil {
			// Under the chore's lock, no new completion can come between checking and undoing.
			_, err := q.LockChore(ctx, sqlcgen.LockChoreParams{ID: *d.ChoreID, HouseholdID: d.HouseholdID})
			if err != nil {
				return err
			}
			last, err := q.LastChoreCompletion(ctx, sqlcgen.LastChoreCompletionParams{HouseholdID: d.HouseholdID, ChoreID: d.ChoreID})
			if err != nil {
				return err
			}
			if last.ID != d.ID {
				return domain.ErrConflict
			}
			if err := q.SetChoreState(ctx, sqlcgen.SetChoreStateParams{DueOn: d.PrevDueOn, AssigneeID: d.PrevAssigneeID, ID: *d.ChoreID}); err != nil {
				return err
			}
		}
		return q.DeleteChoreCompletion(ctx, d.ID)
	})
}

func (s *Store) ChoreCompletions(ctx context.Context, householdID uuid.UUID, since time.Time, limit int) ([]domain.ChoreCompletion, error) {
	rows, err := s.q.ListChoreCompletions(ctx, sqlcgen.ListChoreCompletionsParams{
		HouseholdID: householdID, Since: since, Lim: int32(min(limit, 5000)), //nolint:gosec // bounded above
	})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	thanks, err := s.thanks(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChoreCompletion, len(rows))
	for i, r := range rows {
		out[i] = toChoreCompletion(r, thanks[r.ID])
	}
	return out, nil
}

func (s *Store) ThankChoreCompletion(ctx context.Context, householdID, id, userID uuid.UUID) (bool, error) {
	n, err := s.q.ThankChoreCompletion(ctx, sqlcgen.ThankChoreCompletionParams{UserID: userID, ID: id, HouseholdID: householdID})
	if err != nil || n > 0 {
		return n > 0, err
	}
	// Nothing new: thanked before, or no such completion.
	_, err = s.GetChoreCompletion(ctx, householdID, id)
	return false, err
}

func (s *Store) withThanks(ctx context.Context, r sqlcgen.ChoreCompletion) (domain.ChoreCompletion, error) {
	thanks, err := s.thanks(ctx, []uuid.UUID{r.ID})
	if err != nil {
		return domain.ChoreCompletion{}, err
	}
	return toChoreCompletion(r, thanks[r.ID]), nil
}

// thanks returns who thanked for each of the completions, in the order they did.
func (s *Store) thanks(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ChoreThanks(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CompletionID] = append(out[r.CompletionID], r.UserID)
	}
	return out, nil
}

func toChore(r sqlcgen.Chore) domain.Chore {
	return domain.Chore{
		ID: r.ID, HouseholdID: r.HouseholdID, DoneAt: r.DoneAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		ChoreFields: domain.ChoreFields{
			Title: r.Title, Points: int(r.Points), Repeat: domain.Repeat(r.Repeat), Rotate: r.Rotate, AssigneeID: r.AssigneeID, DueOn: r.DueOn,
		},
	}
}

func toChoreCompletion(r sqlcgen.ChoreCompletion, thankedBy []uuid.UUID) domain.ChoreCompletion {
	return domain.ChoreCompletion{
		ID: r.ID, HouseholdID: r.HouseholdID, ChoreID: r.ChoreID, Title: r.Title, Points: int(r.Points), UserID: r.UserID,
		Day: r.Day, DoneAt: r.DoneAt, PrevDueOn: r.PrevDueOn, PrevAssigneeID: r.PrevAssigneeID, ThankedBy: thankedBy,
	}
}

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
