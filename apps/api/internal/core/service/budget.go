package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Budget is the household's shared money. One member pays, the expense is split into shares
// (evenly between all members unless said otherwise), and where everyone stands is what they
// paid minus their shares. Settling up is a payment from one member to another, kept as an
// expense of its own kind, so the balance is one sum over everything.
type Budget struct {
	households port.HouseholdRepository
	budget     port.BudgetRepository
	loc        *time.Location
	now        func() time.Time
}

// NewBudget counts days in loc, the household's time zone.
func NewBudget(households port.HouseholdRepository, budget port.BudgetRepository, loc *time.Location) *Budget {
	return &Budget{households: households, budget: budget, loc: loc, now: time.Now}
}

func (s *Budget) Month(ctx context.Context, actor domain.User, householdID uuid.UUID, month time.Time) (domain.ExpenseMonth, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.ExpenseMonth{}, err
	}
	if month.IsZero() {
		month = s.today()
	}
	from := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	es, err := s.budget.Expenses(ctx, householdID, from, from.AddDate(0, 1, 0), domain.MaxExpensesPerMonth)
	if err != nil {
		return domain.ExpenseMonth{}, err
	}
	out := domain.ExpenseMonth{Month: from, Expenses: es}
	earlier, err := s.budget.LatestExpenseDay(ctx, householdID, from)
	switch {
	case err == nil:
		out.Earlier = time.Date(earlier.Year(), earlier.Month(), 1, 0, 0, 0, 0, time.UTC)
	case !errors.Is(err, domain.ErrNotFound):
		return domain.ExpenseMonth{}, err
	}
	return out, nil
}

func (s *Budget) Add(ctx context.Context, actor domain.User, householdID uuid.UUID, f domain.ExpenseFields) (domain.Expense, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Expense{}, err
	}
	e, err := s.check(f, members, members)
	if err != nil {
		return domain.Expense{}, err
	}
	e.HouseholdID, e.Kind, e.AddedBy = householdID, domain.KindExpense, &actor.ID
	return s.budget.CreateExpense(ctx, e)
}

func (s *Budget) Update(ctx context.Context, actor domain.User, householdID, id uuid.UUID, f domain.ExpenseFields) (domain.Expense, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Expense{}, err
	}
	old, err := s.budget.Expense(ctx, householdID, id)
	if err != nil {
		return domain.Expense{}, err
	}
	if old.Kind == domain.KindSettlement {
		return domain.Expense{}, domain.ErrSettlement
	}
	// Someone who has left may still be named on what they were part of.
	allowed := slices.Clone(members)
	for _, u := range append([]uuid.UUID{old.PaidBy}, userIDs(old.Shares)...) {
		if !slices.Contains(allowed, u) {
			allowed = append(allowed, u)
		}
	}
	e, err := s.check(f, members, allowed)
	if err != nil {
		return domain.Expense{}, err
	}
	e.ID, e.HouseholdID, e.Kind, e.AddedBy = old.ID, householdID, old.Kind, old.AddedBy
	return s.budget.UpdateExpense(ctx, e)
}

func (s *Budget) Delete(ctx context.Context, actor domain.User, householdID, id uuid.UUID) error {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return err
	}
	return s.budget.DeleteExpense(ctx, householdID, id)
}

func (s *Budget) Settle(ctx context.Context, actor domain.User, householdID, from, to uuid.UUID, cents int64, day time.Time) (domain.Expense, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Expense{}, err
	}
	if from == to || !slices.Contains(members, from) || !slices.Contains(members, to) {
		return domain.Expense{}, domain.ErrPayer
	}
	e, err := s.check(domain.ExpenseFields{Cents: cents, Category: domain.CategoryOther, PaidBy: from, Day: day, Shares: []domain.Share{{UserID: to, Cents: cents}}}, members, members)
	if err != nil {
		return domain.Expense{}, err
	}
	e.HouseholdID, e.Kind, e.AddedBy = householdID, domain.KindSettlement, &actor.ID
	return s.budget.CreateExpense(ctx, e)
}

func (s *Budget) Balance(ctx context.Context, actor domain.User, householdID uuid.UUID) (domain.Balance, error) {
	members, err := s.members(ctx, actor, householdID)
	if err != nil {
		return domain.Balance{}, err
	}
	nets, err := s.budget.Nets(ctx, householdID)
	if err != nil {
		return domain.Balance{}, err
	}
	// Every member is listed, at zero when they have nothing yet; someone who left only while
	// they still owe or are owed.
	out := domain.Balance{Members: make([]domain.Net, 0, len(members))}
	for _, m := range members {
		n := domain.Net{UserID: m}
		if i := slices.IndexFunc(nets, func(x domain.Net) bool { return x.UserID == m }); i >= 0 {
			n.Cents = nets[i].Cents
		}
		out.Members = append(out.Members, n)
	}
	for _, n := range nets {
		if n.Cents != 0 && !slices.Contains(members, n.UserID) {
			out.Members = append(out.Members, n)
		}
	}
	out.Transfers = domain.Settle(out.Members)
	return out, nil
}

// members checks the actor belongs to the household and returns its members' ids, oldest first.
func (s *Budget) members(ctx context.Context, actor domain.User, householdID uuid.UUID) ([]uuid.UUID, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return nil, err
	}
	ms, err := s.households.Members(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, len(ms))
	for i, m := range ms {
		out[i] = m.User.ID
	}
	return out, nil
}

// check validates the fields and fills in what was left out: today for the day, an even split
// between the members for the shares. Payer and shares must be among allowed.
func (s *Budget) check(f domain.ExpenseFields, members, allowed []uuid.UUID) (domain.Expense, error) {
	if f.Cents < 1 || f.Cents > domain.MaxExpenseCents {
		return domain.Expense{}, domain.ErrAmount
	}
	if !f.Category.Valid() {
		return domain.Expense{}, domain.ErrCategory
	}
	if !slices.Contains(allowed, f.PaidBy) {
		return domain.Expense{}, domain.ErrPayer
	}
	note := strings.TrimSpace(f.Note)
	if utf8.RuneCountInString(note) > domain.MaxExpenseNote {
		return domain.Expense{}, domain.TextError{Max: domain.MaxExpenseNote}
	}
	day := f.Day
	if day.IsZero() {
		day = s.today()
	}
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	shares := f.Shares
	if len(shares) == 0 {
		shares = domain.SplitEvenly(f.Cents, members)
	}
	var sum int64
	seen := make(map[uuid.UUID]bool, len(shares))
	kept := make([]domain.Share, 0, len(shares))
	for _, sh := range shares {
		if sh.Cents < 0 || seen[sh.UserID] || !slices.Contains(allowed, sh.UserID) {
			return domain.Expense{}, domain.ErrShares
		}
		seen[sh.UserID] = true
		sum += sh.Cents
		if sh.Cents > 0 {
			kept = append(kept, sh)
		}
	}
	if sum != f.Cents {
		return domain.Expense{}, domain.ErrShares
	}
	return domain.Expense{Cents: f.Cents, Category: f.Category, Note: note, PaidBy: f.PaidBy, Day: day, Shares: kept}, nil
}

func (s *Budget) today() time.Time {
	y, m, d := s.now().In(s.loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func userIDs(shares []domain.Share) []uuid.UUID {
	out := make([]uuid.UUID, len(shares))
	for i, sh := range shares {
		out[i] = sh.UserID
	}
	return out
}
