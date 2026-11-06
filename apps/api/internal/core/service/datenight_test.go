package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

type fakePlanner struct {
	brief domain.DateBrief
	ideas []domain.DateIdea
	err   error
}

func (f *fakePlanner) Ideas(_ context.Context, b domain.DateBrief, _ string, _ int) ([]domain.DateIdea, error) {
	f.brief = b
	return f.ideas, f.err
}

type fakeWeather struct{ err error }

func (f fakeWeather) Forecast(context.Context, string, time.Time) (domain.Forecast, error) {
	return domain.Forecast{Sky: domain.SkyRain, HighC: 8, LowC: 3, RainPercent: 90}, f.err
}

func (f fakeWeather) ForecastAt(context.Context, domain.Coords, time.Time) (domain.Forecast, error) {
	return domain.Forecast{Sky: domain.SkyClear, HighC: 20, LowC: 10}, f.err
}

type fakeNamer struct{}

func (fakeNamer) PlaceName(context.Context, domain.Coords, string) (string, error) {
	return "Hamburg", nil
}

func TestDateNightFromTheDevicesPosition(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice := newUser(t, store, "alice")
	m, _ := service.NewHousehold(store).Create(ctx, alice, "Home")
	planner := &fakePlanner{ideas: []domain.DateIdea{{Title: "Walk", Time: "19:00"}}}
	svc := service.NewDateNight(store, planner, fakeWeather{}, time.UTC).WithNamer(fakeNamer{})

	at := domain.Coords{Latitude: 53.55, Longitude: 10}
	res, err := svc.Ideas(ctx, alice, m.Household.ID, domain.DateQuery{Location: &at}, "en")
	if err != nil || res.Forecast == nil || res.Forecast.Sky != domain.SkyClear || res.Forecast.Place != "Hamburg" || planner.brief.Place != "Hamburg" {
		t.Fatalf("from position: %+v, %+v, %v", res.Forecast, planner.brief, err)
	}
	bad := domain.Coords{Latitude: 91}
	if _, err := svc.Ideas(ctx, alice, m.Household.ID, domain.DateQuery{Location: &bad}, "en"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("bad position: %v", err)
	}
}

func TestDateNight(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	households := service.NewHousehold(store)
	alice, eve := newUser(t, store, "alice"), newUser(t, store, "eve")
	m, _ := households.Create(ctx, alice, "Home")
	berlin, _ := time.LoadLocation("Europe/Berlin")

	planner := &fakePlanner{ideas: []domain.DateIdea{
		{Title: " Pasta at home ", Time: "19:30"},
		{Title: "", Time: "20:00"},
		{Title: "Late walk", Time: "garbled"},
		{Title: "Too early", Time: "08:00"},
	}}
	svc := service.NewDateNight(store, planner, fakeWeather{}, berlin)
	today := time.Now().In(berlin)
	day := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 2)

	if _, err := svc.Ideas(ctx, eve, m.Household.ID, domain.DateQuery{}, "en"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("non-member: want ErrNotFound, got %v", err)
	}
	res, err := svc.Ideas(ctx, alice, m.Household.ID, domain.DateQuery{Place: " Berlin ", Day: day, Mood: "cosy", Constraints: " no kitchen "}, "en")
	if err != nil || len(res.Ideas) != 3 || res.Forecast == nil || res.Forecast.Sky != domain.SkyRain {
		t.Fatalf("ideas: %+v, %v", res, err)
	}
	if planner.brief.Place != "Berlin" || planner.brief.Constraints != "no kitchen" || planner.brief.Budget != domain.DateEasy || planner.brief.Forecast == nil {
		t.Errorf("brief: %+v", planner.brief)
	}
	if got := res.Ideas[0]; got.Title != "Pasta at home" || got.StartsAt != time.Date(day.Year(), day.Month(), day.Day(), 19, 30, 0, 0, berlin) {
		t.Errorf("first idea: %+v", got)
	}
	if res.Ideas[1].Time != "19:00" || res.Ideas[2].Time != "19:00" {
		t.Errorf("unusable times should become 19:00: %+v", res.Ideas)
	}

	for name, q := range map[string]domain.DateQuery{
		"past":       {Day: day.AddDate(0, 0, -10)},
		"too far":    {Day: day.AddDate(0, 0, 30)},
		"budget":     {Budget: "lavish"},
		"long place": {Place: strings.Repeat("x", domain.MaxDatePlace+1)},
	} {
		if _, err := svc.Ideas(ctx, alice, m.Household.ID, q, "en"); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}

	// The weather failing only takes the weather away.
	noWeather := service.NewDateNight(store, planner, fakeWeather{err: domain.ErrUnavailable}, berlin)
	if res, err := noWeather.Ideas(ctx, alice, m.Household.ID, domain.DateQuery{Place: "Berlin"}, "en"); err != nil || res.Forecast != nil || len(res.Ideas) == 0 {
		t.Errorf("no weather: %+v, %v", res, err)
	}
	// No LLM, or nothing usable from it.
	if _, err := service.NewDateNight(store, nil, nil, berlin).Ideas(ctx, alice, m.Household.ID, domain.DateQuery{}, "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("no LLM: want ErrUnavailable, got %v", err)
	}
	planner.ideas = nil
	if _, err := svc.Ideas(ctx, alice, m.Household.ID, domain.DateQuery{}, "en"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("empty: want ErrUnavailable, got %v", err)
	}
}
