package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type entryFixture struct {
	store           *memory.Store
	pusher          *memory.Pusher
	push            *service.Push
	entries         *service.Entry
	alice, bob, eve domain.User
	household       uuid.UUID
	now             time.Time
}

func newEntryFixture(t *testing.T) *entryFixture {
	t.Helper()
	ctx := context.Background()
	f := &entryFixture{store: memory.NewStore(), pusher: &memory.Pusher{}, now: time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)}
	f.push = service.NewPush(f.store, f.pusher)
	f.entries = service.NewEntry(f.store, f.store, f.store, f.push, time.UTC)
	f.entries.SetClock(func() time.Time { return f.now })
	f.alice, f.bob, f.eve = newUser(t, f.store, "alice"), newUser(t, f.store, "bob"), newUser(t, f.store, "eve")
	m, err := service.NewHousehold(f.store).Create(ctx, f.alice, "Home")
	if err != nil {
		t.Fatal(err)
	}
	f.household = m.Household.ID
	f.store.AddMember(f.household, f.bob.ID, domain.RoleMember)
	for _, u := range []domain.User{f.alice, f.bob} {
		sub := domain.PushSubscription{UserID: u.ID, Endpoint: "https://web.push.apple.com/" + u.Name, Lang: "en"}
		if err := f.store.SavePushSubscription(ctx, sub); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *entryFixture) create(t *testing.T, u domain.User, kind domain.EntryKind, fields domain.EntryFields) domain.Entry {
	t.Helper()
	e, err := f.entries.Create(context.Background(), u, f.household, kind, fields)
	if err != nil {
		t.Fatalf("create %+v: %v", fields, err)
	}
	return e
}

// notified returns who got which notification since the last call.
func (f *entryFixture) notified(t *testing.T) map[string][]string {
	t.Helper()
	if _, err := f.entries.NotifyDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	out := map[string][]string{}
	for _, p := range f.pusher.Sent() {
		name := strings.TrimPrefix(p.Subscription.Endpoint, "https://web.push.apple.com/")
		out[name] = append(out[name], p.Notification.Args[1].(string))
	}
	// Notifications go out in the background, in no particular order.
	for _, texts := range out {
		slices.Sort(texts)
	}
	f.pusher = &memory.Pusher{}
	f.push = service.NewPush(f.store, f.pusher)
	f.entries = service.NewEntry(f.store, f.store, f.store, f.push, time.UTC)
	f.entries.SetClock(func() time.Time { return f.now })
	return out
}

func at(t time.Time) *time.Time { return &t }

func TestEntryVisibility(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	private := f.create(t, f.alice, domain.KindNote, domain.EntryFields{Text: "Only mine", Visibility: domain.VisibilityPrivate})
	shared := f.create(t, f.alice, domain.KindNote, domain.EntryFields{Text: " Groceries? ", Visibility: domain.VisibilityShared})
	letter := f.create(t, f.alice, domain.KindNote, domain.EntryFields{
		Text: "Happy anniversary", Visibility: domain.VisibilityReveal, RevealAt: at(f.now.Add(24 * time.Hour)),
	})
	if shared.Text != "Groceries?" {
		t.Fatalf("text is trimmed: %q", shared.Text)
	}

	list, err := f.entries.List(ctx, f.bob, f.household, "")
	if err != nil || len(list) != 2 {
		t.Fatalf("bob sees the shared note and the letter only: %+v, %v", list, err)
	}
	if _, err := f.entries.Get(ctx, f.bob, private.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("private to bob: want ErrNotFound, got %v", err)
	}
	if _, err := f.entries.Update(ctx, f.bob, private.ID, domain.EntryFields{Text: "x", Visibility: domain.VisibilityPrivate}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update private as bob: want ErrNotFound, got %v", err)
	}
	if mine, _ := f.entries.List(ctx, f.alice, f.household, domain.KindNote); len(mine) != 3 {
		t.Fatalf("alice sees all three: %+v", mine)
	}

	if e, _ := f.entries.Get(ctx, f.bob, letter.ID); !e.Sealed || e.Text != "" {
		t.Fatalf("letter is sealed for bob: %+v", e)
	}
	if e, _ := f.entries.Get(ctx, f.alice, letter.ID); e.Sealed || e.Text == "" {
		t.Fatalf("alice reads her own letter: %+v", e)
	}
	if _, err := f.entries.Update(ctx, f.bob, letter.ID, domain.EntryFields{Text: "peek", Visibility: domain.VisibilityShared}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob changes the letter: want ErrForbidden, got %v", err)
	}
	if err := f.entries.Delete(ctx, f.bob, letter.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob deletes the letter: want ErrForbidden, got %v", err)
	}
	f.now = f.now.Add(25 * time.Hour)
	if e, _ := f.entries.Get(ctx, f.bob, letter.ID); e.Sealed || e.Text != "Happy anniversary" {
		t.Fatalf("letter opens after revealAt: %+v", e)
	}

	if e, err := f.entries.Update(ctx, f.bob, shared.ID, domain.EntryFields{Text: "Milk", Visibility: domain.VisibilityShared}); err != nil || e.Text != "Milk" {
		t.Fatalf("bob edits a shared note: %+v, %v", e, err)
	}
	if _, err := f.entries.Update(ctx, f.bob, shared.ID, domain.EntryFields{Text: "Milk", Visibility: domain.VisibilityPrivate}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("bob makes alice's note private: want ErrForbidden, got %v", err)
	}
	if err := f.entries.Delete(ctx, f.bob, shared.ID); err != nil {
		t.Fatalf("bob deletes a shared note: %v", err)
	}

	if _, err := f.entries.List(ctx, f.eve, f.household, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member list: want ErrNotFound, got %v", err)
	}
	if _, err := f.entries.Get(ctx, f.eve, letter.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member get: want ErrNotFound, got %v", err)
	}
	if _, err := f.entries.Create(ctx, f.eve, f.household, domain.KindNote, domain.EntryFields{Text: "hi", Visibility: domain.VisibilityShared}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member create: want ErrNotFound, got %v", err)
	}
}

