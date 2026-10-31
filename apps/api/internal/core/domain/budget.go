package domain

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxExpenseCents is the largest amount one expense or settlement may have (100,000.00).
	MaxExpenseCents = 10_000_000
	MaxExpenseNote  = 200
	// MaxExpensesPerMonth is how many expenses one month lists.
	MaxExpensesPerMonth = 1000
)

var (
	ErrAmount = fmt.Errorf("%w: an amount must be 0.01-%d.00", ErrInvalid, MaxExpenseCents/100)
	// ErrShares means a share names someone outside the household, names someone twice, or the
	// shares don't add up to the amount.
	ErrShares = fmt.Errorf("%w: shares must name members once each and add up to the amount", ErrInvalid)
	// ErrPayer means the payer (or a settlement's receiver) is not a member, or a settlement pays oneself.
	ErrPayer    = fmt.Errorf("%w: who paid must be a member", ErrInvalid)
	ErrCategory = fmt.Errorf("%w: unknown category", ErrInvalid)
	// ErrBill means a bill on something other than a shared reminder, or with a wrong amount,
	// category or payer.
	ErrBill    = fmt.Errorf("%w: only shared reminders can be bills, of 0.01-%d.00 in a category, paid by a member", ErrInvalid, MaxExpenseCents/100)
	ErrReceipt = fmt.Errorf("%w: a receipt must be a JPEG of at most %d MB", ErrInvalid, MaxPhotoBytes>>20)
	// ErrSettlement means a settlement was to be changed like an expense; delete it and settle again.
	ErrSettlement = fmt.Errorf("%w: a settlement can't be changed, only deleted", ErrConflict)
)

// ExpenseKind tells what was spent from what was paid back.
type ExpenseKind string

const (
	KindExpense ExpenseKind = "expense"
	// KindSettlement is money one member gave another to settle up: the payer paid it, the
	// receiver has the one share.
	KindSettlement ExpenseKind = "settlement"
)

// ExpenseCategory is what money was spent on.
type ExpenseCategory string

const (
	CategoryGroceries ExpenseCategory = "groceries"
	CategoryDining    ExpenseCategory = "dining"
	CategoryHome      ExpenseCategory = "home"
	CategoryUtilities ExpenseCategory = "utilities"
	CategoryTransport ExpenseCategory = "transport"
	CategoryTravel    ExpenseCategory = "travel"
	CategoryFun       ExpenseCategory = "fun"
	CategoryHealth    ExpenseCategory = "health"
	CategoryGifts     ExpenseCategory = "gifts"
	CategoryOther     ExpenseCategory = "other"
)

var ExpenseCategories = []ExpenseCategory{
	CategoryGroceries, CategoryDining, CategoryHome, CategoryUtilities, CategoryTransport,
	CategoryTravel, CategoryFun, CategoryHealth, CategoryGifts, CategoryOther,
}

func (c ExpenseCategory) Valid() bool { return slices.Contains(ExpenseCategories, c) }

// Share is the part of an expense that is one member's to carry.
type Share struct {
	UserID uuid.UUID
	Cents  int64
}

// Expense is money one member paid, split into shares that add up to it. Every member sees and
// may change every expense: the budget is shared.
type Expense struct {
	ID          uuid.UUID
	HouseholdID uuid.UUID
	Kind        ExpenseKind
	Cents       int64
	Category    ExpenseCategory
	Note        string
	PaidBy      uuid.UUID
	// Day is the day it was spent, at midnight UTC.
	Day     time.Time
	Shares  []Share
	AddedBy *uuid.UUID
	// ReceiptAt is when a photo of the receipt was added; nil when there is none.
	ReceiptAt *time.Time
	Created   time.Time
	Updated   time.Time
}

// ExpenseFields is what a member writes about an expense. No shares means evenly between all members.
type ExpenseFields struct {
	Cents    int64
	Category ExpenseCategory
	Note     string
	PaidBy   uuid.UUID
	Day      time.Time
	Shares   []Share
}

// ExpenseMonth is one month of expenses, newest day first, what was spent in it by category, and
// the latest earlier month that has any (zero when there is none). Settlements count in neither
// Total nor Categories: paying back isn't spending.
type ExpenseMonth struct {
	Month      time.Time
	Expenses   []Expense
	Total      int64
	Categories []CategoryTotal
	Earlier    time.Time
}

// CategoryTotal is what a month spent in one category.
type CategoryTotal struct {
	Category ExpenseCategory
	Cents    int64
}

// Net is where one person stands: positive when the others owe them, negative when they owe.
type Net struct {
	UserID uuid.UUID
	Cents  int64
}

// Transfer is one payment that settles (part of) the balance.
type Transfer struct {
	From, To uuid.UUID
	Cents    int64
}

// Balance is where everyone stands and the fewest payments, roughly, that settle it.
type Balance struct {
	Members   []Net
	Transfers []Transfer
}

// SplitEvenly divides cents between the users as evenly as cents allow; the first ones in the
// list carry the odd cents.
func SplitEvenly(cents int64, users []uuid.UUID) []Share {
	if len(users) == 0 {
		return nil
	}
	n := int64(len(users))
	out := make([]Share, len(users))
	for i, u := range users {
		out[i] = Share{UserID: u, Cents: cents / n}
		if int64(i) < cents%n {
			out[i].Cents++
		}
	}
	return out
}

// Settle pairs who owes with who is owed, largest first, until everything is even. With two
// people that is one payment.
func Settle(nets []Net) []Transfer {
	var owe, owed []Net
	for _, n := range nets {
		switch {
		case n.Cents < 0:
			owe = append(owe, Net{UserID: n.UserID, Cents: -n.Cents})
		case n.Cents > 0:
			owed = append(owed, n)
		}
	}
	largest := func(a, b Net) int {
		return cmp.Or(cmp.Compare(b.Cents, a.Cents), cmp.Compare(a.UserID.String(), b.UserID.String()))
	}
	slices.SortFunc(owe, largest)
	slices.SortFunc(owed, largest)
	out := []Transfer{}
	for i, j := 0, 0; i < len(owe) && j < len(owed); {
		c := min(owe[i].Cents, owed[j].Cents)
		out = append(out, Transfer{From: owe[i].UserID, To: owed[j].UserID, Cents: c})
		owe[i].Cents -= c
		owed[j].Cents -= c
		if owe[i].Cents == 0 {
			i++
		}
		if owed[j].Cents == 0 {
			j++
		}
	}
	return out
}
