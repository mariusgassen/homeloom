package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// defaultDateTime is when an idea starts if the LLM names no usable time.
const defaultDateTime = "19:00"

// DateNight suggests evenings from the weather, what's on, a budget and a mood. Without the weather
// the ideas simply don't mention it; without an LLM there are none.
type DateNight struct {
	households port.HouseholdRepository
	planner    port.Planner // nil: unavailable
	weather    port.Weather // nil: no forecast
	loc        *time.Location
	now        func() time.Time
}

func NewDateNight(households port.HouseholdRepository, planner port.Planner, weather port.Weather, loc *time.Location) *DateNight {
	return &DateNight{households: households, planner: planner, weather: weather, loc: loc, now: time.Now}
}

func (s *DateNight) Ideas(ctx context.Context, actor domain.User, householdID uuid.UUID, q domain.DateQuery, lang string) (domain.DateNight, error) {
	if _, err := s.households.Role(ctx, householdID, actor.ID); err != nil {
		return domain.DateNight{}, err
	}
	q.Place, q.Mood, q.Constraints = strings.TrimSpace(q.Place), strings.TrimSpace(q.Mood), strings.TrimSpace(q.Constraints)
	if utf8.RuneCountInString(q.Place) > domain.MaxDatePlace || utf8.RuneCountInString(q.Mood) > domain.MaxDateMood || utf8.RuneCountInString(q.Constraints) > domain.MaxDateMood {
		return domain.DateNight{}, fmt.Errorf("%w: place, mood and limits are too long", domain.ErrInvalid)
	}
	if q.Budget == "" {
		q.Budget = domain.DateEasy
	}
	if !q.Budget.Valid() {
		return domain.DateNight{}, fmt.Errorf("%w: unknown budget %q", domain.ErrInvalid, q.Budget)
	}
	today := date(s.now().In(s.loc))
	if q.Day.IsZero() {
		q.Day = today
	}
	if q.Day.Before(today) || q.Day.After(today.AddDate(0, 0, domain.MaxDateAheadDays)) {
		return domain.DateNight{}, fmt.Errorf("%w: the day must be within %d days from today", domain.ErrInvalid, domain.MaxDateAheadDays)
	}
	if s.planner == nil {
		return domain.DateNight{}, fmt.Errorf("%w: no LLM is configured", domain.ErrUnavailable)
	}

	brief := domain.DateBrief{DateQuery: q}
	if s.weather != nil && q.Place != "" {
		f, err := s.weather.Forecast(ctx, q.Place, q.Day)
		switch {
		case err == nil:
			brief.Forecast = &f
		case errors.Is(err, domain.ErrNotFound):
			// An unknown place is the person's to fix; carry on without the weather.
			slog.InfoContext(ctx, "date night: place not found")
		default:
			slog.WarnContext(ctx, "date night: no forecast", "err", err)
		}
	}

	ideas, err := s.planner.Ideas(ctx, brief, lang, domain.DateIdeaCount)
	if err != nil {
		return domain.DateNight{}, err
	}
	out := make([]domain.DateIdea, 0, len(ideas))
	for _, x := range ideas {
		x.Title = strings.TrimSpace(x.Title)
		if x.Title == "" || len(out) == domain.DateIdeaCount {
			continue
		}
		x.StartsAt = s.startsAt(q.Day, x.Time)
		x.Time = x.StartsAt.Format("15:04")
		out = append(out, x)
	}
	if len(out) == 0 {
		return domain.DateNight{}, fmt.Errorf("%w: the LLM suggested nothing usable", domain.ErrUnavailable)
	}
	return domain.DateNight{Ideas: out, Forecast: brief.Forecast}, nil
}

// startsAt is the "HH:MM" on the day in the household's time zone; evenings outside 11:00-23:30
// and unreadable times become 19:00.
func (s *DateNight) startsAt(day time.Time, hhmm string) time.Time {
	at := func(v string) (time.Time, error) {
		t, err := time.Parse("15:04", strings.TrimSpace(v))
		if err != nil {
			return time.Time{}, err
		}
		return time.Date(day.Year(), day.Month(), day.Day(), t.Hour(), t.Minute(), 0, 0, s.loc), nil
	}
	t, err := at(hhmm)
	if err != nil || t.Hour() < 11 || t.Hour() == 23 && t.Minute() > 30 {
		t, _ = at(defaultDateTime)
	}
	return t
}
