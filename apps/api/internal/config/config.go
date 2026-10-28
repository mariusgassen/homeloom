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
	HTTPAddr       string
	DatabaseURL    string
	RedisURL       string
	OIDCIssuerURL  string
	OIDCClientIDs  []string
	AllowedOrigins []string
	UserCacheTTL   time.Duration
	LogLevel       string
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:       env("HTTP_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		RedisURL:       os.Getenv("REDIS_URL"),
		OIDCIssuerURL:  os.Getenv("OIDC_ISSUER_URL"),
		OIDCClientIDs:  list(os.Getenv("OIDC_CLIENT_IDS")),
		AllowedOrigins: list(os.Getenv("CORS_ALLOWED_ORIGINS")),
		LogLevel:       env("LOG_LEVEL", "info"),
	}
	ttl, err := time.ParseDuration(env("USER_CACHE_TTL", "5m"))
	if err != nil {
		return c, fmt.Errorf("USER_CACHE_TTL: %w", err)
	}
	c.UserCacheTTL = ttl

	var missing []string
	for _, req := range []struct{ name, value string }{
		{"DATABASE_URL", c.DatabaseURL},
		{"REDIS_URL", c.RedisURL},
		{"OIDC_ISSUER_URL", c.OIDCIssuerURL},
	} {
		if req.value == "" {
			missing = append(missing, req.name)
		}
	}
	if len(c.OIDCClientIDs) == 0 {
		missing = append(missing, "OIDC_CLIENT_IDS")
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
