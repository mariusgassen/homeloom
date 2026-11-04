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
	weather port.Forecaster
	loc     *time.Location
	now     func() time.Time
}

// NewBriefing counts days in loc, the household's time zone.
func NewBriefing(households port.HouseholdRepository, entries *Entry, chores *Chores, jobs port.JobRepository, notify notifier, loc *time.Location) *Briefing {
	return &Briefing{households: households, entries: entries, chores: chores, jobs: jobs, notify: notify, loc: loc, now: time.Now}
}

// WithWeather adds the day's forecast to the briefing.
func (s *Briefing) WithWeather(w port.Forecaster) *Briefing {
	s.weather = w
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
	var weather string
	if s.weather != nil {
		if w, err := s.weather.Today(ctx); err != nil {
			slog.WarnContext(ctx, "briefing: weather", "err", err)
		} else {
			weather = weatherText(w)
		}
	}
	for _, hh := range ids {
		if err := s.send(ctx, hh, weather, now); err != nil {
			slog.WarnContext(ctx, "briefing: send", "err", err)
		}
	}
	return nil
}

func (s *Briefing) send(ctx context.Context, householdID uuid.UUID, weather string, now time.Time) error {
	members, err := s.households.Members(ctx, householdID)
	if err != nil {
		return err
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
		var parts []string
		if weather != "" {
			parts = append(parts, weather)
		}
		if len(plans) > 0 {
			parts = append(parts, "📅 "+names(plans))
		}
		if len(due) > 0 {
			parts = append(parts, "✅ "+names(due))
		}
		if len(parts) == 0 {
			continue
		}
		s.notify.Notify(ctx, []uuid.UUID{m.User.ID}, domain.Notification{
			Key: "push.briefing", Args: []any{"day", strings.Join(parts, " · ")}, URL: "/", Tag: "briefing",
		})
	}
	return nil
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

// weatherText is an emoji for the sky and the day's low to high, like "🌧️ 8–14°".
func weatherText(w domain.Weather) string {
	return fmt.Sprintf("%s %.0f–%.0f°", weatherEmoji(w.Code), w.Low, w.High)
}

// weatherEmoji maps a WMO weather code to a picture.
func weatherEmoji(code int) string {
	switch {
	case code == 0:
		return "☀️"
	case code <= 2:
		return "🌤️"
	case code == 3:
		return "☁️"
	case code <= 48:
		return "🌫️"
	case code <= 67, code >= 80 && code <= 82:
		return "🌧️"
	case code <= 77, code == 85, code == 86:
		return "❄️"
	case code >= 95:
		return "⛈️"
	default:
		return "🌦️"
	}
}