func TestEntryValidation(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	due := at(f.now.Add(time.Hour))
	bob, stranger := f.bob.ID, f.eve.ID
	cases := []struct {
		name   string
		kind   domain.EntryKind
		fields domain.EntryFields
		want   error
	}{
		{"empty", domain.KindNote, domain.EntryFields{Text: "  ", Visibility: domain.VisibilityShared}, domain.TextError{Max: domain.MaxNoteText}},
		{"long reminder", domain.KindReminder, domain.EntryFields{Text: strings.Repeat("x", domain.MaxReminderText+1), Visibility: domain.VisibilityShared}, domain.TextError{Max: domain.MaxReminderText}},
		{"note with due time", domain.KindNote, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared, DueAt: due}, domain.ErrEntryFields},
		{"repeat without due time", domain.KindReminder, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared, Repeat: domain.RepeatDaily}, domain.ErrEntryFields},
		{"reminder revealed later", domain.KindReminder, domain.EntryFields{Text: "x", Visibility: domain.VisibilityReveal, RevealAt: due}, domain.ErrRevealAt},
		{"reveal without date", domain.KindNote, domain.EntryFields{Text: "x", Visibility: domain.VisibilityReveal}, domain.ErrRevealAt},
		{"assignee on private", domain.KindReminder, domain.EntryFields{Text: "x", Visibility: domain.VisibilityPrivate, AssigneeID: &bob}, domain.ErrAssignee},
		{"assignee not a member", domain.KindReminder, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared, AssigneeID: &stranger}, domain.ErrAssignee},
		{"unknown visibility", domain.KindNote, domain.EntryFields{Text: "x", Visibility: "public"}, domain.ErrInvalid},
	}
	for _, c := range cases {
		_, err := f.entries.Create(ctx, f.alice, f.household, c.kind, c.fields)
		if !errors.Is(err, c.want) || !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
	e := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "Bins", Visibility: domain.VisibilityShared, AssigneeID: &bob, DueAt: due})
	if e.Repeat != domain.RepeatNever || *e.AssigneeID != bob {
		t.Fatalf("defaults: %+v", e)
	}
	n := f.create(t, f.alice, domain.KindNote, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared, RevealAt: due})
	if n.RevealAt != nil {
		t.Fatalf("revealAt is dropped unless the note opens later: %+v", n)
	}
}

func TestRemindersTickOff(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	once := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "Call the landlord", Visibility: domain.VisibilityShared})
	e, err := f.entries.SetDone(ctx, f.bob, once.ID, true)
	if err != nil || e.DoneAt == nil || *e.DoneBy != f.bob.ID {
		t.Fatalf("bob ticks it off: %+v, %v", e, err)
	}
	if e, err = f.entries.SetDone(ctx, f.alice, once.ID, false); err != nil || e.DoneAt != nil || e.DoneBy != nil {
		t.Fatalf("reopen: %+v, %v", e, err)
	}

	weekly := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{
		Text: "Water the plants", Visibility: domain.VisibilityShared, DueAt: at(f.now.Add(-time.Hour)), Repeat: domain.RepeatWeekly,
	})
	e, err = f.entries.SetDone(ctx, f.alice, weekly.ID, true)
	if err != nil || e.DoneAt != nil || !e.DueAt.Equal(weekly.DueAt.AddDate(0, 0, 7)) {
		t.Fatalf("a repeating reminder moves on a week: %+v, %v", e, err)
	}

	note := f.create(t, f.alice, domain.KindNote, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared})
	if _, err := f.entries.SetDone(ctx, f.alice, note.ID, true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("tick off a note: want ErrConflict, got %v", err)
	}
	if _, err := f.entries.SetDone(ctx, f.eve, once.ID, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
}

