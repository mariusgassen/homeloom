package claude_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/claude"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestMuse(t *testing.T) {
	var req map[string]any
	srv := fakeAPI(t, "end_turn", `{"questions":[" Was war dein schönster Urlaub? "]}`, &req)
	muse := claude.NewMuse("key", "claude-opus-5-5", option.WithBaseURL(srv.URL))

	qs, err := muse.Questions(context.Background(), "de", []string{"travel and places"}, []string{"Best | day?"}, 3)
	if err != nil || len(qs) != 1 || qs[0] != "Was war dein schönster Urlaub?" {
		t.Fatalf("questions: %q, %v", qs, err)
	}
	user := fmt.Sprint(req["messages"])
	for _, want := range []string{"travel and places", "Best / day?", "Write 3 new questions in German"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q: %s", want, user)
		}
	}

	down := claude.NewMuse("key", "claude-opus-5-5", option.WithBaseURL(fakeAPI(t, "refusal", "", nil).URL))
	if _, err := down.Questions(context.Background(), "en", nil, nil, 3); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("refusal: want ErrUnavailable, got %v", err)
	}
}

func TestCritic(t *testing.T) {
	var req map[string]any
	srv := fakeAPI(t, "end_turn", `{"picks":[{"id":"abc.10","name":"","year":0,"kind":"movie","reason":" Calm. "},{"id":"","name":"Dark","year":2017,"kind":"show","reason":"Gripping."}]}`, &req)
	critic := claude.NewCritic("key", "claude-opus-5-5", option.WithBaseURL(srv.URL))

	candidates := []domain.Title{{ID: "abc.10", Kind: domain.Movies, Name: "Arrival", Year: 2016, Genres: []string{"Sci-Fi"}, Minutes: 116}}
	seen := []domain.Title{{Name: "Heat", Year: 1995}}
	picks, err := critic.Pick(context.Background(), "calm", "en", candidates, seen, 4)
	if err != nil || len(picks) != 2 || picks[0] != (domain.WatchPick{TitleID: "abc.10", Kind: domain.Movies, Reason: "Calm."}) ||
		picks[1] != (domain.WatchPick{Name: "Dark", Year: 2017, Kind: domain.Shows, Reason: "Gripping."}) {
		t.Fatalf("picks: %+v, %v", picks, err)
	}
	user := fmt.Sprint(req["messages"])
	for _, want := range []string{"abc.10 | movie | Arrival | 2016 | Sci-Fi | 116", "Heat (1995)", "<mood>calm</mood>"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q: %s", want, user)
		}
	}
}

func TestPlanner(t *testing.T) {
	var req map[string]any
	srv := fakeAPI(t, "end_turn", `{"ideas":[{"title":" Open-air cinema ","reason":"Warm night.","where":"Park","cost":"about 10 €","indoor":false,"time":"21:00"}]}`, &req)
	p := claude.NewPlanner("key", "claude-opus-5-5", option.WithBaseURL(srv.URL))

	brief := domain.DateBrief{
		DateQuery: domain.DateQuery{Place: "Berlin", Day: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), Budget: domain.DateEasy, Mood: "cosy"},
		Forecast:  &domain.Forecast{Sky: domain.SkyRain, HighC: 9, LowC: 4, RainPercent: 80},
	}
	ideas, err := p.Ideas(context.Background(), brief, "de", 3)
	if err != nil || len(ideas) != 1 || ideas[0] != (domain.DateIdea{Title: "Open-air cinema", Reason: "Warm night.", Where: "Park", Cost: "about 10 €", Time: "21:00"}) {
		t.Fatalf("ideas: %+v, %v", ideas, err)
	}
	user := fmt.Sprint(req["messages"])
	for _, want := range []string{"<place>Berlin</place>", "Saturday 2026-11-07", "<budget>easy</budget>", "rain, 4 to 9 °C, 80%", "in German"} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q: %s", want, user)
		}
	}
	if !strings.Contains(fmt.Sprint(req["tools"]), "web_search") {
		t.Errorf("with a place, web search should be offered: %v", req["tools"])
	}

	down := claude.NewPlanner("key", "claude-opus-5-5", option.WithBaseURL(fakeAPI(t, "refusal", "", nil).URL))
	if _, err := down.Ideas(context.Background(), brief, "en", 3); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("refusal: want ErrUnavailable, got %v", err)
	}
}
