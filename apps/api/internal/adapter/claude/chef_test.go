package claude_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/claude"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// fakeAPI answers every request with one message whose text and stop reason are given.
func fakeAPI(t *testing.T, stop, text string, seen *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			_ = json.NewDecoder(r.Body).Decode(seen)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"stop_reason": stop,
			"content":     []map[string]any{{"type": "text", "text": text}},
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

var candidates = []domain.RecipeSummary{
	{Slug: "soup", Name: "Mushroom | soup", Tags: []string{"Suppe"}, Description: strings.Repeat("long ", 100)},
	{Slug: "curry", Name: "Curry"},
}

func TestPick(t *testing.T) {
	var req map[string]any
	srv := fakeAPI(t, "end_turn", `{"picks":[{"slug":"soup","reason":" Warm und cremig. "}]}`, &req)
	chef := claude.NewChef("key", "claude-opus-5-5", option.WithBaseURL(srv.URL))

	picks, err := chef.Pick(context.Background(), "cozy", "de", candidates, 3)
	if err != nil || len(picks) != 1 || picks[0] != (domain.Pick{Slug: "soup", Reason: "Warm und cremig."}) {
		t.Fatalf("pick: %+v, %v", picks, err)
	}
	system, _ := json.Marshal(req["system"])
	if !strings.Contains(string(system), "soup | Mushroom / soup | Suppe |  | long") || strings.Contains(string(system), strings.Repeat("long ", 60)) {
		t.Errorf("collection not one clipped line per recipe: %s", system)
	}
	user := fmt.Sprint(req["messages"])
	if !strings.Contains(user, "<mood>cozy</mood>") || !strings.Contains(user, "German") {
		t.Errorf("mood or language missing: %s", user)
	}
	if req["fallbacks"] != "default" {
		t.Errorf("fallbacks = %v", req["fallbacks"])
	}
}

func TestPickFailuresAreUnavailable(t *testing.T) {
	for name, srv := range map[string]*httptest.Server{
		"refusal":  fakeAPI(t, "refusal", "", nil),
		"not json": fakeAPI(t, "end_turn", "Here are some ideas", nil),
		"api down": httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })),
	} {
		t.Cleanup(srv.Close)
		chef := claude.NewChef("key", "claude-opus-5-5", option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
		if _, err := chef.Pick(context.Background(), "cozy", "en", candidates, 3); !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("%s: want ErrUnavailable, got %v", name, err)
		}
	}
}
