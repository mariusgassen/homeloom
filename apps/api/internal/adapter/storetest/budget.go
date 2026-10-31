package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Expenses: stored with their shares, listed by month, changed and deleted only within their
// household, and summed into nets.
func Expenses(t *testing.T, s interface {
	Store
	port.BudgetRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hh := home.Household.ID
	oct, sep := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)

	groceries, err := s.CreateExpense(ctx, domain.Expense{
		HouseholdID: hh, Kind: domain.KindExpense, Cents: 3000, Category: domain.CategoryGroceries, Note: "Market",
		PaidBy: alice.ID, Day: oct, AddedBy: &alice.ID,
		Shares: []domain.Share{{UserID: alice.ID, Cents: 1500}, {UserID: bob.ID, Cents: 1500}},
	})
	if err != nil || groceries.Note != "Market" || len(groceries.Shares) != 2 || groceries.Created.IsZero() {
		t.Fatalf("create: %+v, %v", groceries, err)
	}
	if _, err := s.CreateExpense(ctx, domain.Expense{
		HouseholdID: hh, Kind: domain.KindSettlement, Cents: 500, Category: domain.CategoryOther,
		PaidBy: bob.ID, Day: sep, Shares: []domain.Share{{UserID: alice.ID, Cents: 500}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateExpense(ctx, domain.Expense{
		HouseholdID: other.Household.ID, Kind: domain.KindExpense, Cents: 999, Category: domain.CategoryFun,
		PaidBy: alice.ID, Day: oct, Shares: []domain.Share{{UserID: alice.ID, Cents: 999}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Expenses(ctx, hh, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), 100)
	if err != nil || len(got) != 1 || got[0].ID != groceries.ID || len(got[0].Shares) != 2 || !got[0].Day.Equal(oct) {
		t.Fatalf("october: %+v, %v", got, err)
	}
	if day, err := s.LatestExpenseDay(ctx, hh, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); err != nil || !day.Equal(sep) {
		t.Fatalf("latest before october: %v, %v", day, err)
	}
	if _, err := s.LatestExpenseDay(ctx, hh, sep); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nothing earlier: %v", err)
	}

	nets, err := s.Nets(ctx, hh)
	want := map[string]int64{alice.ID.String(): 3000 - 1500 - 500, bob.ID.String(): 500 - 1500}
	if err != nil || len(nets) != 2 {
		t.Fatalf("nets: %+v, %v", nets, err)
	}
	for _, n := range nets {
		if want[n.UserID.String()] != n.Cents {
			t.Fatalf("nets: %+v, want %v", nets, want)
		}
	}

	groceries.Cents, groceries.Note, groceries.Shares = 4000, "", []domain.Share{{UserID: bob.ID, Cents: 4000}}
	updated, err := s.UpdateExpense(ctx, groceries)
	if err != nil || updated.Cents != 4000 || updated.Note != "" || len(updated.Shares) != 1 {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if read, err := s.Expense(ctx, hh, groceries.ID); err != nil || len(read.Shares) != 1 || read.Shares[0].UserID != bob.ID || read.Kind != domain.KindExpense {
		t.Fatalf("read back: %+v, %v", read, err)
	}
	if _, err := s.Expense(ctx, other.Household.ID, groceries.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("read from another household: %v", err)
	}
	moved := updated
	moved.HouseholdID = other.Household.ID
	if _, err := s.UpdateExpense(ctx, moved); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update from another household: %v", err)
	}
	if err := s.DeleteExpense(ctx, other.Household.ID, groceries.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete from another household: %v", err)
	}
	if err := s.DeleteExpense(ctx, hh, groceries.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExpense(ctx, hh, groceries.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
