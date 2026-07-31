// Package config loads platform API configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	// WebhookPollInterval controls how often the webhook delivery worker
	// polls for published outbox events and pending deliveries
	// (WEBHOOK_POLL_INTERVAL, default 5s).
	WebhookPollInterval time.Duration
	// BillingAdapter selects the billing engine: "noop" (default) or "lago".
	BillingAdapter string
	// LagoAPIURL / LagoAPIKey configure the Lago adapter (required when
	// BillingAdapter == "lago").
	LagoAPIURL string
	LagoAPIKey string
	// UsageLateWindow is how far back usage timestamps may lag
	// (USAGE_LATE_WINDOW_HOURS, default 168h).
	UsageLateWindow time.Duration
	// ReconciliationInterval controls how often the reconciliation worker
	// runs consistency checks (RECONCILIATION_INTERVAL, default 1h).
	ReconciliationInterval time.Duration
	// CORSAllowedOrigins is a comma-separated list of allowed origins
	// (CORS_ALLOWED_ORIGINS, default "*").
	CORSAllowedOrigins []string
	// LogLevel controls structured log verbosity
	// (LOG_LEVEL: debug/info/warn/error, default info).
	LogLevel string
	// RateLimits configures 4-level rate limiting (spec Section 7.3).
	RateLimits RateLimitConfig
	// PSPMasterKey is the 64-char hex string (32 bytes) used for
	// AES-256-GCM encryption of PSP credentials (PSP_MASTER_KEY).
	PSPMasterKey string
	// ZITADELURL is the base URL of the ZITADEL instance for OIDC
	// verification (ZITADEL_URL). When empty, operator auth falls back
	// to the simple OPERATOR_TOKEN comparison.
	ZITADELURL string
	// ZITADELPAT is a Personal Access Token for the ZITADEL Management
	// API (ZITADEL_PAT). Required for Hosted Auth project management.
	ZITADELPAT string
	// SupportSweepInterval controls how often the JIT support session
	// expiry sweeper runs (SUPPORT_SWEEP_INTERVAL, default 30s).
	SupportSweepInterval time.Duration
	// QuotaSweepInterval controls how often the hard quota reservation
	// expiry sweeper runs (QUOTA_SWEEP_INTERVAL, default 15s).
	QuotaSweepInterval time.Duration
	// MigrationScheduleInterval controls how often the cell migration
	// scheduler checks for ready migrations (MIGRATION_SCHEDULE_INTERVAL,
	// default 5m).
	MigrationScheduleInterval time.Duration
}

// RateLimitConfig holds per-level rate limit settings. All limits are
// requests per window (default 1 minute).
type RateLimitConfig struct {
	Provider    int           // per-provider (default 1000)
	Environment int           // per-environment (default 500)
	Credential  int           // per-credential/API-key (default 200)
	Endpoint    int           // per-credential+endpoint (default 60)
	Window      time.Duration // fixed window duration (default 1m)
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		MigrationDatabaseURL: os.Getenv("MIGRATION_DATABASE_URL"),
		HTTPAddr:             envOr("HTTP_ADDR", ":8080"),
		OperatorToken:        os.Getenv("OPERATOR_TOKEN"),
		PlatformBaseDomain:   envOr("PLATFORM_BASE_DOMAIN", "platform.local"),
		OutboxPollInterval:   time.Second,
		WebhookPollInterval:  5 * time.Second,
		BillingAdapter:       envOr("BILLING_ADAPTER", "noop"),
		LagoAPIURL:           os.Getenv("LAGO_API_URL"),
		LagoAPIKey:           os.Getenv("LAGO_API_KEY"),
		UsageLateWindow:      168 * time.Hour,
		ReconciliationInterval: time.Hour,
		SupportSweepInterval:  30 * time.Second,
		QuotaSweepInterval:    15 * time.Second,
		MigrationScheduleInterval: 5 * time.Minute,
		CORSAllowedOrigins:   []string{"*"},
		LogLevel:             "info",
		RateLimits: RateLimitConfig{
			Provider:    1000,
			Environment: 500,
			Credential:  200,
			Endpoint:    60,
			Window:      time.Minute,
		},
	}
	if v := os.Getenv("OUTBOX_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid OUTBOX_POLL_INTERVAL %q: %w", v, err)
		}
		cfg.OutboxPollInterval = d
	}
	if v := os.Getenv("WEBHOOK_POLL_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid WEBHOOK_POLL_INTERVAL %q: %w", v, err)
		}
		cfg.WebhookPollInterval = d
	}
	if v := os.Getenv("USAGE_LATE_WINDOW_HOURS"); v != "" {
		hours, err := strconv.Atoi(v)
		if err != nil || hours < 0 {
			return Config{}, fmt.Errorf("invalid USAGE_LATE_WINDOW_HOURS %q: must be a non-negative integer", v)
		}
		cfg.UsageLateWindow = time.Duration(hours) * time.Hour
	}
	if v := os.Getenv("RECONCILIATION_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ReconciliationInterval = d
		}
	}
	if v := os.Getenv("SUPPORT_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.SupportSweepInterval = d
		}
	}
	if v := os.Getenv("QUOTA_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.QuotaSweepInterval = d
		}
	}
	if v := os.Getenv("MIGRATION_SCHEDULE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.MigrationScheduleInterval = d
		}
	}
	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		cfg.CORSAllowedOrigins = splitComma(v)
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	cfg.PSPMasterKey = os.Getenv("PSP_MASTER_KEY")
	cfg.ZITADELURL = os.Getenv("ZITADEL_URL")
	cfg.ZITADELPAT = os.Getenv("ZITADEL_PAT")
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

func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
