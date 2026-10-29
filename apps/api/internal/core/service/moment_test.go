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

type momentFixture struct {
	store           *memory.Store
	pusher          *memory.Pusher
	push            *service.Push
	moments         *service.Moments
	alice, bob, eve domain.User
	household       uuid.UUID
	now             time.Time
	chance          float64
}

func newMomentFixture(t *testing.T) *momentFixture {
	t.Helper()
	ctx := context.Background()
	f := &momentFixture{store: memory.NewStore(), now: time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC), chance: 0.9}
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
	f.reset()
	return f
}

func (f *momentFixture) reset() {
	f.pusher = &memory.Pusher{}
	f.push = service.NewPush(f.store, f.pusher)
	f.moments = service.NewMoments(f.store, f.store, f.store, f.push, time.UTC)
	f.moments.SetClock(func() time.Time { return f.now })
	f.moments.SetChance(func() float64 { return f.chance })
}

func (f *momentFixture) create(t *testing.T, u domain.User, kind domain.MomentKind, fields domain.MomentFields) domain.Moment {
	t.Helper()
	m, err := f.moments.Create(context.Background(), u, f.household, kind, fields)
	if err != nil {
		t.Fatalf("create %+v: %v", fields, err)
	}
	return m
}

// notified runs the daily notifications at the fixture's time and returns who got which keys.
func (f *momentFixture) notified(t *testing.T) map[string][]string {
	t.Helper()
	if err := f.moments.NotifyDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	out := map[string][]string{}
	for _, p := range f.pusher.Sent() {
		name := strings.TrimPrefix(p.Subscription.Endpoint, "https://web.push.apple.com/")
		out[name] = append(out[name], p.Notification.Key)
	}
	for _, keys := range out {
		slices.Sort(keys)
	}
	f.reset()
	return out
}

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestMomentRules(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	shared := domain.MomentFields{Text: " Lake day ", Visibility: domain.VisibilityShared, Day: day("2026-08-02")}

	m := f.create(t, f.alice, domain.KindMoment, shared)
	if m.Text != "Lake day" || !m.Day.Equal(day("2026-08-02")) {
		t.Fatalf("created: %+v", m)
	}
	if _, err := f.moments.Create(ctx, f.eve, f.household, domain.KindMoment, shared); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	future := shared
	future.Day = day("2026-10-29")
	if _, err := f.moments.Create(ctx, f.alice, f.household, domain.KindMoment, future); !errors.Is(err, domain.ErrMomentDay) {
		t.Fatalf("tomorrow: want ErrMomentDay, got %v", err)
	}
	if _, err := f.moments.Update(ctx, f.bob, m.ID, shared); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("someone else's moment: want ErrForbidden, got %v", err)
	}
	if err := f.moments.Delete(ctx, f.bob, m.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("delete someone else's: want ErrForbidden, got %v", err)
	}

	mine := f.create(t, f.bob, domain.KindMoment, domain.MomentFields{Text: "Just me", Visibility: domain.VisibilityPrivate})
	if _, err := f.moments.Get(ctx, f.alice, mine.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("someone else's private moment: want ErrNotFound, got %v", err)
	}
	if !mine.Day.Equal(day("2026-10-28")) {
		t.Fatalf("a moment without a day is today: %v", mine.Day)
	}

	line := f.create(t, f.alice, domain.KindGratitude, domain.MomentFields{Text: "Coffee", Visibility: domain.VisibilityShared, Day: day("2026-01-01")})
	again := f.create(t, f.alice, domain.KindGratitude, domain.MomentFields{Text: "The walk", Visibility: domain.VisibilityShared})
	if again.ID != line.ID || again.Text != "The walk" || !again.Day.Equal(day("2026-10-28")) {
		t.Fatalf("a second line today replaces the first: %+v, %+v", line, again)
	}
	long := domain.MomentFields{Text: strings.Repeat("x", domain.MaxGratitudeText+1), Visibility: domain.VisibilityShared}
	var te domain.TextError
	if _, err := f.moments.Create(ctx, f.alice, f.household, domain.KindGratitude, long); !errors.As(err, &te) || te.Max != domain.MaxGratitudeText {
		t.Fatalf("long line: %v", err)
	}
	reveal := domain.MomentFields{Text: "Later", Visibility: domain.VisibilityReveal, RevealAt: &f.now}
	if _, err := f.moments.Create(ctx, f.alice, f.household, domain.KindGratitude, reveal); !errors.Is(err, domain.ErrRevealAt) {
		t.Fatalf("a line can't open later: %v", err)
	}
	if _, err := f.moments.SetPhoto(ctx, f.alice, line.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF}}); !errors.Is(err, domain.ErrPhoto) {
		t.Fatalf("a line has no photo: %v", err)
	}

	mm, err := f.moments.Month(ctx, f.alice, f.household, time.Time{})
	if err != nil || !mm.Month.Equal(day("2026-10-01")) || len(mm.Moments) != 1 || mm.Moments[0].ID != line.ID || !mm.Earlier.Equal(day("2026-08-01")) {
		t.Fatalf("this month: %+v, %v", mm, err)
	}
	if mm, err := f.moments.Month(ctx, f.bob, f.household, time.Time{}); err != nil || len(mm.Moments) != 2 {
		t.Fatalf("bob sees his own private moment too: %+v, %v", mm, err)
	}
	if mm, err := f.moments.Month(ctx, f.alice, f.household, day("2026-08-15")); err != nil || len(mm.Moments) != 1 || !mm.Earlier.IsZero() {
		t.Fatalf("august: %+v, %v", mm, err)
	}
}

