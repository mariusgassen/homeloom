package openai_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/openai"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func TestSpeak(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte("mp3"))
	}))
	defer srv.Close()

	v := domain.VoiceSettings{Voice: "marin", Speed: 1.25, Tone: domain.ToneCalm}
	audio, err := openai.NewSpeaker("sk-test").WithURL(srv.URL).Speak(context.Background(), "Hallo", v)
	if err != nil || string(audio) != "mp3" {
		t.Fatalf("speak: %q %v", audio, err)
	}
	if got["model"] != openai.Model || got["voice"] != "marin" || got["speed"] != 1.25 || got["input"] != "Hallo" || got["instructions"] != domain.ToneCalm.Instructions() {
		t.Fatalf("request: %+v", got)
	}

	if _, err := openai.NewSpeaker("wrong").WithURL(srv.URL).Speak(context.Background(), "Hallo", v); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("refused: want ErrUnavailable, got %v", err)
	}
}
