package storetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// Entries: others' private entries stay out of lists, the list order, done reminders drop off
// after a week, past events too, and each due time is claimed once, also by concurrent claimers.
func Entries(t *testing.T, s interface {
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
	now := time.Now().UTC().Truncate(time.Microsecond)
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	create := func(author domain.User, kind domain.EntryKind, f domain.EntryFields) domain.Entry {
		t.Helper()
		if f.Repeat == "" {
			f.Repeat = domain.RepeatNever
		}
		e, err := s.CreateEntry(ctx, domain.Entry{HouseholdID: h, AuthorID: author.ID, Kind: kind, EntryFields: f})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	note := create(alice, domain.KindNote, domain.EntryFields{Text: "shared note", Visibility: domain.VisibilityShared})
	secret := create(alice, domain.KindNote, domain.EntryFields{Text: "alice only", Visibility: domain.VisibilityPrivate})
	letter := create(bob, domain.KindNote, domain.EntryFields{Text: "for later", Visibility: domain.VisibilityReveal, RevealAt: at(time.Hour)})
	undated := create(alice, domain.KindReminder, domain.EntryFields{Text: "someday", Visibility: domain.VisibilityShared})
	soon := create(alice, domain.KindReminder, domain.EntryFields{Text: "soon", Visibility: domain.VisibilityShared, DueAt: at(-time.Minute), AssigneeID: &bob.ID})
	later := create(bob, domain.KindReminder, domain.EntryFields{Text: "later", Visibility: domain.VisibilityPrivate, DueAt: at(time.Hour), Repeat: domain.RepeatDaily})

	got, err := s.GetEntry(ctx, soon.ID)
	if err != nil || got.Text != "soon" || got.AssigneeID == nil || *got.AssigneeID != bob.ID || !got.DueAt.Equal(*soon.DueAt) || got.Kind != domain.KindReminder {
		t.Fatalf("get: %+v, %v", got, err)
	}
	if _, err := s.GetEntry(ctx, uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown entry: want ErrNotFound, got %v", err)
	}

	// A bill goes with its reminder and can be taken off again.
	rent := create(alice, domain.KindReminder, domain.EntryFields{Text: "Rent", Visibility: domain.VisibilityShared, Bill: &domain.Bill{Cents: 90000, Category: domain.CategoryHome, PaidBy: &alice.ID}})
	if got, err := s.GetEntry(ctx, rent.ID); err != nil || got.Bill == nil || got.Bill.Cents != 90000 || got.Bill.Category != domain.CategoryHome || got.Bill.PaidBy == nil || *got.Bill.PaidBy != alice.ID {
		t.Fatalf("bill: %+v, %v", got.Bill, err)
	}
	rent.Bill = &domain.Bill{Cents: 1, Category: domain.CategoryOther}
	if got, err := s.UpdateEntry(ctx, rent); err != nil || got.Bill == nil || got.Bill.PaidBy != nil || got.Bill.Cents != 1 {
		t.Fatalf("change bill: %+v, %v", got.Bill, err)
	}
	rent.Bill = nil
	if got, err := s.UpdateEntry(ctx, rent); err != nil || got.Bill != nil {
		t.Fatalf("drop bill: %+v, %v", got.Bill, err)
	}
	if err := s.DeleteEntry(ctx, rent.ID); err != nil {
		t.Fatal(err)
	}

	// An entry a calendar app made is found by the name the app gave it, in its household only.
	name := uuid.NewString() + ".ics"
	synced, err := s.CreateEntry(ctx, domain.Entry{HouseholdID: h, AuthorID: bob.ID, Kind: domain.KindReminder, ICalUID: "uid-1", ICalName: name,
		EntryFields: domain.EntryFields{Text: "from the phone", Visibility: domain.VisibilityShared, Repeat: domain.RepeatNever}})
	if err != nil || synced.ICalUID != "uid-1" || synced.ICalName != name {
		t.Fatalf("create synced: %+v, %v", synced, err)
	}
	if got, err := s.EntryByICalName(ctx, h, name); err != nil || got.ID != synced.ID || got.ICalUID != "uid-1" {
		t.Fatalf("by name: %+v, %v", got, err)
	}
	if _, err := s.EntryByICalName(ctx, uuid.New(), name); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("another household's name: %v", err)
	}
	renamed := uuid.NewString() + ".md"
	if err := s.RenameEntry(ctx, synced.ID, renamed); err != nil {
		t.Fatal(err)
	}
	if got, err := s.EntryByICalName(ctx, h, renamed); err != nil || got.ID != synced.ID {
		t.Fatalf("by new name: %+v, %v", got, err)
	}
	if err := s.RenameEntry(ctx, uuid.New(), renamed); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rename unknown entry: %v", err)
	}
	if err := s.DeleteEntry(ctx, synced.ID); err != nil {
		t.Fatal(err)
	}

	ids := func(es []domain.Entry) []uuid.UUID {
		out := make([]uuid.UUID, len(es))
		for i, e := range es {
			out[i] = e.ID
		}
		return out
	}
	expect := func(name string, viewer domain.User, kind domain.EntryKind, want ...domain.Entry) {
		t.Helper()
		list, err := s.ListEntries(ctx, h, viewer.ID, kind, now, 100)
		if err != nil {
			t.Fatal(err)
		}
		if g, w := ids(list), ids(want); len(g) != len(w) {
			t.Fatalf("%s: got %v, want %v", name, g, w)
		} else {
			for i := range g {
				if g[i] != w[i] {
					t.Fatalf("%s: got %v, want %v", name, g, w)
				}
			}
		}
	}
	// Notes newest first; reminders by due time, undated last.
	expect("bob's notes", bob, domain.KindNote, letter, note)
	expect("alice's notes", alice, domain.KindNote, letter, secret, note)
	expect("alice's reminders", alice, domain.KindReminder, soon, undated)
	expect("bob's everything", bob, "", letter, note, soon, later, undated)
	if list, _ := s.ListEntries(ctx, h, bob.ID, "", now, 2); len(list) != 2 {
		t.Fatalf("limit: got %d", len(list))
	}

	soon.DoneAt, soon.DoneBy = at(0), &bob.ID
	soon.Text = "soon, done"
	if got, err := s.UpdateEntry(ctx, soon); err != nil || got.Text != "soon, done" || got.DoneAt == nil || *got.DoneBy != bob.ID {
		t.Fatalf("update: %+v, %v", got, err)
	}
	expect("done after open", alice, domain.KindReminder, undated, soon)
	soon.DoneAt = at(-(domain.DoneReminderDays*24 + 1) * time.Hour)
	if _, err := s.UpdateEntry(ctx, soon); err != nil {
		t.Fatal(err)
	}
	expect("done a week ago drops off", alice, domain.KindReminder, undated)
	if err := s.DeleteEntry(ctx, undated.ID); err != nil {
		t.Fatal(err)
	}
	expect("deleted", alice, domain.KindReminder)

	// Photos in a note: kept in order, only found in their note, gone with the note.
	pic := domain.Photo{ContentType: "image/jpeg", Data: []byte{1, 2, 3}}
	first, err := s.AddEntryPhoto(ctx, note.ID, pic)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddEntryPhoto(ctx, note.ID, pic)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := s.EntryPhoto(ctx, note.ID, first); err != nil || p.ContentType != "image/jpeg" || len(p.Data) != 3 {
		t.Fatalf("photo: %+v, %v", p, err)
	}
	if _, err := s.EntryPhoto(ctx, letter.ID, first); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo of another note: %v", err)
	}
	if got, err := s.EntryPhotoIDs(ctx, note.ID); err != nil || len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("photo ids: %v, %v", got, err)
	}
	if err := s.DeleteEntryPhotos(ctx, note.ID, []uuid.UUID{first}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.EntryPhotoIDs(ctx, note.ID); len(got) != 1 || got[0] != second {
		t.Fatalf("after deleting one: %v", got)
	}
	if err := s.DeleteEntry(ctx, note.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EntryPhoto(ctx, note.ID, second); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("photo of a deleted note: %v", err)
	}

	// Claiming: only open, due, not yet notified reminders, once each.
	due := create(alice, domain.KindReminder, domain.EntryFields{Text: "due", Visibility: domain.VisibilityShared, DueAt: at(-time.Minute)})
	notified := create(alice, domain.KindReminder, domain.EntryFields{Text: "notified", Visibility: domain.VisibilityShared, DueAt: at(-time.Hour)})
	notified.NotifiedAt = at(-time.Minute)
	if _, err := s.UpdateEntry(ctx, notified); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var claimed []domain.Entry
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			got, err := s.ClaimDue(ctx, now, 10)
			if err != nil {
				t.Errorf("claim: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			claimed = append(claimed, got...)
		})
	}
	wg.Wait()
	if len(claimed) != 1 || claimed[0].ID != due.ID || claimed[0].NotifiedAt == nil || !claimed[0].NotifiedAt.Equal(now) {
		t.Fatalf("claimed %+v", claimed)
	}
	// Events: listed from since on unless they repeat, by start; their alerts are claimed too.
	alert := 15
	past := create(alice, domain.KindEvent, domain.EntryFields{Text: "past", Visibility: domain.VisibilityShared, StartsAt: at(-3 * time.Hour), EndsAt: at(-2 * time.Hour)})
	weekly := create(alice, domain.KindEvent, domain.EntryFields{Text: "weekly", Visibility: domain.VisibilityShared, StartsAt: at(-50 * time.Hour), Repeat: domain.RepeatWeekly})
	party := create(alice, domain.KindEvent, domain.EntryFields{
		Text: "party", Visibility: domain.VisibilityShared, StartsAt: at(time.Hour), EndsAt: at(5 * time.Hour), AllDay: false,
		AlertMinutes: &alert, DueAt: at(-time.Second),
	})
	expect("events from now on", alice, domain.KindEvent, weekly, party)
	if got, err := s.GetEntry(ctx, party.ID); err != nil || *got.AlertMinutes != 15 || !got.EndsAt.Equal(*party.EndsAt) || got.AllDay {
		t.Fatalf("event fields: %+v, %v", got, err)
	}
	if list, _ := s.ListEntries(ctx, h, alice.ID, domain.KindEvent, now.Add(-4*time.Hour), 10); len(list) != 3 || list[1].ID != past.ID {
		t.Fatalf("events since four hours ago: %+v", list)
	}
	if got, err := s.ClaimDue(ctx, now, 10); err != nil || len(got) != 1 || got[0].ID != party.ID {
		t.Fatalf("an event's alert is claimed: %+v, %v", got, err)
	}

	later.NotifiedAt = at(-24 * time.Hour)
	if _, err := s.UpdateEntry(ctx, later); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ClaimDue(ctx, now.Add(2*time.Hour), 10); err != nil || len(got) != 1 || got[0].ID != later.ID {
		t.Fatalf("a new due time is claimed again: %+v, %v", got, err)
	}
}

// CalendarFeeds: one link per member and household; a new one replaces the old.
func CalendarFeeds(t *testing.T, s interface {
	Store
	port.CalendarFeedRepository
}) {
	ctx := context.Background()
	alice := user(t, s, "Alice")
	m, err := s.Create(ctx, "Home", alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	f := domain.CalendarFeed{UserID: alice.ID, HouseholdID: m.Household.ID}
	if has, err := s.HasCalendarFeed(ctx, f); err != nil || has {
		t.Fatalf("none yet: %v, %v", has, err)
	}
	old, token := hash(uuid.NewString()), hash(uuid.NewString())
	for _, h := range [][]byte{old, token} {
		if err := s.SaveCalendarFeed(ctx, h, f); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CalendarFeed(ctx, old); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("replaced link: want ErrNotFound, got %v", err)
	}
	if got, err := s.CalendarFeed(ctx, token); err != nil || got != f {
		t.Fatalf("got %+v, %v", got, err)
	}
	if has, _ := s.HasCalendarFeed(ctx, f); !has {
		t.Fatal("has a feed")
	}
	if err := s.DeleteCalendarFeed(ctx, f); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CalendarFeed(ctx, token); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: want ErrNotFound, got %v", err)
	}
}
