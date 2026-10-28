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
}

// RedirectURL is the login callback to register in authentik.
func (c Config) RedirectURL() string { return strings.TrimRight(c.PublicURL, "/") + "/auth/callback" }

// PostLogoutURL is where authentik sends the browser after signing out.
func (c Config) PostLogoutURL() string { return strings.TrimRight(c.PublicURL, "/") + "/" }

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
	}
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
