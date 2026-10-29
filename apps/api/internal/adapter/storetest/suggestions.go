package storetest

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// suggestionTexts returns a function that takes a suggestions call's results and returns the
// texts, failing the test on an error.
func suggestionTexts(t *testing.T) func([]domain.TitleSuggestion, error) []string {
	return func(ss []domain.TitleSuggestion, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(ss))
		for i, s := range ss {
			out[i] = s.Text
		}
		return out
	}
}

// ReminderSuggestions: past reminder titles, grouped regardless of case, most used first, then
// most recent; open ones and others' private ones left out.
func ReminderSuggestions(t *testing.T, s interface {
	Store
	port.EntryRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Household.ID
	join(t, s, h, alice, bob)
	done := time.Now().Add(-30 * 24 * time.Hour)
	add := func(author domain.User, kind domain.EntryKind, text string, v domain.Visibility, isDone bool) {
		t.Helper()
		e, err := s.CreateEntry(ctx, domain.Entry{HouseholdID: h, AuthorID: author.ID, Kind: kind, EntryFields: domain.EntryFields{Text: text, Visibility: v, Repeat: domain.RepeatNever}})
		if err != nil {
			t.Fatal(err)
		}
		if isDone {
			e.DoneAt = &done
			if _, err := s.UpdateEntry(ctx, e); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(alice, domain.KindReminder, "water plants", domain.VisibilityShared, true)
	add(bob, domain.KindReminder, "Water plants", domain.VisibilityShared, true)
	add(alice, domain.KindReminder, "call mum", domain.VisibilityShared, true)
	add(alice, domain.KindReminder, "bins", domain.VisibilityShared, true)
	add(alice, domain.KindReminder, "bins", domain.VisibilityShared, false)
	add(bob, domain.KindReminder, "bob's secret", domain.VisibilityPrivate, true)
	add(alice, domain.KindNote, "a note", domain.VisibilityShared, false)
	add(alice, domain.KindReminder, "pay rent", domain.VisibilityPrivate, true)

	got := suggestionTexts(t)(s.ReminderSuggestions(ctx, h, alice.ID, 10))
	if want := []string{"Water plants", "pay rent", "call mum"}; !slices.Equal(got, want) {
		t.Fatalf("alice: got %v, want %v", got, want)
	}
	ss, _ := s.ReminderSuggestions(ctx, h, alice.ID, 1)
	if len(ss) != 1 || ss[0].Uses != 2 {
		t.Fatalf("limit: %+v", ss)
	}
	got = suggestionTexts(t)(s.ReminderSuggestions(ctx, h, bob.ID, 10))
	if want := []string{"Water plants", "bob's secret", "call mum"}; !slices.Equal(got, want) {
		t.Fatalf("bob: got %v, want %v", got, want)
	}
}

// ShoppingSuggestions: what was on the list, kept after clearing, most used first, then most
// recent; what's open on the list left out.
func ShoppingSuggestions(t *testing.T, s interface {
	Store
	port.ShoppingRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create(ctx, "Elsewhere", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hh := home.Household.ID
	add := func(text string) domain.ShoppingItem {
		t.Helper()
		items, err := s.AddShoppingItems(ctx, []domain.ShoppingItem{{HouseholdID: hh, Text: text}})
		if err != nil {
			t.Fatal(err)
		}
		return items[0]
	}
	for _, text := range []string{"milk", "Milk", "bread", "eggs"} {
		it := add(text)
		if _, err := s.SetShoppingItemDone(ctx, hh, it.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	add("tea")
	if _, err := s.AddShoppingItems(ctx, []domain.ShoppingItem{{HouseholdID: other.Household.ID, Text: "coffee"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearDoneShoppingItems(ctx, hh); err != nil {
		t.Fatal(err)
	}

	got := suggestionTexts(t)(s.ShoppingSuggestions(ctx, hh, 10))
	if want := []string{"Milk", "eggs", "bread"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	ss, _ := s.ShoppingSuggestions(ctx, hh, 1)
	if len(ss) != 1 || ss[0].Uses != 2 {
		t.Fatalf("limit: %+v", ss)
	}
	got = suggestionTexts(t)(s.ShoppingSuggestions(ctx, other.Household.ID, 10))
	if len(got) != 0 {
		t.Fatalf("other household: %v", got)
	}
}