func TestDueRemindersNotifyTheRightPeople(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	in := func(d time.Duration) *time.Time { return at(f.now.Add(d)) }
	bob := f.bob.ID
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "past", Visibility: domain.VisibilityShared, DueAt: in(-time.Minute)})
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "both", Visibility: domain.VisibilityShared, DueAt: in(time.Hour)})
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "for bob", Visibility: domain.VisibilityShared, DueAt: in(time.Hour), AssigneeID: &bob})
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "mine", Visibility: domain.VisibilityPrivate, DueAt: in(time.Hour)})
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "later", Visibility: domain.VisibilityShared, DueAt: in(3 * time.Hour)})
	daily := f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "daily", Visibility: domain.VisibilityPrivate, DueAt: in(time.Hour), Repeat: domain.RepeatDaily})

	if got := f.notified(t); len(got) != 0 {
		t.Fatalf("nothing is due yet; a time set in the past doesn't ring: %v", got)
	}
	f.now = f.now.Add(2 * time.Hour)
	got := f.notified(t)
	if strings.Join(got["alice"], ",") != "both,daily,mine" || strings.Join(got["bob"], ",") != "both,for bob" {
		t.Fatalf("got %v", got)
	}
	if got := f.notified(t); len(got) != 0 {
		t.Fatalf("each due time rings once: %v", got)
	}

	if _, err := f.entries.SetDone(ctx, f.alice, daily.ID, true); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(24 * time.Hour)
	if got := f.notified(t); strings.Join(got["alice"], ",") != "daily,later" || len(got["bob"]) != 1 {
		t.Fatalf("the next day's time rings again: %v", got)
	}
}

func TestRepeatKeepsTheTimeOfDay(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// Summer time ends in Berlin on 25 October 2026.
	due := time.Date(2026, 10, 24, 8, 0, 0, 0, berlin)
	next := domain.RepeatDaily.Next(due, due.Add(time.Minute), berlin)
	if want := time.Date(2026, 10, 25, 8, 0, 0, 0, berlin); !next.Equal(want) {
		t.Fatalf("daily across the clock change: got %v, want %v", next, want)
	}
	if next := domain.RepeatWeekly.Next(due, due.AddDate(0, 0, 20), berlin); !next.Equal(time.Date(2026, 11, 14, 8, 0, 0, 0, berlin)) {
		t.Fatalf("weekly skips to the first time after now: %v", next)
	}
	end := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	if next := domain.RepeatMonthly.Next(end, time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), time.UTC); !next.Equal(time.Date(2026, 3, 31, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("monthly keeps the 31st after a short month: %v", next)
	}
	if next := domain.RepeatYearly.Next(end, end, time.UTC); !next.Equal(end.AddDate(1, 0, 0)) {
		t.Fatalf("yearly: %v", next)
	}
}

