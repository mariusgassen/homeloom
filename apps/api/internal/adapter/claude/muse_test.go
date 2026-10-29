package claude_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
