// Package config reads all settings from environment variables. Secrets never come from files in the repo.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	RedisURL      string
	PublicURL     string // where the app is served, e.g. https://homeloom.example.com
	OIDCIssuerURL string
	// OIDCClientID and OIDCClientSecret are the confidential client the browser login uses.
	OIDCClientID     string
	OIDCClientSecret string
	// OIDCClientIDs are further clients (iOS, MCP, ...) whose bearer tokens are accepted.
	OIDCClientIDs  []string
	AllowedOrigins []string
	UserCacheTTL   time.Duration
	SessionTTL     time.Duration
	LogLevel       string
	// Mealie is optional and reached through its MCP server: without MealieMCPURL cooking is
	// switched off. MealieURL is Mealie itself, only for links into it.
	MealieMCPURL   string
	MealieMCPToken string
	MealieURL      string
	// AnthropicAPIKey is optional: without it recipes are suggested at random.
	AnthropicAPIKey string
	LLMModel        string
	// Home Assistant is optional: without HomeAssistantURL the Home tab is switched off. A member
	// connects it by signing in there; SecretKey encrypts the token that sign-in gives.
	HomeAssistantURL string
	SecretKey        string
	// Plex is on when HOMELOOM_SECRET_KEY is set, unless PLEX_ENABLED is false: each member signs
	// in at plex.tv, so there is nothing else to configure. The key encrypts their tokens.
	Plex bool
	// Web Push is optional: without the VAPID key pair notifications are switched off. VAPIDSubject
	// tells the push services who sends them (an https URL or e-mail address; PUBLIC_URL by default).
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
	// Calls are optional: without the LiveKit API key pair they are switched off. LiveKitURL is
	// where the API reaches the server; clients connect through PUBLIC_URL, which forwards /rtc.
	LiveKitURL       string
	LiveKitAPIKey    string
	LiveKitAPISecret string
	// VoiceAgent is the name the Homeloom voice agent registers with LiveKit; "" switches voice
	// calls with Homeloom off. It is off without ANTHROPIC_API_KEY or OPENAI_API_KEY, since the
	// agent thinks with Claude and hears and speaks with OpenAI.
	VoiceAgent string
	// OpenAIAPIKey lets the API speak voice previews the way the agent speaks on a call.
	OpenAIAPIKey string
	// TimeZone is where the household lives; repeating reminders keep their time of day in it.
	TimeZone *time.Location
	// S3 is optional: with a key pair, new moment photos go to the bucket (the bundled Garage by
	// default) instead of Postgres.
	S3Endpoint        string
	S3Region          string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
}

// RedirectURL is the login callback to register in authentik.
func (c Config) RedirectURL() string { return strings.TrimRight(c.PublicURL, "/") + "/auth/callback" }

// PostLogoutURL is where authentik sends the browser after signing out.
func (c Config) PostLogoutURL() string { return strings.TrimRight(c.PublicURL, "/") + "/" }

// LiveKitPublicURL is where clients connect to LiveKit: the app's own origin, as a WebSocket URL.
func (c Config) LiveKitPublicURL() string {
	u := strings.TrimRight(c.PublicURL, "/")
	if rest, ok := strings.CutPrefix(u, "https://"); ok {
		return "wss://" + rest
	}
	return "ws://" + strings.TrimPrefix(u, "http://")
}

// SecureCookies is true when the app is served over HTTPS.
func (c Config) SecureCookies() bool { return strings.HasPrefix(c.PublicURL, "https://") }