func TestEvents(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	alert := 30
	in := func(d time.Duration) *time.Time { return at(f.now.Add(d)) }

	for name, fields := range map[string]domain.EntryFields{
		"no start":       {Text: "x", Visibility: domain.VisibilityShared},
		"ends too early": {Text: "x", Visibility: domain.VisibilityShared, StartsAt: in(time.Hour), EndsAt: in(time.Minute)},
		"alert too soon": {Text: "x", Visibility: domain.VisibilityShared, StartsAt: in(time.Hour), AlertMinutes: new(domain.MaxAlertMinutes + 1)},
	} {
		if _, err := f.entries.Create(ctx, f.alice, f.household, domain.KindEvent, fields); !errors.Is(err, domain.ErrEventTime) {
			t.Errorf("%s: want ErrEventTime, got %v", name, err)
		}
	}
	if _, err := f.entries.Create(ctx, f.alice, f.household, domain.KindReminder, domain.EntryFields{Text: "x", Visibility: domain.VisibilityShared, StartsAt: in(0)}); !errors.Is(err, domain.ErrEntryFields) {
		t.Errorf("reminder with a start: want ErrEntryFields, got %v", err)
	}

	dinner := f.create(t, f.alice, domain.KindEvent, domain.EntryFields{
		Text: "Dinner at Jo's", Visibility: domain.VisibilityShared, StartsAt: in(2 * time.Hour), EndsAt: in(4 * time.Hour), AlertMinutes: &alert,
	})
	if !dinner.DueAt.Equal(f.now.Add(90 * time.Minute)) {
		t.Fatalf("the alert goes out 30 minutes before: %v", dinner.DueAt)
	}
	trip := f.create(t, f.alice, domain.KindEvent, domain.EntryFields{
		Text: "Ghent", Visibility: domain.VisibilityShared, AllDay: true, StartsAt: in(48 * time.Hour), EndsAt: in(72 * time.Hour),
	})
	if trip.StartsAt.Hour() != 0 || trip.EndsAt.Hour() != 0 || trip.DueAt != nil {
		t.Fatalf("all-day events start at midnight and have no alert: %+v", trip)
	}
	f.create(t, f.alice, domain.KindEvent, domain.EntryFields{Text: "Yesterday", Visibility: domain.VisibilityShared, StartsAt: in(-48 * time.Hour)})
	yoga := f.create(t, f.bob, domain.KindEvent, domain.EntryFields{
		Text: "Yoga", Visibility: domain.VisibilityShared, StartsAt: in(-7*24*time.Hour + time.Hour), Repeat: domain.RepeatWeekly, AlertMinutes: &alert,
	})
	if !yoga.DueAt.Equal(f.now.Add(30 * time.Minute)) {
		t.Fatalf("a repeating event's alert is for its next time: %v", yoga.DueAt)
	}

	list, err := f.entries.List(ctx, f.alice, f.household, domain.KindEvent)
	if err != nil || len(list) != 3 || list[0].ID != yoga.ID || list[1].ID != dinner.ID || list[2].ID != trip.ID {
		t.Fatalf("events from today by start, past ones gone: %+v, %v", list, err)
	}
	if !list[0].StartsAt.Equal(f.now.Add(time.Hour)) {
		t.Fatalf("a repeating event is listed at its next time: %v", list[0].StartsAt)
	}

	f.now = f.now.Add(time.Hour)
	if got := f.notified(t); strings.Join(got["alice"], ",") != "Yoga" || strings.Join(got["bob"], ",") != "Yoga" {
		t.Fatalf("yoga's alert: %v", got)
	}
	f.now = f.now.Add(time.Hour)
	if got := f.notified(t); strings.Join(got["alice"], ",") != "Dinner at Jo's" {
		t.Fatalf("dinner's alert: %v", got)
	}
	f.now = f.now.Add(7 * 24 * time.Hour)
	if got := f.notified(t); strings.Join(got["bob"], ",") != "Yoga" {
		t.Fatalf("yoga's alert a week later: %v", got)
	}
}

func TestCalendarFeed(t *testing.T) {
	ctx := context.Background()
	f := newEntryFixture(t)
	if _, err := f.entries.CreateCalendarFeed(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member feed: want ErrNotFound, got %v", err)
	}
	first, err := f.entries.CreateCalendarFeed(ctx, f.bob, f.household)
	if err != nil {
		t.Fatal(err)
	}
	token, _ := f.entries.CreateCalendarFeed(ctx, f.bob, f.household)
	if _, _, err := f.entries.CalendarFeed(ctx, first); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("a new link replaces the old one: %v", err)
	}
	if active, _ := f.entries.HasCalendarFeed(ctx, f.bob, f.household); !active {
		t.Fatal("bob has a link")
	}

	due := at(f.now.Add(time.Hour))
	f.create(t, f.alice, domain.KindEvent, domain.EntryFields{Text: "shared event", Visibility: domain.VisibilityShared, StartsAt: due})
	f.create(t, f.alice, domain.KindEvent, domain.EntryFields{Text: "alice's event", Visibility: domain.VisibilityPrivate, StartsAt: due})
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "dated", Visibility: domain.VisibilityShared, DueAt: due})
	f.create(t, f.alice, domain.KindReminder, domain.EntryFields{Text: "undated", Visibility: domain.VisibilityShared})
	f.create(t, f.alice, domain.KindNote, domain.EntryFields{Text: "note", Visibility: domain.VisibilityShared})
	es, _, err := f.entries.CalendarFeed(ctx, token)
	if err != nil || len(es) != 2 || es[0].Text != "shared event" || es[1].Text != "dated" {
		t.Fatalf("bob's feed holds shared events and dated reminders: %+v, %v", es, err)
	}

	if err := f.entries.DeleteCalendarFeed(ctx, f.bob, f.household); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.entries.CalendarFeed(ctx, token); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after turning it off: want ErrNotFound, got %v", err)
	}
	if _, _, err := f.entries.CalendarFeed(ctx, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("empty token: want ErrNotFound, got %v", err)
	}
}
