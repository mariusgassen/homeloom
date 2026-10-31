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

type choreFixture struct {
	store           *memory.Store
	pusher          *memory.Pusher
	push            *service.Push
	moments         *service.Moments
	chores          *service.Chores
	alice, bob, eve domain.User
	household       uuid.UUID
	now             time.Time
}

// newChoreFixture starts on Wednesday, 28 October 2026, with Alice and Bob at home.
func newChoreFixture(t *testing.T) *choreFixture {
	t.Helper()
	ctx := context.Background()
	f := &choreFixture{store: memory.NewStore(), now: time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)}
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
	f.pusher = &memory.Pusher{}
	f.push = service.NewPush(f.store, f.pusher)
	f.moments = service.NewMoments(f.store, f.store, f.store, f.push, time.UTC)
	f.moments.SetClock(func() time.Time { return f.now })
	f.chores = service.NewChores(f.store, f.store, f.store, f.moments, f.push, time.UTC)
	f.chores.SetClock(func() time.Time { return f.now })
	return f
}

func (f *choreFixture) create(t *testing.T, u domain.User, fields domain.ChoreFields) domain.Chore {
	t.Helper()
	c, err := f.chores.Create(context.Background(), u, f.household, fields)
	if err != nil {
		t.Fatalf("create %+v: %v", fields, err)
	}
	return c
}

func (f *choreFixture) complete(t *testing.T, u domain.User, c domain.Chore) domain.ChoreCompletion {
	t.Helper()
	d, err := f.chores.Complete(context.Background(), u, f.household, c.ID)
	if err != nil {
		t.Fatalf("%s completes %q: %v", u.Name, c.Title, err)
	}
	return d
}

func (f *choreFixture) chore(t *testing.T, id uuid.UUID) (domain.Chore, bool) {
	t.Helper()
	cs, err := f.chores.List(context.Background(), f.alice, f.household)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(cs, func(c domain.Chore) bool { return c.ID == id })
	if i < 0 {
		return domain.Chore{}, false
	}
	return cs[i], true
}

// sent returns who got which notification keys, and forgets them.
func (f *choreFixture) sent() map[string][]string {
	f.push.Wait()
	out := map[string][]string{}
	for _, p := range f.pusher.Sent() {
		name := strings.TrimPrefix(p.Subscription.Endpoint, "https://web.push.apple.com/")
		out[name] = append(out[name], p.Notification.Key)
	}
	f.pusher = &memory.Pusher{}
	f.push = service.NewPush(f.store, f.pusher)
	f.chores = service.NewChores(f.store, f.store, f.store, f.moments, f.push, time.UTC)
	f.chores.SetClock(func() time.Time { return f.now })
	return out
}

func TestChoresRotateAndMoveOn(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	vacuum := f.create(t, f.alice, domain.ChoreFields{Title: " Vacuum ", Points: 2, Repeat: domain.RepeatWeekly, Rotate: true})
	if vacuum.Title != "Vacuum" || !vacuum.DueOn.Equal(day("2026-10-28")) || vacuum.AssigneeID == nil || *vacuum.AssigneeID != f.alice.ID {
		t.Fatalf("a rotating chore starts today with whoever set it up: %+v", vacuum)
	}

	d := f.complete(t, f.alice, vacuum)
	if d.UserID != f.alice.ID || d.Points != 2 || d.Title != "Vacuum" || !d.Day.Equal(day("2026-10-28")) {
		t.Fatalf("completion: %+v", d)
	}
	c, _ := f.chore(t, vacuum.ID)
	if !c.DueOn.Equal(day("2026-11-04")) || *c.AssigneeID != f.bob.ID {
		t.Fatalf("after Alice: due %v for %v; want 4 Nov, Bob's turn", c.DueOn, *c.AssigneeID)
	}
	if _, err := f.chores.Complete(ctx, f.bob, f.household, vacuum.ID); !errors.Is(err, domain.ErrChoreDone) {
		t.Fatalf("twice on one day: want ErrChoreDone, got %v", err)
	}

	// Done early by Alice again, it still moves past its due day, and back to Bob.
	f.now = f.now.AddDate(0, 0, 1)
	f.complete(t, f.alice, vacuum)
	c, _ = f.chore(t, vacuum.ID)
	if !c.DueOn.Equal(day("2026-11-11")) || *c.AssigneeID != f.bob.ID {
		t.Fatalf("done early: due %v for %v; want 11 Nov, still Bob's turn", c.DueOn, *c.AssigneeID)
	}

	// Without rotation the turn stays; a one-off leaves the list once done.
	once := f.create(t, f.bob, domain.ChoreFields{Title: "Descale the kettle", Points: 1, AssigneeID: &f.bob.ID})
	f.complete(t, f.alice, once)
	if _, listed := f.chore(t, once.ID); listed {
		t.Fatal("a one-off chore is still listed after it was done")
	}
	if _, err := f.chores.Complete(ctx, f.bob, f.household, once.ID); !errors.Is(err, domain.ErrChoreDone) {
		t.Fatalf("a one-off again: want ErrChoreDone, got %v", err)
	}

	if _, err := f.chores.Complete(ctx, f.eve, f.household, vacuum.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	for _, bad := range []domain.ChoreFields{
		{Title: "", Points: 1},
		{Title: "Dust", Points: 4},
		{Title: "Dust", Points: 1, Repeat: "hourly"},
		{Title: "Dust", Points: 1, AssigneeID: &f.eve.ID},
	} {
		if _, err := f.chores.Create(ctx, f.alice, f.household, bad); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("create %+v: want ErrInvalid, got %v", bad, err)
		}
	}
}