func TestLetterStaysSealedUntilItOpens(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	opens := f.now.Add(48 * time.Hour)
	letter := f.create(t, f.alice, domain.KindMoment, domain.MomentFields{Text: "Dear Bob", Visibility: domain.VisibilityReveal, RevealAt: &opens})
	if _, err := f.moments.SetPhoto(ctx, f.alice, letter.ID, domain.Photo{ContentType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 1}}); err != nil {
		t.Fatal(err)
	}

	got, err := f.moments.Get(ctx, f.bob, letter.ID)
	if err != nil || !got.Sealed || got.Text != "" || got.PhotoAt != nil {
		t.Fatalf("sealed for bob: %+v, %v", got, err)
	}
	if _, err := f.moments.Photo(ctx, f.bob, letter.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sealed photo: want ErrNotFound, got %v", err)
	}
	if got, err := f.moments.Get(ctx, f.alice, letter.ID); err != nil || got.Sealed || got.Text != "Dear Bob" {
		t.Fatalf("open for its author: %+v, %v", got, err)
	}
	f.now = opens
	f.reset()
	if p, err := f.moments.Photo(ctx, f.bob, letter.ID); err != nil || len(p.Data) != 4 {
		t.Fatalf("opened photo: %+v, %v", p, err)
	}
}

func TestResurfacedAndDailyNotifications(t *testing.T) {
	ctx := context.Background()
	f := newMomentFixture(t)
	// A year ago today, and a private one of bob's that alice must never get back.
	f.create(t, f.alice, domain.KindMoment, domain.MomentFields{Text: "First apartment", Visibility: domain.VisibilityShared, Day: day("2025-10-28")})
	f.create(t, f.bob, domain.KindMoment, domain.MomentFields{Text: "Secret", Visibility: domain.VisibilityPrivate, Day: day("2024-10-28")})

	// Lines are always written for today, so write bob's on earlier days.
	f.now = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	f.reset()
	f.create(t, f.bob, domain.KindGratitude, domain.MomentFields{Text: "Her laugh", Visibility: domain.VisibilityShared})
	f.now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	f.reset()
	f.create(t, f.bob, domain.KindGratitude, domain.MomentFields{Text: "Too new", Visibility: domain.VisibilityShared})
	f.now = time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	f.reset()

	onThisDay, line, err := f.moments.Resurfaced(ctx, f.alice, f.household)
	if err != nil || len(onThisDay) != 1 || onThisDay[0].Text != "First apartment" || line == nil || line.Text != "Her laugh" {
		t.Fatalf("resurfaced: %+v, %+v, %v", onThisDay, line, err)
	}
	if onThisDay, _, err := f.moments.Resurfaced(ctx, f.bob, f.household); err != nil || len(onThisDay) != 2 {
		t.Fatalf("bob gets his private one back too: %+v, %v", onThisDay, err)
	}
	if _, _, err := f.moments.Resurfaced(ctx, f.eve, f.household); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}

	if got := f.notified(t); len(got) != 0 {
		t.Fatalf("before 9 nothing goes out: %v", got)
	}
	f.now = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	if got := f.notified(t); len(got["alice"]) != 1 || len(got["bob"]) != 1 || got["alice"][0] != "push.on_this_day" {
		t.Fatalf("on this day: %v", got)
	}
	if got := f.notified(t); len(got) != 0 {
		t.Fatalf("once a day: %v", got)
	}

	f.now = time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	if got := f.notified(t); len(got) != 0 {
		t.Fatalf("no past line when chance says no: %v", got)
	}
	f.now = time.Date(2026, 10, 29, 20, 0, 0, 0, time.UTC)
	f.chance = 0
	got := f.notified(t)
	if !slices.Equal(got["alice"], []string{"push.gratitude"}) || !slices.Equal(got["bob"], []string{"push.gratitude_own"}) {
		t.Fatalf("a past line, by bob: %v", got)
	}

	f.now = time.Date(2026, 11, 1, 19, 0, 0, 0, time.UTC)
	f.chance = 0.9
	got = f.notified(t)
	if !slices.Equal(got["alice"], []string{"push.last_month"}) || !slices.Equal(got["bob"], []string{"push.last_month"}) {
		t.Fatalf("last month on the 1st: %v", got)
	}
}
