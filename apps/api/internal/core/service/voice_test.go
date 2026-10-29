package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/memory"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/service"
)

func TestVoice(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStore()
	alice, _ := store.UpsertByIdentity(ctx, domain.Identity{Subject: "a", Name: "Alice"})
	speaker := &memory.Speaker{}
	voice := service.NewVoice(store, speaker)

	if v, err := voice.Settings(ctx, alice); err != nil || v != domain.DefaultVoice {
		t.Fatalf("defaults: %+v %v", v, err)
	}
	for _, bad := range []domain.VoiceSettings{
		{Voice: "robot", Speed: 1, Tone: domain.ToneWarm},
		{Voice: "marin", Speed: 0.5, Tone: domain.ToneWarm},
		{Voice: "marin", Speed: 1, Tone: "shouty"},
	} {
		if _, err := voice.Save(ctx, alice, bad); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%+v: want ErrInvalid, got %v", bad, err)
		}
		if _, err := voice.Preview(ctx, alice, bad, "Hi"); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("preview %+v: want ErrInvalid, got %v", bad, err)
		}
	}
	calm := domain.VoiceSettings{Voice: "sage", Speed: 0.9, Tone: domain.ToneCalm}
	if _, err := voice.Save(ctx, alice, calm); err != nil {
		t.Fatal(err)
	}
	if v, _ := voice.Settings(ctx, alice); v != calm {
		t.Fatalf("saved: %+v", v)
	}
	if audio, err := voice.Preview(ctx, alice, calm, "Hi"); err != nil || string(audio) != "sage:Hi" {
		t.Fatalf("preview: %q %v", audio, err)
	}
	speaker.Down = true
	if _, err := voice.Preview(ctx, alice, calm, "Hi"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("speech down: %v", err)
	}
	if _, err := service.NewVoice(store, nil).Preview(ctx, alice, calm, "Hi"); !errors.Is(err, domain.ErrUnavailable) || service.NewVoice(store, nil).Available() {
		t.Fatalf("without speech: %v", err)
	}
}