func Load() (Config, error) {
	c := Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		PublicURL:        os.Getenv("PUBLIC_URL"),
		OIDCIssuerURL:    os.Getenv("OIDC_ISSUER_URL"),
		OIDCClientID:     os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCClientIDs:    list(os.Getenv("OIDC_CLIENT_IDS")),
		AllowedOrigins:   list(os.Getenv("CORS_ALLOWED_ORIGINS")),
		LogLevel:         env("LOG_LEVEL", "info"),
		MealieMCPURL:     os.Getenv("MEALIE_MCP_URL"),
		MealieMCPToken:   os.Getenv("MEALIE_MCP_TOKEN"),
		MealieURL:        os.Getenv("MEALIE_URL"),
		AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		LLMModel:         env("LLM_MODEL", "claude-opus-5-5"),
		HomeAssistantURL: os.Getenv("HOME_ASSISTANT_URL"),
		SecretKey:        os.Getenv("HOMELOOM_SECRET_KEY"),
		VAPIDPublicKey:   os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey:  os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:     env("VAPID_SUBJECT", os.Getenv("PUBLIC_URL")),
		LiveKitURL:       env("LIVEKIT_URL", "http://homeloom-livekit:7880"),
		LiveKitAPIKey:    os.Getenv("LIVEKIT_API_KEY"),
		LiveKitAPISecret: os.Getenv("LIVEKIT_API_SECRET"),
		VoiceAgent:       env("VOICE_AGENT", "homeloom"),
		OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
	}
	c.S3Endpoint, c.S3Region, c.S3Bucket = env("S3_ENDPOINT", "http://homeloom-garage:3900"), env("S3_REGION", "garage"), env("S3_BUCKET", "homeloom")
	c.S3AccessKeyID, c.S3SecretAccessKey = os.Getenv("S3_ACCESS_KEY_ID"), os.Getenv("S3_SECRET_ACCESS_KEY")
	c.Plex = c.SecretKey != "" && env("PLEX_ENABLED", "true") != "false"
	if c.AnthropicAPIKey == "" || c.OpenAIAPIKey == "" {
		c.VoiceAgent = ""
	}
	loc, err := time.LoadLocation(env("TIME_ZONE", "UTC"))
	if err != nil {
		return c, fmt.Errorf("TIME_ZONE: %w", err)
	}
	c.TimeZone = loc
	ttl, err := time.ParseDuration(env("USER_CACHE_TTL", "5m"))
	if err != nil {
		return c, fmt.Errorf("USER_CACHE_TTL: %w", err)
	}
	c.UserCacheTTL = ttl
	if c.SessionTTL, err = time.ParseDuration(env("SESSION_TTL", "720h")); err != nil {
		return c, fmt.Errorf("SESSION_TTL: %w", err)
	}

	var missing []string
	for _, req := range []struct{ name, value string }{
		{"DATABASE_URL", c.DatabaseURL},
		{"REDIS_URL", c.RedisURL},
		{"PUBLIC_URL", c.PublicURL},
		{"OIDC_ISSUER_URL", c.OIDCIssuerURL},
		{"OIDC_CLIENT_ID", c.OIDCClientID},
		{"OIDC_CLIENT_SECRET", c.OIDCClientSecret},
	} {
		if req.value == "" {
			missing = append(missing, req.name)
		}
	}
	if c.MealieMCPURL != "" && c.MealieMCPToken == "" {
		missing = append(missing, "MEALIE_MCP_TOKEN")
	}
	if c.HomeAssistantURL != "" && c.SecretKey == "" {
		missing = append(missing, "HOMELOOM_SECRET_KEY")
	}
	if c.VAPIDPublicKey != "" && c.VAPIDPrivateKey == "" {
		missing = append(missing, "VAPID_PRIVATE_KEY")
	}
	if c.VAPIDPrivateKey != "" && c.VAPIDPublicKey == "" {
		missing = append(missing, "VAPID_PUBLIC_KEY")
	}
	if c.LiveKitAPIKey != "" && c.LiveKitAPISecret == "" {
		missing = append(missing, "LIVEKIT_API_SECRET")
	}
	if c.S3AccessKeyID != "" && c.S3SecretAccessKey == "" {
		missing = append(missing, "S3_SECRET_ACCESS_KEY")
	}
	if c.S3SecretAccessKey != "" && c.S3AccessKeyID == "" {
		missing = append(missing, "S3_ACCESS_KEY_ID")
	}
	if len(missing) > 0 {
		return c, errors.New("missing required env: " + strings.Join(missing, ", "))
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func list(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
