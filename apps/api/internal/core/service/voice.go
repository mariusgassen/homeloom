package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// previewTimeout bounds speaking one preview sentence.
const previewTimeout = 20 * time.Second

// Voice keeps how Homeloom sounds on each member's calls with it.
type Voice struct {
	settings port.VoiceSettingsRepository
	speaker  port.Speaker
}

// NewVoice works with a nil speaker: Homeloom then can't speak, and settings are kept for later.
func NewVoice(settings port.VoiceSettingsRepository, speaker port.Speaker) *Voice {
	return &Voice{settings: settings, speaker: speaker}
}

func (s *Voice) Available() bool { return s.speaker != nil }

func (s *Voice) Settings(ctx context.Context, actor domain.User) (domain.VoiceSettings, error) {
	v, err := s.settings.VoiceSettings(ctx, actor.ID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.DefaultVoice, nil
	}
	return v, err
}

func (s *Voice) Save(ctx context.Context, actor domain.User, v domain.VoiceSettings) (domain.VoiceSettings, error) {
	if err := v.Validate(); err != nil {
		return domain.VoiceSettings{}, err
	}
	if err := s.settings.SaveVoiceSettings(ctx, actor.ID, v); err != nil {
		return domain.VoiceSettings{}, err
	}
	return v, nil
}

func (s *Voice) Preview(ctx context.Context, _ domain.User, v domain.VoiceSettings, text string) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	if s.speaker == nil {
		return nil, fmt.Errorf("%w: speech is not set up", domain.ErrUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, previewTimeout)
	defer cancel()
	return s.speaker.Speak(ctx, text, v)
}

// forCall is how Homeloom sounds on the actor's call. Settings that can't be read don't stop the
// call: it then uses the defaults.
func (s *Voice) forCall(ctx context.Context, actor domain.User) domain.VoiceSettings {
	if s == nil {
		return domain.DefaultVoice
	}
	v, err := s.Settings(ctx, actor)
	if err != nil {
		slog.WarnContext(ctx, "voice settings unavailable; using the defaults", "err", err)
		return domain.DefaultVoice
	}
	return v
}
