package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type briefingWeather struct{}

func (briefingWeather) Forecast(context.Context, string, time.Time) (domain.Forecast, error) {
	return domain.Forecast{Sky: domain.SkyRain, HighC: 14.2, LowC: 7.9}, nil
}

func TestBriefingInTheMorning(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	entries := service.NewEntry(f.store, f.store, f.store, f.push, time.UTC)
	entries.SetClock(func() time.Time { return f.now })
	f.now = time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC)
	b := service.NewBriefing(f.store, entries, f.chores, f.store, f.push, time.UTC).WithWeather(briefingWeather{})
	b.SetClock(func() time.Time { return f.now })

	if err := f.store.SetPlace(ctx, f.household, "Berlin"); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	later := start.AddDate(0, 0, 2)
	for _, e := range []struct {
		text string
		at   *time.Time
	}{{"Dentist", &start}, {"Trip", &later}} {
		if _, err := entries.Create(ctx, f.alice, f.household, domain.KindEvent, domain.EntryFields{Visibility: domain.VisibilityShared, Text: e.text, StartsAt: e.at}); err != nil {
			t.Fatal(err)
		}
	}
	f.create(t, f.alice, domain.ChoreFields{Title: "Bins", Points: 1, AssigneeID: &f.bob.ID, DueOn: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)})

	if err := b.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	if got := f.pusher.Sent(); len(got) != 0 {
		t.Fatalf("before 7:00: %v", got)
	}
	f.now = time.Date(2026, 10, 28, 7, 0, 0, 0, time.UTC)
	if err := b.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	bodies := map[string]string{}
	for _, p := range f.pusher.Sent() {
		bodies[strings.TrimPrefix(p.Subscription.Endpoint, "https://web.push.apple.com/")] = p.Notification.Args[1].(string)
	}
	if bodies["alice"] != "🌧️ 8–14° · 📅 10:00 Dentist" || bodies["bob"] != "🌧️ 8–14° · 📅 10:00 Dentist · ✅ Bins" {
		t.Fatalf("briefed %q", bodies)
	}
	n := len(f.pusher.Sent())
	if err := b.NotifyDaily(ctx); err != nil {
		t.Fatal(err)
	}
	f.push.Wait()
	if len(f.pusher.Sent()) != n {
		t.Fatal("a second briefing the same day")
	}
}
