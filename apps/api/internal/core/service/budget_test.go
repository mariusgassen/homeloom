package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestBudgetSplitsBalancesAndSettles(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob, eve := newUser(t, store, "alice"), newUser(t, store, "bob"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	store.AddMember(hh, bob.ID, domain.RoleMember)
	budget := service.NewBudget(store, store, time.UTC)

	// Evenly by default: Alice paid 30.01, so Bob owes 15.00 (Alice carries the odd cent).
	e, err := budget.Add(ctx, alice, hh, domain.ExpenseFields{Cents: 3001, Category: domain.CategoryGroceries, Note: " Market ", PaidBy: alice.ID})
	if err != nil || e.Note != "Market" || len(e.Shares) != 2 || e.Shares[0].Cents != 1501 || e.Day.IsZero() || e.Kind != domain.KindExpense {
		t.Fatalf("add: %+v, %v", e, err)
	}
	// Custom: Bob paid 10.00 for something only Alice uses.
	if _, err := budget.Add(ctx, bob, hh, domain.ExpenseFields{Cents: 1000, Category: domain.CategoryHealth, PaidBy: bob.ID, Shares: []domain.Share{{UserID: alice.ID, Cents: 1000}, {UserID: bob.ID}}}); err != nil {
		t.Fatal(err)
	}
	b, err := budget.Balance(ctx, bob, hh)
	if err != nil || len(b.Transfers) != 1 || b.Transfers[0] != (domain.Transfer{From: bob.ID, To: alice.ID, Cents: 500}) {
		t.Fatalf("balance: %+v, %v", b, err)
	}

	for name, f := range map[string]domain.ExpenseFields{
		"no amount":       {Category: domain.CategoryFun, PaidBy: alice.ID},
		"too much":        {Cents: domain.MaxExpenseCents + 1, Category: domain.CategoryFun, PaidBy: alice.ID},
		"unknown payer":   {Cents: 100, Category: domain.CategoryFun, PaidBy: eve.ID},
		"bad category":    {Cents: 100, Category: "yachts", PaidBy: alice.ID},
		"shares too low":  {Cents: 100, Category: domain.CategoryFun, PaidBy: alice.ID, Shares: []domain.Share{{UserID: alice.ID, Cents: 50}}},
		"outsider shares": {Cents: 100, Category: domain.CategoryFun, PaidBy: alice.ID, Shares: []domain.Share{{UserID: eve.ID, Cents: 100}}},
		"twice":           {Cents: 100, Category: domain.CategoryFun, PaidBy: alice.ID, Shares: []domain.Share{{UserID: bob.ID, Cents: 50}, {UserID: bob.ID, Cents: 50}}},
		"negative":        {Cents: 100, Category: domain.CategoryFun, PaidBy: alice.ID, Shares: []domain.Share{{UserID: bob.ID, Cents: 150}, {UserID: alice.ID, Cents: -50}}},
	} {
		if _, err := budget.Add(ctx, alice, hh, f); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := budget.Add(ctx, eve, hh, domain.ExpenseFields{Cents: 100, Category: domain.CategoryFun, PaidBy: eve.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider adds: %v", err)
	}
	if _, err := budget.Balance(ctx, eve, hh); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider reads: %v", err)
	}

	// Bob may change what Alice wrote down.
	e, err = budget.Update(ctx, bob, hh, e.ID, domain.ExpenseFields{Cents: 4000, Category: domain.CategoryGroceries, PaidBy: alice.ID})
	if err != nil || e.Cents != 4000 || e.Shares[1].Cents != 2000 || e.AddedBy == nil || *e.AddedBy != alice.ID {
		t.Fatalf("update: %+v, %v", e, err)
	}

	if _, err := budget.Settle(ctx, bob, hh, bob.ID, bob.ID, 100, time.Time{}); !errors.Is(err, domain.ErrPayer) {
		t.Fatalf("settle with oneself: %v", err)
	}
	s, err := budget.Settle(ctx, bob, hh, bob.ID, alice.ID, 1000, time.Time{})
	if err != nil || s.Kind != domain.KindSettlement || s.PaidBy != bob.ID || s.Shares[0].UserID != alice.ID {
		t.Fatalf("settle: %+v, %v", s, err)
	}
	if b, _ := budget.Balance(ctx, alice, hh); len(b.Transfers) != 0 || b.Members[0].Cents != 0 || b.Members[1].Cents != 0 {
		t.Fatalf("after settling: %+v", b)
	}
	if _, err := budget.Update(ctx, alice, hh, s.ID, domain.ExpenseFields{Cents: 100, Category: domain.CategoryOther, PaidBy: bob.ID}); !errors.Is(err, domain.ErrSettlement) {
		t.Fatalf("change a settlement: %v", err)
	}

	month, err := budget.Month(ctx, alice, hh, time.Time{})
	if err != nil || len(month.Expenses) != 3 || month.Expenses[0].ID != s.ID || !month.Earlier.IsZero() {
		t.Fatalf("month: %+v, %v", month, err)
	}
	if err := budget.Delete(ctx, eve, hh, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider deletes: %v", err)
	}
	if err := budget.Delete(ctx, alice, hh, s.ID); err != nil {
		t.Fatal(err)
	}
	if b, _ := budget.Balance(ctx, alice, hh); len(b.Transfers) != 1 || b.Transfers[0].Cents != 1000 {
		t.Fatalf("after deleting the settlement: %+v", b)
	}
}

func TestBudgetPagesByMonth(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	budget := service.NewBudget(store, store, time.UTC)
	august := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	if _, err := budget.Add(ctx, alice, hh, domain.ExpenseFields{Cents: 500, Category: domain.CategoryFun, PaidBy: alice.ID, Day: august}); err != nil {
		t.Fatal(err)
	}
	oct, err := budget.Month(ctx, alice, hh, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(oct.Expenses) != 0 || !oct.Earlier.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("october: %+v, %v", oct, err)
	}
	aug, _ := budget.Month(ctx, alice, hh, oct.Earlier)
	if len(aug.Expenses) != 1 || !aug.Expenses[0].Day.Equal(august) {
		t.Fatalf("august: %+v", aug)
	}
}

func TestBudgetSumsTheMonthByCategory(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, bob := newUser(t, store, "alice"), newUser(t, store, "bob")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	store.AddMember(hh, bob.ID, domain.RoleMember)
	budget := service.NewBudget(store, store, time.UTC)
	for _, f := range []domain.ExpenseFields{
		{Cents: 1000, Category: domain.CategoryFun, PaidBy: alice.ID},
		{Cents: 3000, Category: domain.CategoryGroceries, PaidBy: bob.ID},
		{Cents: 500, Category: domain.CategoryFun, PaidBy: bob.ID},
	} {
		if _, err := budget.Add(ctx, alice, hh, f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := budget.Settle(ctx, alice, hh, alice.ID, bob.ID, 999, time.Time{}); err != nil {
		t.Fatal(err)
	}
	month, err := budget.Month(ctx, alice, hh, time.Time{})
	want := []domain.CategoryTotal{{Category: domain.CategoryGroceries, Cents: 3000}, {Category: domain.CategoryFun, Cents: 1500}}
	if err != nil || month.Total != 4500 || len(month.Categories) != 2 || month.Categories[0] != want[0] || month.Categories[1] != want[1] {
		t.Fatalf("summary: %d %+v, %v", month.Total, month.Categories, err)
	}
}

func TestBudgetKeepsReceipts(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	hh := m.Household.ID
	e, err := service.NewBudget(store, store, time.UTC).Add(ctx, alice, hh, domain.ExpenseFields{Cents: 100, Category: domain.CategoryOther, PaidBy: alice.ID})
	if err != nil {
		t.Fatal(err)
	}
	jpeg := domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 1}}

	if _, err := service.NewBudget(store, store, time.UTC).SetReceipt(ctx, alice, hh, e.ID, jpeg); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("without a store: %v", err)
	}

	photos := memory.NewPhotos()
	budget := service.NewBudget(store, store, time.UTC).WithReceipts(photos)
	if _, err := budget.SetReceipt(ctx, alice, hh, e.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte("<svg>")}); !errors.Is(err, domain.ErrReceipt) {
		t.Fatalf("not a JPEG: %v", err)
	}
	if _, err := budget.SetReceipt(ctx, eve, hh, e.ID, jpeg); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	if _, err := budget.Receipt(ctx, alice, hh, e.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no receipt yet: %v", err)
	}
	got, err := budget.SetReceipt(ctx, alice, hh, e.ID, jpeg)
	if err != nil || got.ReceiptAt == nil {
		t.Fatalf("set: %+v, %v", got, err)
	}
	if p, err := budget.Receipt(ctx, alice, hh, e.ID); err != nil || len(p.Data) != 4 {
		t.Fatalf("read: %v", err)
	}
	if got, err := budget.DeleteReceipt(ctx, alice, hh, e.ID); err != nil || got.ReceiptAt != nil {
		t.Fatalf("delete receipt: %+v, %v", got, err)
	}
	if _, err := photos.GetPhoto(ctx, "receipts/"+e.ID.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo left behind: %v", err)
	}
	if _, err := budget.SetReceipt(ctx, alice, hh, e.ID, jpeg); err != nil {
		t.Fatal(err)
	}
	if err := budget.Delete(ctx, alice, hh, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := photos.GetPhoto(ctx, "receipts/"+e.ID.String()+".jpg"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("deleting the expense left its receipt: %v", err)
	}
}

func TestTickingABillBooksIt(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	budget := service.NewBudget(f.store, f.store, time.UTC)
	f.entries.WithBills(budget)
	due := f.now.Add(time.Hour)

	for name, fields := range map[string]domain.EntryFields{
		"private":       {Text: "Gym", Visibility: domain.VisibilityPrivate, Bill: &domain.Bill{Cents: 3000, Category: domain.CategoryHealth}},
		"no amount":     {Text: "Rent", Visibility: domain.VisibilityShared, Bill: &domain.Bill{Category: domain.CategoryHome}},
		"no category":   {Text: "Rent", Visibility: domain.VisibilityShared, Bill: &domain.Bill{Cents: 100}},
		"outside payer": {Text: "Rent", Visibility: domain.VisibilityShared, Bill: &domain.Bill{Cents: 100, Category: domain.CategoryHome, PaidBy: &f.eve.ID}},
	} {
		if _, err := f.entries.Create(ctx, f.alice, f.household, domain.KindReminder, fields); !errors.Is(err, domain.ErrBill) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.entries.Create(ctx, f.alice, f.household, domain.KindNote, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared, Bill: &domain.Bill{Cents: 100, Category: domain.CategoryHome}}); !errors.Is(err, domain.ErrBill) {
		t.Errorf("note: %v", err)
	}

	rent := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{
		Text: "Rent", Visibility: domain.VisibilityShared, DueAt: &due, Repeat: domain.RepeatMonthly,
		Bill: &domain.Bill{Cents: 90000, Category: domain.CategoryHome, PaidBy: &f.alice.ID},
	})
	if rent.Bill == nil || rent.Bill.Cents != 90000 {
		t.Fatalf("created: %+v", rent)
	}
	// Bob ticks it off; Alice pays rent, so Bob owes her half.
	ticked, err := f.entries.SetDone(ctx, f.bob, rent.ID, true)
	if err != nil || ticked.DueAt.Equal(due) {
		t.Fatalf("tick: %+v, %v", ticked, err)
	}
	month, _ := budget.Month(ctx, f.alice, f.household, f.now)
	if len(month.Expenses) != 1 || month.Expenses[0].Note != "Rent" || month.Expenses[0].PaidBy != f.alice.ID || month.Expenses[0].Category != domain.CategoryHome {
		t.Fatalf("booked: %+v", month.Expenses)
	}
	if b, _ := budget.Balance(ctx, f.bob, f.household); len(b.Transfers) != 1 || b.Transfers[0] != (domain.Transfer{From: f.bob.ID, To: f.alice.ID, Cents: 45000}) {
		t.Fatalf("balance: %+v", b)
	}

	// Without a payer, whoever ticks it off paid.
	once := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "Electricity", Visibility: domain.VisibilityShared, Bill: &domain.Bill{Cents: 6000, Category: domain.CategoryUtilities}})
	if _, err := f.entries.SetDone(ctx, f.bob, once.ID, true); err != nil {
		t.Fatal(err)
	}
	// Ticking it again once it's done books nothing more.
	if _, err := f.entries.SetDone(ctx, f.bob, once.ID, true); err != nil {
		t.Fatal(err)
	}
	month, _ = budget.Month(ctx, f.alice, f.household, f.now)
	if len(month.Expenses) != 2 || month.Expenses[0].PaidBy != f.bob.ID && month.Expenses[1].PaidBy != f.bob.ID {
		t.Fatalf("booked: %+v", month.Expenses)
	}

	// Without a bill an update drops it.
	upd, err := f.entries.Update(ctx, f.alice, rent.ID, domain.EntryFields{Text: "Rent", Visibility: domain.VisibilityShared, DueAt: ticked.DueAt, Repeat: domain.RepeatMonthly}, nil)
	if err != nil || upd.Bill != nil {
		t.Fatalf("update: %+v, %v", upd, err)
	}
}
