package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/postgres/sqlcgen"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
)

func (s *Store) VoiceSettings(ctx context.Context, userID uuid.UUID) (domain.VoiceSettings, error) {
	r, err := s.q.GetVoiceSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VoiceSettings{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.VoiceSettings{}, err
	}
	return domain.VoiceSettings{Voice: r.Voice, Speed: r.Speed, Tone: domain.VoiceTone(r.Tone)}, nil
}

func (s *Store) SaveVoiceSettings(ctx context.Context, userID uuid.UUID, v domain.VoiceSettings) error {
	return s.q.UpsertVoiceSettings(ctx, sqlcgen.UpsertVoiceSettingsParams{UserID: userID, Voice: v.Voice, Speed: v.Speed, Tone: string(v.Tone)})
}
