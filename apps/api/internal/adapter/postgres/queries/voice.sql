-- name: GetVoiceSettings :one
SELECT * FROM voice_settings WHERE user_id = $1;

-- name: UpsertVoiceSettings :exec
INSERT INTO voice_settings (user_id, voice, speed, tone) VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE
SET voice = EXCLUDED.voice, speed = EXCLUDED.speed, tone = EXCLUDED.tone, updated_at = now();
