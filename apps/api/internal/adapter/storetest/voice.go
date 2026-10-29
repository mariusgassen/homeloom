package storetest

import (
	"context"
	"errors"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/port"
)

// VoiceSettings: none until saved, then one per member, replaced by the next save.
func VoiceSettings(t *testing.T, s interface {
	Store
	port.VoiceSettingsRepository
}) {
	ctx := context.Background()
	alice, bob := user(t, s, "Alice"), user(t, s, "Bob")
	if _, err := s.VoiceSettings(ctx, alice.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("before saving: %v", err)
	}
	for _, v := range []domain.VoiceSettings{{Voice: "marin", Speed: 1.25, Tone: domain.ToneCalm}, {Voice: "cedar", Speed: 0.9, Tone: domain.TonePlayful}} {
		if err := s.SaveVoiceSettings(ctx, alice.ID, v); err != nil {
			t.Fatal(err)
		}
		if got, err := s.VoiceSettings(ctx, alice.ID); err != nil || got != v {
			t.Fatalf("got %+v %v, want %+v", got, err, v)
		}
	}
	if _, err := s.VoiceSettings(ctx, bob.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("bob has none: %v", err)
	}
}
