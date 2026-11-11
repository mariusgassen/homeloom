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

// Gifts: birthdays are kept per member, and an idea is only ever read, changed or deleted with
// its author's id, so no one else's call reaches it.
func Gifts(t *testing.T, s interface {
	Store
	port.GiftRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	home, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	hh := home.Household.ID
	join(t, s, hh, alice, bob)

	if err := s.SetBirthday(ctx, bob.ID, &domain.Birthday{Month: time.March, Day: 9}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetBirthday(ctx, bob.ID, &domain.Birthday{Month: time.April, Day: 1}); err != nil {
		t.Fatal(err)
	}
	if bs, err := s.Birthdays(ctx, hh); err != nil || len(bs) != 1 || bs[bob.ID] != (domain.Birthday{Month: time.April, Day: 1}) {
		t.Fatalf("birthdays: %v, %v", bs, err)
	}
	if err := s.SetBirthday(ctx, bob.ID, &domain.Birthday{Month: time.April, Day: 1, Year: 1988}); err != nil {
		t.Fatal(err)
	}
	if bs, err := s.Birthdays(ctx, hh); err != nil || bs[bob.ID] != (domain.Birthday{Month: time.April, Day: 1, Year: 1988}) {
		t.Fatalf("birthday with year = %v, %v", bs, err)
	}
	if err := s.SetBirthday(ctx, bob.ID, nil); err != nil {
		t.Fatal(err)
	}
	if bs, _ := s.Birthdays(ctx, hh); len(bs) != 0 {
		t.Fatalf("removed: %v", bs)
	}

	a, err := s.AddGiftIdea(ctx, domain.GiftIdea{HouseholdID: hh, AuthorID: alice.ID, ForID: bob.ID, Text: "Shoes"})
	if err != nil || a.ID.String() == "" || a.GivenAt != nil {
		t.Fatalf("add: %+v, %v", a, err)
	}
	b, err := s.AddGiftIdea(ctx, domain.GiftIdea{HouseholdID: hh, AuthorID: alice.ID, ForID: bob.ID, Text: "Chalk"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.GiftIdeas(ctx, hh, bob.ID, 10); err != nil || len(got) != 0 {
		t.Fatalf("bob sees %+v, %v", got, err)
	}
	if _, err := s.SetGiftIdeaGiven(ctx, hh, bob.ID, a.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob ticks Alice's idea: %v", err)
	}
	if err := s.DeleteGiftIdea(ctx, hh, bob.ID, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob deletes Alice's idea: %v", err)
	}
	if i, err := s.SetGiftIdeaGiven(ctx, hh, alice.ID, a.ID, true); err != nil || i.GivenAt == nil {
		t.Fatalf("given: %+v, %v", i, err)
	}
	// Open ideas come first, newest first; given ones last.
	got, err := s.GiftIdeas(ctx, hh, alice.ID, 10)
	if err != nil || len(got) != 2 || got[0].ID != b.ID || got[1].ID != a.ID {
		t.Fatalf("order: %+v, %v", got, err)
	}
	if i, err := s.SetGiftIdeaGiven(ctx, hh, alice.ID, a.ID, false); err != nil || i.GivenAt != nil {
		t.Fatalf("undo: %+v, %v", i, err)
	}
	if err := s.DeleteGiftIdea(ctx, hh, alice.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GiftIdeas(ctx, hh, alice.ID, 10); len(got) != 1 {
		t.Fatalf("after delete: %+v", got)
	}

	// Occasions belong to the household; deleting one keeps the ideas for it, without it.
	o, err := s.AddOccasion(ctx, domain.Occasion{HouseholdID: hh, Name: "Christmas", Date: domain.Birthday{Month: time.December, Day: 25}})
	if err != nil || o.ID.String() == "" {
		t.Fatalf("add occasion: %+v, %v", o, err)
	}
	if os, err := s.Occasions(ctx, hh); err != nil || len(os) != 1 || os[0].Name != "Christmas" || os[0].Date.Day != 25 {
		t.Fatalf("occasions: %+v, %v", os, err)
	}
	c, err := s.AddGiftIdea(ctx, domain.GiftIdea{HouseholdID: hh, AuthorID: alice.ID, ForID: bob.ID, OccasionID: &o.ID, Text: "Scarf"})
	if err != nil || c.OccasionID == nil || *c.OccasionID != o.ID {
		t.Fatalf("idea for occasion: %+v, %v", c, err)
	}
	if err := s.DeleteOccasion(ctx, uuid.New(), o.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("delete from another household: %v", err)
	}
	if err := s.DeleteOccasion(ctx, hh, o.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GiftIdeas(ctx, hh, alice.ID, 10); len(got) != 2 || slices.ContainsFunc(got, func(i domain.GiftIdea) bool { return i.OccasionID != nil }) {
		t.Fatalf("after deleting the occasion: %+v", got)
	}
}
