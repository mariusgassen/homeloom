package http

import (
	"bytes"
	"context"
	"errors"
	"log/slog"

	"github.com/mariusgassen/homeloom/apps/api/internal/adapter/http/oapi"
	"github.com/mariusgassen/homeloom/apps/api/internal/core/domain"
	"github.com/mariusgassen/homeloom/apps/api/internal/i18n"
)

func (h *handlers) GetVoice(ctx context.Context, _ oapi.GetVoiceRequestObject) (oapi.GetVoiceResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.voice.Settings(ctx, u)
	if err != nil {
		return nil, err
	}
	return oapi.GetVoice200JSONResponse{Available: h.voice.Available(), Settings: toVoiceSettings(v)}, nil
}

func (h *handlers) SaveVoice(ctx context.Context, req oapi.SaveVoiceRequestObject) (oapi.SaveVoiceResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := h.voice.Save(ctx, u, fromVoiceSettings(*req.Body))
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.SaveVoice400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	case err != nil:
		return nil, err
	}
	return oapi.SaveVoice200JSONResponse(toVoiceSettings(v)), nil
}

func (h *handlers) PreviewVoice(ctx context.Context, req oapi.PreviewVoiceRequestObject) (oapi.PreviewVoiceResponseObject, error) {
	u, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	audio, err := h.voice.Preview(ctx, u, fromVoiceSettings(*req.Body), i18n.T(i18n.FromContext(ctx), "voice.preview"))
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return oapi.PreviewVoice400JSONResponse{BadRequestJSONResponse: badRequest(ctx, err)}, nil
	case errors.Is(err, domain.ErrUnavailable):
		slog.WarnContext(ctx, "voice preview unavailable", "err", err)
		return oapi.PreviewVoice503JSONResponse{UnavailableJSONResponse: oapi.UnavailableJSONResponse(errorBody(ctx, "voice_unavailable", ""))}, nil
	case err != nil:
		return nil, err
	}
	return oapi.PreviewVoice200AudiompegResponse{Body: bytes.NewReader(audio), ContentLength: int64(len(audio))}, nil
}

func toVoiceSettings(v domain.VoiceSettings) oapi.VoiceSettings {
	return oapi.VoiceSettings{Voice: oapi.VoiceSettingsVoice(v.Voice), Speed: v.Speed, Tone: oapi.VoiceSettingsTone(v.Tone)}
}

func fromVoiceSettings(v oapi.VoiceSettings) domain.VoiceSettings {
	return domain.VoiceSettings{Voice: string(v.Voice), Speed: v.Speed, Tone: domain.VoiceTone(v.Tone)}
}
