package storetest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Chores: open chores soonest first, completing only a chore that is as it was read, undoing
// only the latest completion, thanks once per person, and no reaching into another household.
func Chores(t *testing.T, s interface {
	Store
	port.ChoreRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	join(t, s, home.Household.ID, alice, bob)
	hh := home.Household.ID
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

	add := func(title string, due time.Time, assignee *uuid.UUID) domain.Chore {
		t.Helper()
		c, err := s.CreateChore(ctx, domain.Chore{HouseholdID: hh, CreatedBy: &alice.ID, ChoreFields: domain.ChoreFields{
			Title: title, Points: 2, Repeat: domain.RepeatWeekly, Rotate: true, AssigneeID: assignee, DueOn: due,
		}})
		if err != nil || c.ID == uuid.Nil || c.Title != title || !c.DueOn.Equal(due) || c.Points != 2 || !c.Rotate {
			t.Fatalf("create %q: %+v, %v", title, c, err)
		}
		return c
	}
	later := add("Windows", day(30), nil)
	bins := add("Bins", day(28), &alice.ID)
	if _, err := s.CreateChore(ctx, domain.Chore{HouseholdID: other.Household.ID, ChoreFields: domain.ChoreFields{Title: "Theirs", Points: 1, Repeat: domain.RepeatNever, DueOn: day(1)}}); err != nil {
		t.Fatal(err)
	}
	if cs, err := s.Chores(ctx, hh, 10); err != nil || len(cs) != 2 || cs[0].ID != bins.ID || cs[1].ID != later.ID {
		t.Fatalf("list: soonest first, only this household's: %+v, %v", cs, err)
	}
	if _, err := s.GetChore(ctx, other.Household.ID, bins.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get from another household: want ErrNotFound, got %v", err)
	}

	later.Title, later.Points = "Clean windows", 3
	if c, err := s.UpdateChore(ctx, later); err != nil || c.Title != "Clean windows" || c.Points != 3 {
		t.Fatalf("update: %+v, %v", c, err)
	}

	complete := func(before domain.Chore, by domain.User, due time.Time, next *uuid.UUID) (domain.ChoreCompletion, error) {
		after := before
		after.DueOn, after.AssigneeID = due, next
		return s.CompleteChore(ctx, before, after, domain.ChoreCompletion{
			HouseholdID: hh, ChoreID: &before.ID, Title: before.Title, Points: before.Points, UserID: by.ID,
			Day: before.DueOn, PrevDueOn: before.DueOn, PrevAssigneeID: before.AssigneeID,
		})
	}
	first, err := complete(bins, alice, day(4).AddDate(0, 1, 0), &bob.ID)
	if err != nil || first.ID == uuid.Nil || first.UserID != alice.ID || first.Points != 2 || len(first.ThankedBy) != 0 {
		t.Fatalf("complete: %+v, %v", first, err)
	}
	if _, err := complete(bins, bob, day(4).AddDate(0, 1, 0), &alice.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("complete from a stale read: want ErrConflict, got %v", err)
	}
	moved, _ := s.GetChore(ctx, hh, bins.ID)
	if !moved.DueOn.Equal(day(4).AddDate(0, 1, 0)) || *moved.AssigneeID != bob.ID {
		t.Fatalf("after completing: %+v", moved)
	}
	second, err := complete(moved, bob, day(11).AddDate(0, 1, 0), &alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if last, err := s.LastChoreCompletion(ctx, hh, bins.ID); err != nil || last.ID != second.ID {
		t.Fatalf("last: %+v, %v", last, err)
	}
	if _, err := s.LastChoreCompletion(ctx, hh, later.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("last of a chore never done: want ErrNotFound, got %v", err)
	}

	// Thanks: once per person, and only in the household.
	if added, err := s.ThankChoreCompletion(ctx, hh, first.ID, bob.ID); err != nil || !added {
		t.Fatalf("thank: %v, %v", added, err)
	}
	if added, err := s.ThankChoreCompletion(ctx, hh, first.ID, bob.ID); err != nil || added {
		t.Fatalf("thank again: %v, %v", added, err)
	}
	if _, err := s.ThankChoreCompletion(ctx, other.Household.ID, first.ID, bob.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("thank in another household: want ErrNotFound, got %v", err)
	}
	if got, err := s.GetChoreCompletion(ctx, hh, first.ID); err != nil || !slices.Equal(got.ThankedBy, []uuid.UUID{bob.ID}) {
		t.Fatalf("get: %+v, %v", got, err)
	}

	if err := s.UndoChoreCompletion(ctx, first); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("undo an earlier one: want ErrConflict, got %v", err)
	}
	if err := s.UndoChoreCompletion(ctx, second); err != nil {
		t.Fatal(err)
	}
	if back, _ := s.GetChore(ctx, hh, bins.ID); !back.DueOn.Equal(moved.DueOn) || *back.AssigneeID != bob.ID {
		t.Fatalf("after undo: %+v; want it as before the second", back)
	}

	if done, err := s.ChoreCompletions(ctx, hh, day(1), 10); err != nil || len(done) != 1 || done[0].ID != first.ID || !slices.Equal(done[0].ThankedBy, []uuid.UUID{bob.ID}) {
		t.Fatalf("history: %+v, %v", done, err)
	}
	if done, err := s.ChoreCompletions(ctx, hh, day(29), 10); err != nil || len(done) != 0 {
		t.Fatalf("history since a later day: %+v, %v", done, err)
	}

	// Deleting the chore keeps what was done, without the chore.
	if err := s.DeleteChore(ctx, other.Household.ID, bins.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete from another household: want ErrNotFound, got %v", err)
	}
	if err := s.DeleteChore(ctx, hh, bins.ID); err != nil {
		t.Fatal(err)
	}
	done, err := s.ChoreCompletions(ctx, hh, day(1), 10)
	if err != nil || len(done) != 1 || done[0].ChoreID != nil || done[0].Title != "Bins" {
		t.Fatalf("history after delete: %+v, %v", done, err)
	}
	if err := s.UndoChoreCompletion(ctx, done[0]); err != nil {
		t.Fatalf("undo once the chore is gone: %v", err)
	}
}
