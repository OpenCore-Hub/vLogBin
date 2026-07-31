package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("OPERATOR_TOKEN", "test-token")
	os.Unsetenv("HTTP_ADDR")
	os.Unsetenv("SUPPORT_SWEEP_INTERVAL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %v, want :8080", cfg.HTTPAddr)
	}
	if cfg.SupportSweepInterval != 30*time.Second {
		t.Fatalf("SupportSweepInterval = %v, want 30s", cfg.SupportSweepInterval)
	}
	if cfg.QuotaSweepInterval != 15*time.Second {
		t.Fatalf("QuotaSweepInterval = %v, want 15s", cfg.QuotaSweepInterval)
	}
	if cfg.MigrationScheduleInterval != 5*time.Minute {
		t.Fatalf("MigrationScheduleInterval = %v, want 5m", cfg.MigrationScheduleInterval)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("OPERATOR_TOKEN", "test-token")
	t.Setenv("HTTP_ADDR", "9090")
	t.Setenv("PLATFORM_BASE_DOMAIN", "api.example.com")
	t.Setenv("SUPPORT_SWEEP_INTERVAL", "1m")
	t.Setenv("QUOTA_SWEEP_INTERVAL", "30s")
	t.Setenv("MIGRATION_SCHEDULE_INTERVAL", "10m")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.com,https://b.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.HTTPAddr != "9090" {
		t.Fatalf("HTTPAddr = %v, want 9090", cfg.HTTPAddr)
	}
	if cfg.PlatformBaseDomain != "api.example.com" {
		t.Fatalf("PlatformBaseDomain = %v, want api.example.com", cfg.PlatformBaseDomain)
	}
	if cfg.SupportSweepInterval != 1*time.Minute {
		t.Fatalf("SupportSweepInterval = %v, want 1m", cfg.SupportSweepInterval)
	}
	if cfg.QuotaSweepInterval != 30*time.Second {
		t.Fatalf("QuotaSweepInterval = %v, want 30s", cfg.QuotaSweepInterval)
	}
	if cfg.MigrationScheduleInterval != 10*time.Minute {
		t.Fatalf("MigrationScheduleInterval = %v, want 10m", cfg.MigrationScheduleInterval)
	}
	if len(cfg.CORSAllowedOrigins) != 2 {
		t.Fatalf("CORSAllowedOrigins = %v, want 2 entries", cfg.CORSAllowedOrigins)
	}
}

func TestLoadInvalidDuration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("OPERATOR_TOKEN", "test-token")
	t.Setenv("SUPPORT_SWEEP_INTERVAL", "invalid")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Invalid duration should fall back to default.
	if cfg.SupportSweepInterval != 30*time.Second {
		t.Fatalf("SupportSweepInterval = %v, want default 30s (invalid input ignored)", cfg.SupportSweepInterval)
	}
}
