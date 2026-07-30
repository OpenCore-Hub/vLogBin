// Package config loads platform API configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	// DatabaseURL is the runtime DSN. The app must connect as the
	// platform_app role so row level security applies to every query.
	DatabaseURL string
	// MigrationDatabaseURL is the DSN used to run goose migrations (needs
	// DDL privileges). Falls back to DatabaseURL when unset.
	MigrationDatabaseURL string
	HTTPAddr             string
	OperatorToken        string
	PlatformBaseDomain   string
	OutboxPollInterval   time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		MigrationDatabaseURL: os.Getenv("MIGRATION_DATABASE_URL"),
		HTTPAddr:             envOr("HTTP_ADDR", ":8080"),
		OperatorToken:        os.Getenv("OPERATOR_TOKEN"),
		PlatformBaseDomain:   envOr("PLATFORM_BASE_DOMAIN", "platform.local"),
		OutboxPollInterval:   time.Second,
	}
	if v := os.Getenv("OUTBOX_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid OUTBOX_POLL_INTERVAL %q: %w", v, err)
		}
		cfg.OutboxPollInterval = d
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.MigrationDatabaseURL == "" {
		cfg.MigrationDatabaseURL = cfg.DatabaseURL
	}
	if cfg.OperatorToken == "" {
		return Config{}, fmt.Errorf("OPERATOR_TOKEN is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
