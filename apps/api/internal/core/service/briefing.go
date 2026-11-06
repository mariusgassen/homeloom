package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

const (
	// briefingHour is the local hour from which the morning briefing goes out.
	briefingHour = 7
	// briefingNames is how many plans or due things the briefing names before it says "…".
	briefingNames = 3
)

// Briefing sends everyone one push each morning: the weather, today's plans, and what is due. The
// push is made of numbers and names only, so it reads the same in every language.
type Briefing struct {
	households port.HouseholdRepository
	entries    *Entry
	chores     *Chores
	jobs       port.JobRepository
	notify     notifier
	// weather is nil without a place to forecast; the briefing then has no weather.
	weather port.Weather
	// locations holds where members' devices are; nil: everyone gets the household's place.
	locations port.LocationRepository
	loc       *time.Location
	now       func() time.Time
}

// NewBriefing counts days in loc, the household's time zone.
func NewBriefing(households port.HouseholdRepository, entries *Entry, chores *Chores, jobs port.JobRepository, notify notifier, loc *time.Location) *Briefing {
	return &Briefing{households: households, entries: entries, chores: chores, jobs: jobs, notify: notify, loc: loc, now: time.Now}
}

// WithWeather adds the day's forecast to the briefing, for households that chose a place.
func (s *Briefing) WithWeather(w port.Weather) *Briefing {
	s.weather = w
	return s
}

// WithLocations gives members who share where their device is the weather from there.
func (s *Briefing) WithLocations(r port.LocationRepository) *Briefing {
	s.locations = r
	return s
}

// NotifyDaily sends the briefing once the morning has come. It is meant to run every minute or
// so; it goes out once per day, also with several API instances.
func (s *Briefing) NotifyDaily(ctx context.Context) error {
	now := s.now().In(s.loc)
	if now.Hour() < briefingHour {
		return nil
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	claimed, err := s.jobs.ClaimJobRun(ctx, "briefing", day)
	if err != nil || !claimed {
		return err
	}
	ids, err := s.households.HouseholdIDs(ctx)
	if err != nil {
		return err
	}
	for _, hh := range ids {
		if err := s.send(ctx, hh, now); err != nil {
			slog.WarnContext(ctx, "briefing: send", "err", err)
		}
	}
	return nil
}

func (s *Briefing) send(ctx context.Context, householdID uuid.UUID, now time.Time) error {
	weather, err := s.forecast(ctx, householdID, now)
	if err != nil {
		slog.WarnContext(ctx, "briefing: weather", "err", err)
	}
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return err
	}
	var where map[uuid.UUID]domain.MemberLocation
	if s.locations != nil && s.weather != nil {
		ids := make([]uuid.UUID, len(members))
		for i, m := range members {
			ids[i] = m.User.ID
		}
		if where, err = s.locations.Locations(ctx, ids); err != nil {
			slog.WarnContext(ctx, "briefing: locations", "err", err)
		}
	}
	chores, err := s.chores.chores.Chores(ctx, householdID, domain.MaxChores)
	if err != nil {
		return err
	}
	end := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, s.loc)
	for _, m := range members {
		entries, err := s.entries.List(ctx, m.User, householdID, "")
		if err != nil {
			slog.WarnContext(ctx, "briefing: entries", "err", err)
			continue
		}
		var plans, due []string
		for _, e := range entries {
			switch {
			case e.Sealed:
			// List already left out events that ended before today and moved repeating ones to their next time.
			case e.Kind == domain.KindEvent && e.StartsAt != nil && e.StartsAt.Before(end):
				plans = append(plans, s.planText(e))
			case e.Kind == domain.KindReminder && e.DoneAt == nil && e.DueAt != nil && e.DueAt.Before(end) && (e.AssigneeID == nil || *e.AssigneeID == m.User.ID):
				due = append(due, e.Text)
			}
		}
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		for _, c := range chores {
			if !c.DueOn.After(today) && (c.AssigneeID == nil || *c.AssigneeID == m.User.ID) {
				due = append(due, c.Title)
			}
		}
		weather := weather
		if l, ok := where[m.User.ID]; ok && l.Fresh(now) {
			if f, err := s.weather.ForecastAt(ctx, l.Coords, today); err != nil {
				// Their own weather is out of reach; the household's still is a fair guess.
				slog.WarnContext(ctx, "briefing: weather at member", "err", err)
			} else {
				f.Place = l.Place
				p := weatherPhrase(f)
				weather = &p
			}
		}
		var parts []string
		if len(plans) > 0 {
			parts = append(parts, "📅 "+names(plans))
		}
		if len(due) > 0 {
			parts = append(parts, "✅ "+names(due))
		}
		if len(parts) == 0 && weather == nil {
			continue
		}
		day := strings.Join(parts, " · ")
		if weather != nil && day != "" {
			day = " · " + day
		}
		args := []any{"day", day, "weather", ""}
		if weather != nil {
			args[3] = *weather
		}
		s.notify.Notify(ctx, []uuid.UUID{m.User.ID}, domain.Notification{
			Key: "push.briefing", Args: args, URL: "/", Tag: "briefing",
		})
	}
	return nil
}

// forecast is the weather line for the household's place, nil without a place or forecast.
func (s *Briefing) forecast(ctx context.Context, householdID uuid.UUID, now time.Time) (*domain.Phrase, error) {
	if s.weather == nil {
		return nil, nil
	}
	h, err := s.households.Get(ctx, householdID)
	if err != nil || h.Place == "" {
		return nil, err
	}
	f, err := s.weather.Forecast(ctx, h.Place, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		return nil, err
	}
	if f.Place == "" {
		f.Place = h.Place
	}
	p := weatherPhrase(f)
	return &p, nil
}

// planText is an event's title with its start time, unless it lasts all day.
func (s *Briefing) planText(e domain.Entry) string {
	if e.AllDay {
		return e.Text
	}
	return e.StartsAt.In(s.loc).Format("15:04") + " " + e.Text
}

// names joins the first few, then "…".
func names(xs []string) string {
	if len(xs) > briefingNames {
		xs = append(xs[:briefingNames:briefingNames], "…")
	}
	return strings.Join(xs, ", ")
}

// weatherPhrase is the place, the weather in words with an emoji for the sky, and the day's low to
// high, like "🌧️ Berlin: light rain, 8–14°".
func weatherPhrase(f domain.Forecast) domain.Phrase {
	key := "weather.line"
	if f.Place == "" {
		key = "weather.lineNoPlace"
	}
	return domain.Phrase{Key: key, Args: []any{
		"emoji", skyEmoji(f.Sky), "place", f.Place, "condition", domain.Phrase{Key: "weather." + string(f.Condition)},
		"low", fmt.Sprintf("%.0f", f.LowC), "high", fmt.Sprintf("%.0f", f.HighC),
	}}
}

func skyEmoji(s domain.Sky) string {
	switch s {
	case domain.SkyClear:
		return "☀️"
	case domain.SkyCloudy:
		return "☁️"
	case domain.SkyFog:
		return "🌫️"
	case domain.SkyRain:
		return "🌧️"
	case domain.SkySnow:
		return "❄️"
	case domain.SkyStorm:
		return "⛈️"
	}
	return "🌦️"
}
