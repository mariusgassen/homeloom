package memory

import (
	"context"

	"github.com/google/uuid"

	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) VoiceSettings(_ context.Context, userID uuid.UUID) (domain.VoiceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.voices[userID]
	if !ok {
		return domain.VoiceSettings{}, domain.ErrNotFound
	}
	return v, nil
}

func (s *Store) SaveVoiceSettings(_ context.Context, userID uuid.UUID, v domain.VoiceSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.voices[userID] = v
	return nil
}

// Speaker stands in for the speech model: it "speaks" the text and settings as bytes. Set Down
// to make it unreachable.
type Speaker struct{ Down bool }

func (s *Speaker) Speak(_ context.Context, text string, v domain.VoiceSettings) ([]byte, error) {
	if s.Down {
		return nil, domain.ErrUnavailable
	}
	return []byte(v.Voice + ":" + text), nil
}