func TestChoresUndo(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	bins := f.create(t, f.alice, domain.ChoreFields{Title: "Bins", Points: 1, Repeat: domain.RepeatWeekly, Rotate: true})
	d := f.complete(t, f.alice, bins)

	if err := f.chores.Undo(ctx, f.bob, f.household, d.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("someone else's: want ErrForbidden, got %v", err)
	}
	if err := f.chores.Undo(ctx, f.alice, f.household, d.ID); err != nil {
		t.Fatal(err)
	}
	c, _ := f.chore(t, bins.ID)
	if !c.DueOn.Equal(bins.DueOn) || *c.AssigneeID != f.alice.ID {
		t.Fatalf("undone: %+v; want it back as before", c)
	}
	if h, err := f.chores.History(ctx, f.alice, f.household); err != nil || len(h) != 0 {
		t.Fatalf("history after undo: %+v, %v", h, err)
	}

	// Ticking off again the same day works once undone; only the latest can be undone.
	first := f.complete(t, f.alice, bins)
	f.now = f.now.AddDate(0, 0, 1)
	f.complete(t, f.bob, bins)
	if err := f.chores.Undo(ctx, f.alice, f.household, first.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("not the latest: want ErrConflict, got %v", err)
	}
}

func TestChoresFairness(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	small := f.create(t, f.alice, domain.ChoreFields{Title: "Plants", Points: 1, Repeat: domain.RepeatDaily})
	big := f.create(t, f.alice, domain.ChoreFields{Title: "Bathroom", Points: 3, Repeat: domain.RepeatDaily})
	at := func(d time.Time, u domain.User, c domain.Chore) {
		f.now = d.Add(10 * time.Hour)
		f.complete(t, u, c)
	}
	at(day("2026-09-30"), f.alice, big)   // last month
	at(day("2026-10-20"), f.alice, small) // last week
	at(day("2026-10-27"), f.bob, big)
	at(day("2026-10-28"), f.alice, small)
	at(day("2026-10-28"), f.alice, big)

	fair, err := f.chores.Fairness(ctx, f.bob, f.household)
	if err != nil {
		t.Fatal(err)
	}
	want := func(name string, tally domain.ChoreTally, from time.Time, alice, aliceCount, bob, bobCount int) {
		t.Helper()
		got := map[uuid.UUID]domain.ChoreShare{}
		for _, m := range tally.Members {
			got[m.UserID] = m
		}
		if !tally.From.Equal(from) || len(tally.Members) != 2 ||
			got[f.alice.ID] != (domain.ChoreShare{UserID: f.alice.ID, Points: alice, Count: aliceCount}) ||
			got[f.bob.ID] != (domain.ChoreShare{UserID: f.bob.ID, Points: bob, Count: bobCount}) {
			t.Fatalf("%s: %+v", name, tally)
		}
	}
	want("week", fair.Week, day("2026-10-26"), 4, 2, 3, 1)
	want("month", fair.Month, day("2026-10-01"), 5, 3, 3, 1)

	// On Sunday 1 November the week still began in October; the month has just begun.
	f.now = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	if fair, err = f.chores.Fairness(ctx, f.alice, f.household); err != nil {
		t.Fatal(err)
	}
	want("week across months", fair.Week, day("2026-10-26"), 4, 2, 3, 1)
	want("new month", fair.Month, day("2026-11-01"), 0, 0, 0, 0)

	// Deleting a chore keeps what was done.
	if err := f.chores.Delete(ctx, f.alice, f.household, big.ID); err != nil {
		t.Fatal(err)
	}
	if fair, err = f.chores.Fairness(ctx, f.alice, f.household); err != nil {
		t.Fatal(err)
	}
	want("after delete", fair.Week, day("2026-10-26"), 4, 2, 3, 1)
	if _, err := f.chores.Fairness(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
}

func TestChoresThanksGoToGratitude(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	dishes := f.create(t, f.alice, domain.ChoreFields{Title: "Dishes", Points: 1, Repeat: domain.RepeatDaily})
	d := f.complete(t, f.alice, dishes)

	if _, _, err := f.chores.Thank(ctx, f.alice, f.household, d.ID, ""); !errors.Is(err, domain.ErrOwnChore) {
		t.Fatalf("thanking yourself: want ErrOwnChore, got %v", err)
	}
	if _, _, err := f.chores.Thank(ctx, f.bob, f.household, d.ID, strings.Repeat("x", domain.MaxGratitudeText+1)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("a long line: want ErrInvalid, got %v", err)
	}
	done, line, err := f.chores.Thank(ctx, f.bob, f.household, d.ID, "Alice did the dishes")
	if err != nil || !slices.Equal(done.ThankedBy, []uuid.UUID{f.bob.ID}) || line == nil ||
		line.Kind != domain.KindGratitude || line.AuthorID != f.bob.ID || line.Text != "Alice did the dishes" || line.Visibility != domain.VisibilityShared {
		t.Fatalf("thanks: %+v, %+v, %v", done, line, err)
	}
	if got := f.sent(); !slices.Equal(got["alice"], []string{"push.chore_thanks"}) || len(got["bob"]) != 0 {
		t.Fatalf("pushed %v; want Alice to hear thanks", got)
	}

	// Again, nobody hears it twice, and Bob's line for today stays as it is.
	d2 := f.complete(t, f.alice, f.create(t, f.alice, domain.ChoreFields{Title: "Laundry", Points: 2}))
	if _, line, err = f.chores.Thank(ctx, f.bob, f.household, d.ID, "again"); err != nil || line != nil {
		t.Fatalf("thanks again: %+v, %v", line, err)
	}
	if _, line, err = f.chores.Thank(ctx, f.bob, f.household, d2.ID, "Laundry too"); err != nil || line != nil {
		t.Fatalf("a second line on one day: %+v, %v; want none", line, err)
	}
	if got := f.sent(); len(got["alice"]) != 1 {
		t.Fatalf("pushed %v; want only the thanks for the laundry", got)
	}
	month, _ := f.moments.Month(ctx, f.bob, f.household, time.Time{})
	if len(month.Moments) != 1 || month.Moments[0].Text != "Alice did the dishes" {
		t.Fatalf("gratitude wall: %+v", month.Moments)
	}
	if h, _ := f.chores.History(ctx, f.alice, f.household); len(h) != 2 || !slices.Equal(h[1].ThankedBy, []uuid.UUID{f.bob.ID}) {
		t.Fatalf("history keeps the thanks: %+v", h)
	}
}

func TestChoresNudgeInTheEvening(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	f.create(t, f.alice, domain.ChoreFields{Title: "Plants", Points: 1, Repeat: domain.RepeatWeekly, AssigneeID: &f.alice.ID})
	f.create(t, f.alice, domain.ChoreFields{Title: "Bins", Points: 1, Repeat: domain.RepeatWeekly, DueOn: day("2026-10-27")})
	f.create(t, f.alice, domain.ChoreFields{Title: "Windows", Points: 3, AssigneeID: &f.bob.ID, DueOn: day("2026-10-29")})
	done := f.create(t, f.alice, domain.ChoreFields{Title: "Dishes", Points: 1, Repeat: domain.RepeatDaily})
	f.complete(t, f.bob, done)

	f.now = time.Date(2026, 10, 28, 17, 59, 0, 0, time.UTC)
	if err := f.chores.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.sent(); len(got) != 0 {
		t.Fatalf("before 18:00: %v", got)
	}
	f.now = time.Date(2026, 10, 28, 18, 0, 0, 0, time.UTC)
	if err := f.chores.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	bodies := map[string]string{}
	for _, p := range f.pusher.Sent() {
		bodies[strings.TrimPrefix(p.Subscription.Endpoint, "https://web.push.apple.com/")] = p.Notification.Args[1].(string)
	}
	if bodies["alice"] != "Bins, Plants" || bodies["bob"] != "Bins" || len(bodies) != 2 {
		t.Fatalf("nudged %v; want Alice for hers and anyone's, Bob for anyone's", bodies)
	}
	f.sent()
	if err := f.chores.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.sent(); len(got) != 0 {
		t.Fatalf("a second nudge the same day: %v", got)
	}
}
