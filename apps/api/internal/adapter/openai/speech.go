// Package openai speaks with OpenAI's speech model, the same one the voice agent uses on calls.
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

// Model is the speech model; the voice agent speaks with the same one.
const Model = "gpt-4o-mini-tts"

// maxAudio caps a spoken answer; a preview sentence is far smaller.
const maxAudio = 4 << 20

type Speaker struct {
	url  string
	key  string
	http *http.Client
}

func NewSpeaker(apiKey string) *Speaker {
	return &Speaker{url: "https://api.openai.com/v1/audio/speech", key: apiKey, http: &http.Client{Timeout: 20 * time.Second}}
}

// WithURL points the speaker at another server, for tests.
func (s *Speaker) WithURL(url string) *Speaker {
	s.url = url
	return s
}

func (s *Speaker) Speak(ctx context.Context, text string, v domain.VoiceSettings) ([]byte, error) {
	body, err := json.Marshal(map[string]any{
		"model":           Model,
		"input":           text,
		"voice":           v.Voice,
		"speed":           v.Speed,
		"instructions":    v.Tone.Instructions(),
		"response_format": "mp3",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: speech: %w", domain.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: speech answered %s", domain.ErrUnavailable, resp.Status)
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, maxAudio))
	if err != nil {
		return nil, fmt.Errorf("%w: speech: %w", domain.ErrUnavailable, err)
	}
	return audio, nil
}
