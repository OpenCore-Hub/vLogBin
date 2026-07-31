package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAllEnvVars(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("MIGRATION_DATABASE_URL", "postgres://u:p@host/mig")
	t.Setenv("HTTP_ADDR", ":9999")
	t.Setenv("PLATFORM_BASE_DOMAIN", "platform.test")
	t.Setenv("BILLING_ADAPTER", "lago")
	t.Setenv("LAGO_API_URL", "http://lago:3000")
	t.Setenv("LAGO_API_KEY", "lago-key")
	t.Setenv("USAGE_LATE_WINDOW_HOURS", "72")
	t.Setenv("RECONCILIATION_INTERVAL", "2h")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("PSP_MASTER_KEY", "abcdef")
	t.Setenv("ZITADEL_URL", "https://auth.test")
	t.Setenv("ZITADEL_PAT", "pat-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MigrationDatabaseURL != "postgres://u:p@host/mig" {
		t.Fatalf("MigrationDatabaseURL = %v", cfg.MigrationDatabaseURL)
	}
	if cfg.HTTPAddr != ":9999" {
		t.Fatalf("HTTPAddr = %v", cfg.HTTPAddr)
	}
	if cfg.PlatformBaseDomain != "platform.test" {
		t.Fatalf("PlatformBaseDomain = %v", cfg.PlatformBaseDomain)
	}
	if cfg.BillingAdapter != "lago" {
		t.Fatalf("BillingAdapter = %v", cfg.BillingAdapter)
	}
	if cfg.LagoAPIURL != "http://lago:3000" {
		t.Fatalf("LagoAPIURL = %v", cfg.LagoAPIURL)
	}
	if cfg.LagoAPIKey != "lago-key" {
		t.Fatalf("LagoAPIKey = %v", cfg.LagoAPIKey)
	}
	if cfg.UsageLateWindow != 72*time.Hour {
		t.Fatalf("UsageLateWindow = %v, want 72h", cfg.UsageLateWindow)
	}
	if cfg.ReconciliationInterval != 2*time.Hour {
		t.Fatalf("ReconciliationInterval = %v, want 2h", cfg.ReconciliationInterval)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %v", cfg.LogLevel)
	}
	if cfg.PSPMasterKey != "abcdef" {
		t.Fatalf("PSPMasterKey = %v", cfg.PSPMasterKey)
	}
	if cfg.ZITADELURL != "https://auth.test" {
		t.Fatalf("ZITADELURL = %v", cfg.ZITADELURL)
	}
	if cfg.ZITADELPAT != "pat-token" {
		t.Fatalf("ZITADELPAT = %v", cfg.ZITADELPAT)
	}
}

func TestLoadMissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("OPERATOR_TOKEN", "tok")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail without DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error should mention DATABASE_URL: %v", err)
	}
}

func TestLoadMissingOperatorToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail without OPERATOR_TOKEN")
	}
	if !strings.Contains(err.Error(), "OPERATOR_TOKEN") {
		t.Fatalf("error should mention OPERATOR_TOKEN: %v", err)
	}
}

func TestLoadMigrationDatabaseURLFallback(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("MIGRATION_DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.MigrationDatabaseURL != cfg.DatabaseURL {
		t.Fatalf("MigrationDatabaseURL should fall back to DatabaseURL")
	}
}

func TestLoadInvalidOutboxPollInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("OUTBOX_POLL_INTERVAL", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail on invalid OUTBOX_POLL_INTERVAL")
	}
}

func TestLoadInvalidWebhookPollInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("WEBHOOK_POLL_INTERVAL", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail on invalid WEBHOOK_POLL_INTERVAL")
	}
}

func TestLoadInvalidUsageLateWindow(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("USAGE_LATE_WINDOW_HOURS", "invalid")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail on invalid USAGE_LATE_WINDOW_HOURS")
	}
}

func TestLoadNegativeUsageLateWindow(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("USAGE_LATE_WINDOW_HOURS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail on negative USAGE_LATE_WINDOW_HOURS")
	}
}

func TestLoadCORSOrigins(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	// With explicit origins.
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.com,https://b.com")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORSAllowedOrigins = %v, want 2 entries", cfg.CORSAllowedOrigins)
	}

	// With single origin.
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.com")
	cfg, _ = Load()
	if len(cfg.CORSAllowedOrigins) != 1 {
		t.Fatalf("single origin: got %d", len(cfg.CORSAllowedOrigins))
	}

	// With whitespace-trimmed origins.
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://a.com , https://b.com , https://c.com ")
	cfg, _ = Load()
	if len(cfg.CORSAllowedOrigins) != 3 {
		t.Fatalf("whitespace-trimmed: got %d", len(cfg.CORSAllowedOrigins))
	}

	// With empty value → falls back to default "*".
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	cfg, _ = Load()
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "*" {
		t.Fatalf("empty CORS should fall back to default '*': got %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadDefaultRateLimits(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.RateLimits.Provider != 1000 {
		t.Fatalf("Provider limit = %v, want 1000", cfg.RateLimits.Provider)
	}
	if cfg.RateLimits.Environment != 500 {
		t.Fatalf("Environment limit = %v, want 500", cfg.RateLimits.Environment)
	}
	if cfg.RateLimits.Credential != 200 {
		t.Fatalf("Credential limit = %v, want 200", cfg.RateLimits.Credential)
	}
	if cfg.RateLimits.Endpoint != 60 {
		t.Fatalf("Endpoint limit = %v, want 60", cfg.RateLimits.Endpoint)
	}
	if cfg.RateLimits.Window != time.Minute {
		t.Fatalf("Window = %v, want 1m", cfg.RateLimits.Window)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_ENV_OR_KEY", "value")
	if v := envOr("TEST_ENV_OR_KEY", "default"); v != "value" {
		t.Fatalf("envOr = %q, want value", v)
	}

	if v := envOr("TEST_ENV_OR_MISSING", "default"); v != "default" {
		t.Fatalf("envOr = %q, want default", v)
	}
}

func TestSplitComma(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a,b,c", 3},
		{"a", 1},
		{"", 0},
		{"a, b , c", 3}, // trims whitespace
		{"  ", 0},
		{",,,", 0},
	}

	for _, tt := range tests {
		got := splitComma(tt.input)
		if len(got) != tt.want {
			t.Fatalf("splitComma(%q) = %v, want %d entries", tt.input, got, tt.want)
		}
	}
}
