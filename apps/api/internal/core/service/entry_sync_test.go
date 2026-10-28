package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

var (
	sharedEvents     = domain.SyncList{Kind: domain.KindEvent, Visibility: domain.VisibilityShared}
	privateEvents    = domain.SyncList{Kind: domain.KindEvent, Visibility: domain.VisibilityPrivate}
	sharedReminders  = domain.SyncList{Kind: domain.KindReminder, Visibility: domain.VisibilityShared}
	privateReminders = domain.SyncList{Kind: domain.KindReminder, Visibility: domain.VisibilityPrivate}
)

func TestSyncListsShowWhatTheActorMaySee(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	start := f.now.Add(time.Hour)
	shared := f.create(t, f.alice, domain.KindEvent, domain.EntryFields{Visibility: domain.VisibilityShared, Text: "Dinner", StartsAt: &start})
	f.create(t, f.alice, domain.KindEvent, domain.EntryFields{Visibility: domain.VisibilityPrivate, Text: "Gift shopping", StartsAt: &start})
	bobs := f.create(t, f.bob, domain.KindEvent, domain.EntryFields{Visibility: domain.VisibilityPrivate, Text: "Dentist", StartsAt: &start})
	f.create(t, f.bob, domain.KindReminder, domain.EntryFields{Visibility: domain.VisibilityShared, Text: "Bins"})

	names := func(u domain.User, l domain.SyncList) []string {
		t.Helper()
		es, err := f.entries.SyncEntries(ctx, u, f.household, l)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, e := range es {
			out = append(out, e.Text)
		}
		return out
	}
	if got := names(f.alice, sharedEvents); len(got) != 1 || got[0] != "Dinner" {
		t.Fatalf("alice's shared events: %v", got)
	}
	if got := names(f.alice, privateEvents); len(got) != 1 || got[0] != "Gift shopping" {
		t.Fatalf("alice's private events: %v", got)
	}
	if got := names(f.alice, sharedReminders); len(got) != 1 || got[0] != "Bins" {
		t.Fatalf("alice's shared reminders: %v", got)
	}
	if got := names(f.bob, privateEvents); len(got) != 1 || got[0] != "Dentist" {
		t.Fatalf("bob's private events: %v", got)
	}
	if _, err := f.entries.SyncEntries(ctx, f.eve, f.household, sharedEvents); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("eve listed: %v", err)
	}
	if _, err := f.entries.SyncEntry(ctx, f.alice, f.household, privateEvents, bobs.SyncName()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("alice read bob's private event: %v", err)
	}
	if err := f.entries.SyncDelete(ctx, f.alice, f.household, privateEvents, bobs.SyncName()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("alice deleted bob's private event: %v", err)
	}
	if e, err := f.entries.SyncEntry(ctx, f.bob, f.household, sharedEvents, shared.SyncName()); err != nil || e.ID != shared.ID {
		t.Fatalf("bob reads the shared event by its id: %+v, %v", e, err)
	}
	if _, err := f.entries.SyncEntry(ctx, f.bob, f.household, privateEvents, shared.SyncName()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("shared event in the private list: %v", err)
	}
}

func TestSyncPutMakesChangesAndMovesEntries(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	due := f.now.Add(2 * time.Hour)
	in := domain.SyncEntry{UID: "ABC-1", EntryFields: domain.EntryFields{Text: "Call mum", DueAt: &due, Repeat: domain.RepeatWeekly}}

	e, created, err := f.entries.SyncPut(ctx, f.alice, f.household, privateReminders, "ABC-1.ics", in)
	if err != nil || !created || e.ICalName != "ABC-1.ics" || e.ICalUID != "ABC-1" || e.Visibility != domain.VisibilityPrivate || e.AuthorID != f.alice.ID {
		t.Fatalf("create: %+v %v, %v", e, created, err)
	}
	if got, err := f.entries.SyncEntry(ctx, f.alice, f.household, privateReminders, "ABC-1.ics"); err != nil || got.ID != e.ID {
		t.Fatalf("read back: %+v, %v", got, err)
	}

	// Ticking a weekly reminder off moves it to next week.
	in.Done = true
	e, created, err = f.entries.SyncPut(ctx, f.alice, f.household, privateReminders, "ABC-1.ics", in)
	if err != nil || created || e.DoneAt != nil || !e.DueAt.Equal(due.AddDate(0, 0, 7)) {
		t.Fatalf("done: %+v %v, %v", e, created, err)
	}

	// Moving it to the shared list puts it there, and the delete from the old list leaves it.
	in.Done = false
	if e, _, err = f.entries.SyncPut(ctx, f.alice, f.household, sharedReminders, "ABC-1.ics", in); err != nil || e.Visibility != domain.VisibilityShared {
		t.Fatalf("move: %+v, %v", e, err)
	}
	if err := f.entries.SyncDelete(ctx, f.alice, f.household, privateReminders, "ABC-1.ics"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.entries.SyncEntry(ctx, f.bob, f.household, sharedReminders, "ABC-1.ics"); err != nil {
		t.Fatalf("bob sees the moved reminder: %v", err)
	}

	// Bob may change it, but only alice decides who sees it.
	in.Text = "Call mum and dad"
	if e, _, err = f.entries.SyncPut(ctx, f.bob, f.household, sharedReminders, "ABC-1.ics", in); err != nil || e.Text != in.Text {
		t.Fatalf("bob edits: %+v, %v", e, err)
	}
	if _, _, err := f.entries.SyncPut(ctx, f.bob, f.household, privateReminders, "ABC-1.ics", in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob made alice's reminder private: %v", err)
	}
	if _, _, err := f.entries.SyncPut(ctx, f.bob, f.household, sharedEvents, "ABC-1.ics", in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("an event under a reminder's name: %v", err)
	}
	if _, _, err := f.entries.SyncPut(ctx, f.alice, f.household, sharedReminders, "a/b.ics", in); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("name with a slash: %v", err)
	}

	if err := f.entries.SyncDelete(ctx, f.bob, f.household, sharedReminders, "ABC-1.ics"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.entries.SyncEntry(ctx, f.alice, f.household, sharedReminders, "ABC-1.ics"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
