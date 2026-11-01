// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime settings for accountd.
type Config struct {
	// Addr is the bind address of the HTTP server, e.g. "127.0.0.1:8080".
	Addr string
	// FrontendURL is where non-API, non-OIDC routes are proxied.
	// In dev this is the Vite dev server; in prod the built SvelteKit server.
	FrontendURL string
	// DatabaseURL is a Postgres connection string (pgx).
	DatabaseURL string
	// RedisURL is a Redis connection string, e.g. "redis://localhost:6379/0".
	RedisURL string
	// Issuer is the public OIDC issuer URL, e.g. "https://account.zealhaven.id".
	Issuer string
	// CookieSecure marks session cookies Secure. False for local dev.
	CookieSecure bool
	// SMTP settings for identity-verification codes (optional in dev:
	// when unset, codes are logged instead of emailed).
	SMTP SMTPConfig
}

// SMTPConfig carries the outbound mail settings.
type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// Load reads configuration from the environment, applying defaults suitable
// for local development.
func Load() (Config, error) {
	cfg := Config{
		Addr:         getenv("ACCOUNT_ADDR", "127.0.0.1:8080"),
		FrontendURL:  os.Getenv("FRONTEND_URL"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		RedisURL:     os.Getenv("REDIS_URL"),
		Issuer:       getenv("OIDC_ISSUER", "http://localhost:8080"),
		CookieSecure: os.Getenv("COOKIE_SECURE") == "true",
		SMTP: SMTPConfig{
			Host: os.Getenv("SMTP_HOST"),
			Port: getenv("SMTP_PORT", "587"),
			User: os.Getenv("SMTP_USER"),
			Pass: os.Getenv("SMTP_PASS"),
			From: os.Getenv("SMTP_FROM"),
		},
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return Config{}, fmt.Errorf("REDIS_URL is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// GetenvInt reads an integer env var, falling back on error or absence.
func GetenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
