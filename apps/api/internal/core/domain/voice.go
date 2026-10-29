package domain

import (
	"fmt"
	"slices"
)

// Voices are the speech model's voices a member can pick for Homeloom, best first.
var Voices = []string{"marin", "cedar", "coral", "sage", "shimmer", "nova", "alloy", "ash", "ballad", "echo", "fable", "onyx", "verse"}

// The slowest and fastest Homeloom speaks; 1 is the voice's natural pace.
const (
	MinVoiceSpeed = 0.75
	MaxVoiceSpeed = 1.5
)

// VoiceTone is how Homeloom speaks, whatever the voice.
type VoiceTone string

const (
	ToneWarm    VoiceTone = "warm"
	ToneCalm    VoiceTone = "calm"
	TonePlayful VoiceTone = "playful"
	ToneClear   VoiceTone = "clear"
)

// tones are the instructions the speech model gets for each tone.
var tones = map[VoiceTone]string{
	ToneWarm:    "Speak warmly and naturally, like a friend at home.",
	ToneCalm:    "Speak calmly and softly, with a relaxed, unhurried, soothing delivery.",
	TonePlayful: "Speak in a playful, upbeat way, with a smile in your voice and lively intonation.",
	ToneClear:   "Speak clearly and evenly, with crisp pronunciation and a friendly, matter-of-fact delivery.",
}

// Instructions tells the speech model how to speak in this tone.
func (t VoiceTone) Instructions() string {
	if s, ok := tones[t]; ok {
		return s
	}
	return tones[ToneWarm]
}

// VoiceSettings are how Homeloom sounds on one member's calls with it.
type VoiceSettings struct {
	Voice string
	Speed float64
	Tone  VoiceTone
}

// DefaultVoice is how Homeloom sounds until a member picks something else.
var DefaultVoice = VoiceSettings{Voice: "coral", Speed: 1, Tone: ToneWarm}

func (v VoiceSettings) Validate() error {
	if !slices.Contains(Voices, v.Voice) {
		return fmt.Errorf("%w: unknown voice %q", ErrInvalid, v.Voice)
	}
	if v.Speed < MinVoiceSpeed || v.Speed > MaxVoiceSpeed {
		return fmt.Errorf("%w: speed is between %v and %v", ErrInvalid, MinVoiceSpeed, MaxVoiceSpeed)
	}
	if _, ok := tones[v.Tone]; !ok {
		return fmt.Errorf("%w: unknown tone %q", ErrInvalid, v.Tone)
	}
	return nil
}
