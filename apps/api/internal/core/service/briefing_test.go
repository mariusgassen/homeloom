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
	return domain.Forecast{Place: "Berlin", Sky: domain.SkyRain, Condition: domain.CondDrizzle, HighC: 14.2, LowC: 7.9}, nil
}

func (briefingWeather) ForecastAt(context.Context, domain.Coords, time.Time) (domain.Forecast, error) {
	return domain.Forecast{Sky: domain.SkyClear, Condition: domain.CondClear, HighC: 21.4, LowC: 12}, nil
}

func TestBriefingInTheMorning(t *testing.T) {
	ctx := context.Background()
	f := newChoreFixture(t)
	entries := service.NewEntry(f.store, f.store, f.store, f.push, time.UTC)
	entries.SetClock(func() time.Time { return f.now })
	f.now = time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC)
	b := service.NewBriefing(f.store, entries, f.chores, f.store, f.push, time.UTC).WithWeather(briefingWeather{}).WithLocations(f.store)
	b.SetClock(func() time.Time { return f.now })

	if err := f.store.SetPlace(ctx, f.household, "Berlin"); err != nil {
		t.Fatal(err)
	}
	// Bob shares where he is; Alice's was shared too long ago to count.
	if err := f.store.SaveLocation(ctx, f.bob.ID, domain.MemberLocation{Coords: domain.Coords{Latitude: 53.55, Longitude: 10}, Place: "Hamburg", UpdatedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SaveLocation(ctx, f.alice.ID, domain.MemberLocation{Coords: domain.Coords{Latitude: 48.14, Longitude: 11.58}, Place: "Munich", UpdatedAt: f.now.AddDate(0, 0, -4)}); err != nil {
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
	if bodies["alice"] != " · 📅 10:00 Dentist" || bodies["bob"] != " · 📅 10:00 Dentist · ✅ Bins" {
		t.Fatalf("briefed %q", bodies)
	}
	for _, p := range f.pusher.Sent() {
		w, _ := p.Notification.Args[3].(domain.Phrase)
		if strings.HasSuffix(p.Subscription.Endpoint, "bob") {
			if w.Key != "weather.line" || w.Args[3] != "Hamburg" || w.Args[5].(domain.Phrase).Key != "weather.clear" || w.Args[9] != "21" {
				t.Fatalf("bob's weather %+v", p.Notification.Args[3])
			}
			continue
		}
		if w.Key != "weather.line" || w.Args[3] != "Berlin" || w.Args[5].(domain.Phrase).Key != "weather.drizzle" || w.Args[7] != "8" || w.Args[9] != "14" {
			t.Fatalf("weather %+v", p.Notification.Args[3])
		}
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
