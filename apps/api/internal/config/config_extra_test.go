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
	t.Setenv("AUTH_VAULT_SERVICE_TOKEN", "vault-service-token")
	t.Setenv("AUTH_VAULT_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("AUTH_VAULT_MASTER_KEY_PREVIOUS", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	t.Setenv("AUTH_VAULT_SWEEP_INTERVAL", "30m")
	t.Setenv("AUDIT_RETENTION_DAYS", "730")
	t.Setenv("AUDIT_RETENTION_SWEEP_INTERVAL", "6h")

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
	if cfg.AuthVaultServiceToken != "vault-service-token" {
		t.Fatalf("AuthVaultServiceToken = %v", cfg.AuthVaultServiceToken)
	}
	if cfg.AuthVaultMasterKey == "" {
		t.Fatal("AuthVaultMasterKey = empty")
	}
	if len(cfg.AuthVaultMasterKeyPrevious) != 1 {
		t.Fatalf("AuthVaultMasterKeyPrevious = %v", cfg.AuthVaultMasterKeyPrevious)
	}
	if cfg.AuthVaultSweepInterval != 30*time.Minute {
		t.Fatalf("AuthVaultSweepInterval = %v", cfg.AuthVaultSweepInterval)
	}
	if cfg.AuditRetentionDays != 730 {
		t.Fatalf("AuditRetentionDays = %v, want 730", cfg.AuditRetentionDays)
	}
	if cfg.AuditRetentionSweepInterval != 6*time.Hour {
		t.Fatalf("AuditRetentionSweepInterval = %v, want 6h", cfg.AuditRetentionSweepInterval)
	}
}

func TestAuditRetentionDefaultsOff(t *testing.T) {
	// Audit events are compliance evidence: retention must default to off so
	// an unconfigured deployment never silently deletes them.
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuditRetentionDays != 0 {
		t.Fatalf("AuditRetentionDays = %v, want default 0 (disabled)", cfg.AuditRetentionDays)
	}
	if cfg.AuditRetentionSweepInterval != time.Hour {
		t.Fatalf("AuditRetentionSweepInterval = %v, want default 1h", cfg.AuditRetentionSweepInterval)
	}
}

func TestAuditRetentionInvalidDays(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("AUDIT_RETENTION_DAYS", "-5")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail with negative AUDIT_RETENTION_DAYS")
	}
	if !strings.Contains(err.Error(), "AUDIT_RETENTION_DAYS") {
		t.Fatalf("error should mention AUDIT_RETENTION_DAYS: %v", err)
	}
}

func TestAuditChainAnchorIntervalParsed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("AUDIT_CHAIN_ANCHOR_INTERVAL", "6h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuditChainAnchorInterval != 6*time.Hour {
		t.Fatalf("AuditChainAnchorInterval = %v, want 6h", cfg.AuditChainAnchorInterval)
	}
}

func TestAuditChainAnchorIntervalDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuditChainAnchorInterval != 24*time.Hour {
		t.Fatalf("AuditChainAnchorInterval = %v, want default 24h", cfg.AuditChainAnchorInterval)
	}
}

func TestAuditChainAnchorIntervalInvalid(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("AUDIT_CHAIN_ANCHOR_INTERVAL", "-1h")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail with negative AUDIT_CHAIN_ANCHOR_INTERVAL")
	}
	if !strings.Contains(err.Error(), "AUDIT_CHAIN_ANCHOR_INTERVAL") {
		t.Fatalf("error should mention AUDIT_CHAIN_ANCHOR_INTERVAL: %v", err)
	}
}

func TestWorkerBackoffMaxParsed(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("WORKER_BACKOFF_MAX", "90s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WorkerBackoffMax != 90*time.Second {
		t.Fatalf("WorkerBackoffMax = %v, want 90s", cfg.WorkerBackoffMax)
	}
}

func TestWorkerBackoffMaxDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WorkerBackoffMax != 30*time.Second {
		t.Fatalf("WorkerBackoffMax = %v, want default 30s", cfg.WorkerBackoffMax)
	}
}

func TestWorkerBackoffMaxInvalid(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@host/db")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("WORKER_BACKOFF_MAX", "-5s")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail with negative WORKER_BACKOFF_MAX")
	}
	if !strings.Contains(err.Error(), "WORKER_BACKOFF_MAX") {
		t.Fatalf("error should mention WORKER_BACKOFF_MAX: %v", err)
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
	if cfg.RateLimits.IP != 6000 {
		t.Fatalf("IP limit = %v, want 6000", cfg.RateLimits.IP)
	}
	if cfg.RateLimits.Window != time.Minute {
		t.Fatalf("Window = %v, want 1m", cfg.RateLimits.Window)
	}
}

func TestLoadRateLimitIPOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("RL_IP_LIMIT", "250")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RateLimits.IP != 250 {
		t.Fatalf("IP limit = %v, want 250", cfg.RateLimits.IP)
	}

	t.Setenv("RL_IP_LIMIT", "0")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load with RL_IP_LIMIT=0: %v", err)
	}
	if cfg.RateLimits.IP != 0 {
		t.Fatalf("IP limit = %v, want 0 (disabled)", cfg.RateLimits.IP)
	}

	t.Setenv("RL_IP_LIMIT", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("Load with invalid RL_IP_LIMIT must fail")
	}
}

func TestLoadRateLimitAllLayersOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("RL_PROVIDER_LIMIT", "111")
	t.Setenv("RL_ENVIRONMENT_LIMIT", "222")
	t.Setenv("RL_CREDENTIAL_LIMIT", "333")
	t.Setenv("RL_ENDPOINT_LIMIT", "444")
	t.Setenv("RL_IP_LIMIT", "555")
	t.Setenv("RL_WINDOW", "90s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RateLimits.Provider != 111 || cfg.RateLimits.Environment != 222 ||
		cfg.RateLimits.Credential != 333 || cfg.RateLimits.Endpoint != 444 ||
		cfg.RateLimits.IP != 555 {
		t.Fatalf("rate limits not applied: %+v", cfg.RateLimits)
	}
	if cfg.RateLimits.Window != 90*time.Second {
		t.Fatalf("Window = %v, want 90s", cfg.RateLimits.Window)
	}
}

func TestLoadRateLimitInvalidValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	for _, env := range []struct{ k, v string }{
		{"RL_PROVIDER_LIMIT", "abc"},
		{"RL_ENVIRONMENT_LIMIT", "-1"},
		{"RL_CREDENTIAL_LIMIT", "1.5"},
		{"RL_ENDPOINT_LIMIT", "12x"},
		{"RL_WINDOW", "0s"},
		{"RL_WINDOW", "-1m"},
		{"RL_WINDOW", "soon"},
	} {
		t.Setenv(env.k, env.v)
		if _, err := Load(); err == nil {
			t.Fatalf("Load with %s=%q must fail", env.k, env.v)
		}
	}
}

func TestLoadConfigReloadInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigReloadInterval != 0 {
		t.Fatalf("default ConfigReloadInterval = %v, want 0", cfg.ConfigReloadInterval)
	}

	t.Setenv("CONFIG_RELOAD_INTERVAL", "15s")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load with CONFIG_RELOAD_INTERVAL: %v", err)
	}
	if cfg.ConfigReloadInterval != 15*time.Second {
		t.Fatalf("ConfigReloadInterval = %v, want 15s", cfg.ConfigReloadInterval)
	}

	t.Setenv("CONFIG_RELOAD_INTERVAL", "bogus")
	if _, err := Load(); err == nil {
		t.Fatal("Load with invalid CONFIG_RELOAD_INTERVAL must fail")
	}
}

func TestLoadCircuitBreakerDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("CB_FAILURE_THRESHOLD", "")
	t.Setenv("CB_OPEN_TIMEOUT", "")
	t.Setenv("CB_HALF_OPEN_MAX", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CircuitBreakers.FailureThreshold != 5 {
		t.Fatalf("FailureThreshold = %v, want 5", cfg.CircuitBreakers.FailureThreshold)
	}
	if cfg.CircuitBreakers.OpenTimeout != 30*time.Second {
		t.Fatalf("OpenTimeout = %v, want 30s", cfg.CircuitBreakers.OpenTimeout)
	}
	if cfg.CircuitBreakers.HalfOpenMax != 1 {
		t.Fatalf("HalfOpenMax = %v, want 1", cfg.CircuitBreakers.HalfOpenMax)
	}
	opts := cfg.CircuitBreakers.ToOptions()
	if opts.FailureThreshold != 5 || opts.OpenTimeout != 30*time.Second || opts.HalfOpenMax != 1 {
		t.Fatalf("ToOptions = %+v, want defaults", opts)
	}
}

func TestLoadCircuitBreakerOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("CB_FAILURE_THRESHOLD", "7")
	t.Setenv("CB_OPEN_TIMEOUT", "90s")
	t.Setenv("CB_HALF_OPEN_MAX", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CircuitBreakers.FailureThreshold != 7 {
		t.Fatalf("FailureThreshold = %v, want 7", cfg.CircuitBreakers.FailureThreshold)
	}
	if cfg.CircuitBreakers.OpenTimeout != 90*time.Second {
		t.Fatalf("OpenTimeout = %v, want 90s", cfg.CircuitBreakers.OpenTimeout)
	}
	if cfg.CircuitBreakers.HalfOpenMax != 3 {
		t.Fatalf("HalfOpenMax = %v, want 3", cfg.CircuitBreakers.HalfOpenMax)
	}
}

func TestLoadInvalidCircuitBreakers(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	for _, tc := range []struct{ key, value string }{
		{"CB_FAILURE_THRESHOLD", "0"},
		{"CB_FAILURE_THRESHOLD", "-3"},
		{"CB_FAILURE_THRESHOLD", "abc"},
		{"CB_OPEN_TIMEOUT", "soon"},
		{"CB_HALF_OPEN_MAX", "0"},
		{"CB_HALF_OPEN_MAX", "-1"},
		{"CB_HALF_OPEN_MAX", "many"},
	} {
		t.Setenv(tc.key, tc.value)
		if _, err := Load(); err == nil {
			t.Errorf("Load with %s=%q must fail", tc.key, tc.value)
		}
		t.Setenv(tc.key, "")
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

func TestLoadMetricsEnabled(t *testing.T) {
	// Defaults: enabled, 30s sweep.
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("METRICS_ENABLED", "")
	t.Setenv("METRICS_SWEEP_INTERVAL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.MetricsEnabled {
		t.Fatal("MetricsEnabled should default to true")
	}
	if cfg.MetricsSweepInterval != 30*time.Second {
		t.Fatalf("MetricsSweepInterval = %v, want 30s", cfg.MetricsSweepInterval)
	}

	// Explicit disable.
	t.Setenv("METRICS_ENABLED", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MetricsEnabled {
		t.Fatal("MetricsEnabled should be false when METRICS_ENABLED=false")
	}

	// Explicit enable with custom sweep interval.
	t.Setenv("METRICS_ENABLED", "true")
	t.Setenv("METRICS_SWEEP_INTERVAL", "1m")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.MetricsEnabled {
		t.Fatal("MetricsEnabled should be true when METRICS_ENABLED=true")
	}
	if cfg.MetricsSweepInterval != time.Minute {
		t.Fatalf("MetricsSweepInterval = %v, want 1m", cfg.MetricsSweepInterval)
	}
}

func TestLoadInvalidMetricsEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("METRICS_ENABLED", "not-a-bool")

	_, err := Load()
	if err == nil {
		t.Fatal("should fail on invalid METRICS_ENABLED")
	}
	if !strings.Contains(err.Error(), "METRICS_ENABLED") {
		t.Fatalf("error should mention METRICS_ENABLED: %v", err)
	}
}

func TestLoadPoolConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	for _, k := range []string{
		"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME",
		"DB_MAX_CONN_IDLE_TIME", "DB_HEALTH_CHECK_PERIOD",
		"DB_QUERY_TIMEOUT", "HTTP_REQUEST_TIMEOUT",
	} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBMaxConns != 10 || cfg.DBMinConns != 1 {
		t.Fatalf("pool sizes = (%d, %d), want (10, 1)", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetime != 30*time.Minute || cfg.DBMaxConnIdleTime != 5*time.Minute || cfg.DBHealthCheckPeriod != time.Minute {
		t.Fatalf("pool durations = %v %v %v, want 30m/5m/1m", cfg.DBMaxConnLifetime, cfg.DBMaxConnIdleTime, cfg.DBHealthCheckPeriod)
	}
	if cfg.DBQueryTimeout != 10*time.Second || cfg.HTTPRequestTimeout != 30*time.Second {
		t.Fatalf("timeouts = %v/%v, want 10s/30s", cfg.DBQueryTimeout, cfg.HTTPRequestTimeout)
	}

	t.Setenv("DB_MAX_CONNS", "25")
	t.Setenv("DB_MIN_CONNS", "5")
	t.Setenv("DB_MAX_CONN_LIFETIME", "1h")
	t.Setenv("DB_MAX_CONN_IDLE_TIME", "90s")
	t.Setenv("DB_HEALTH_CHECK_PERIOD", "15s")
	t.Setenv("DB_QUERY_TIMEOUT", "45s")
	t.Setenv("HTTP_REQUEST_TIMEOUT", "2m")

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DBMaxConns != 25 || cfg.DBMinConns != 5 || cfg.DBMaxConnLifetime != time.Hour ||
		cfg.DBMaxConnIdleTime != 90*time.Second || cfg.DBHealthCheckPeriod != 15*time.Second ||
		cfg.DBQueryTimeout != 45*time.Second || cfg.HTTPRequestTimeout != 2*time.Minute {
		t.Fatalf("overrides not applied: max=%d min=%d life=%v idle=%v hc=%v qt=%v ht=%v",
			cfg.DBMaxConns, cfg.DBMinConns, cfg.DBMaxConnLifetime, cfg.DBMaxConnIdleTime,
			cfg.DBHealthCheckPeriod, cfg.DBQueryTimeout, cfg.HTTPRequestTimeout)
	}
}

func TestLoadInvalidPoolConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	invalid := []struct{ key, value string }{
		{"DB_MAX_CONNS", "abc"},
		{"DB_MAX_CONNS", "-1"},
		{"DB_MIN_CONNS", "xyz"},
		{"DB_MAX_CONN_LIFETIME", "not-a-duration"},
		{"DB_QUERY_TIMEOUT", "soon"},
		{"HTTP_REQUEST_TIMEOUT", "10"},
	}
	for _, tt := range invalid {
		t.Setenv(tt.key, tt.value)
		if _, err := Load(); err == nil {
			t.Fatalf("Load should fail for %s=%q", tt.key, tt.value)
		}
		t.Setenv(tt.key, "")
	}

	t.Setenv("DB_MIN_CONNS", "50")
	t.Setenv("DB_MAX_CONNS", "5")
	if _, err := Load(); err == nil {
		t.Fatal("Load should fail when DB_MIN_CONNS > DB_MAX_CONNS")
	}
}

func TestLoadHTTPServerHardening(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	for _, k := range []string{
		"HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT", "HTTP_IDLE_TIMEOUT",
		"HTTP_MAX_HEADER_BYTES", "SHUTDOWN_GRACE_PERIOD", "READY_DB_TIMEOUT",
	} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPReadTimeout != 30*time.Second || cfg.HTTPWriteTimeout != 30*time.Second {
		t.Fatalf("read/write timeouts = %v/%v, want 30s/30s", cfg.HTTPReadTimeout, cfg.HTTPWriteTimeout)
	}
	if cfg.HTTPIdleTimeout != 120*time.Second {
		t.Fatalf("idle timeout = %v, want 120s", cfg.HTTPIdleTimeout)
	}
	if cfg.HTTPMaxHeaderBytes != 1<<20 {
		t.Fatalf("max header bytes = %d, want 1 MiB", cfg.HTTPMaxHeaderBytes)
	}
	if cfg.ShutdownGracePeriod != 20*time.Second || cfg.ReadyTimeout != 2*time.Second {
		t.Fatalf("shutdown/ready = %v/%v, want 20s/2s", cfg.ShutdownGracePeriod, cfg.ReadyTimeout)
	}

	t.Setenv("HTTP_READ_TIMEOUT", "10s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "15s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "60s")
	t.Setenv("HTTP_MAX_HEADER_BYTES", "65536")
	t.Setenv("SHUTDOWN_GRACE_PERIOD", "45s")
	t.Setenv("READY_DB_TIMEOUT", "5s")

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPReadTimeout != 10*time.Second || cfg.HTTPWriteTimeout != 15*time.Second ||
		cfg.HTTPIdleTimeout != 60*time.Second || cfg.HTTPMaxHeaderBytes != 65536 ||
		cfg.ShutdownGracePeriod != 45*time.Second || cfg.ReadyTimeout != 5*time.Second {
		t.Fatalf("overrides not applied: read=%v write=%v idle=%v maxhdr=%d grace=%v ready=%v",
			cfg.HTTPReadTimeout, cfg.HTTPWriteTimeout, cfg.HTTPIdleTimeout,
			cfg.HTTPMaxHeaderBytes, cfg.ShutdownGracePeriod, cfg.ReadyTimeout)
	}
}

func TestLoadInvalidHTTPServerHardening(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	invalid := []struct{ key, value string }{
		{"HTTP_READ_TIMEOUT", "not-a-duration"},
		{"HTTP_WRITE_TIMEOUT", "soon"},
		{"HTTP_IDLE_TIMEOUT", "idle"},
		{"HTTP_MAX_HEADER_BYTES", "abc"},
		{"HTTP_MAX_HEADER_BYTES", "-5"},
		{"SHUTDOWN_GRACE_PERIOD", "grace"},
		{"READY_DB_TIMEOUT", "never"},
	}
	for _, tt := range invalid {
		t.Setenv(tt.key, tt.value)
		if _, err := Load(); err == nil {
			t.Fatalf("Load should fail for %s=%q", tt.key, tt.value)
		}
		t.Setenv(tt.key, "")
	}
}

func TestLoadPprofDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("PPROF_ENABLED", "")
	t.Setenv("PPROF_ADDR", "")
	t.Setenv("SLOW_REQUEST_THRESHOLD", "")
	t.Setenv("DB_SLOW_QUERY_THRESHOLD", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PprofEnabled {
		t.Fatal("pprof must default to disabled")
	}
	if cfg.PprofAddr != ":6060" {
		t.Fatalf("PprofAddr = %q, want :6060", cfg.PprofAddr)
	}
	if cfg.SlowRequestThreshold != 0 {
		t.Fatalf("SlowRequestThreshold = %v, want 0 (disabled)", cfg.SlowRequestThreshold)
	}
	if cfg.DBQuerySlowThreshold != 0 {
		t.Fatalf("DBQuerySlowThreshold = %v, want 0 (disabled)", cfg.DBQuerySlowThreshold)
	}
}

func TestLoadPprofAndSlowRequestOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("PPROF_ENABLED", "true")
	t.Setenv("PPROF_ADDR", "127.0.0.1:9090")
	t.Setenv("SLOW_REQUEST_THRESHOLD", "250ms")
	t.Setenv("DB_SLOW_QUERY_THRESHOLD", "150ms")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.PprofEnabled {
		t.Fatal("pprof must be enabled")
	}
	if cfg.PprofAddr != "127.0.0.1:9090" {
		t.Fatalf("PprofAddr = %q, want 127.0.0.1:9090", cfg.PprofAddr)
	}
	if cfg.SlowRequestThreshold != 250*time.Millisecond {
		t.Fatalf("SlowRequestThreshold = %v, want 250ms", cfg.SlowRequestThreshold)
	}
	if cfg.DBQuerySlowThreshold != 150*time.Millisecond {
		t.Fatalf("DBQuerySlowThreshold = %v, want 150ms", cfg.DBQuerySlowThreshold)
	}
}

func TestLoadInvalidPprofAndSlowRequest(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	invalid := []struct{ key, value string }{
		{"PPROF_ENABLED", "not-a-bool"},
		{"SLOW_REQUEST_THRESHOLD", "not-a-duration"},
		{"DB_SLOW_QUERY_THRESHOLD", "not-a-duration"},
	}
	for _, tt := range invalid {
		t.Setenv(tt.key, tt.value)
		if _, err := Load(); err == nil {
			t.Fatalf("Load should fail for %s=%q", tt.key, tt.value)
		}
		t.Setenv(tt.key, "")
	}
}

func TestLoadTelemetryDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	for _, k := range []string{
		"OTEL_ENABLED", "OTEL_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_SERVICE_NAME", "OTEL_ENVIRONMENT", "OTEL_SAMPLE_RATIO",
		"OTEL_BATCH_TIMEOUT", "OTEL_EXPORT_TIMEOUT",
		"OTEL_MAX_QUEUE_SIZE", "OTEL_MAX_EXPORT_BATCH_SIZE",
	} {
		t.Setenv(k, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tel := cfg.Telemetry
	if tel.Enabled {
		t.Fatal("tracing must default to disabled")
	}
	if tel.Exporter != "otlp" {
		t.Fatalf("Exporter = %q, want otlp", tel.Exporter)
	}
	if tel.ServiceName != "vlogbin-api" {
		t.Fatalf("ServiceName = %q, want vlogbin-api", tel.ServiceName)
	}
	if tel.Environment != "development" {
		t.Fatalf("Environment = %q, want development", tel.Environment)
	}
	if tel.SampleRatio != 1 {
		t.Fatalf("SampleRatio = %v, want 1", tel.SampleRatio)
	}
	if tel.BatchTimeout != 5*time.Second || tel.ExportTimeout != 30*time.Second {
		t.Fatalf("timeouts = %v/%v, want 5s/30s", tel.BatchTimeout, tel.ExportTimeout)
	}
	if tel.MaxQueueSize != 2048 || tel.MaxExportBatchSize != 512 {
		t.Fatalf("batch sizes = %d/%d, want 2048/512", tel.MaxQueueSize, tel.MaxExportBatchSize)
	}
}

func TestLoadTelemetryOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER", "stdout")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
	t.Setenv("OTEL_SERVICE_NAME", "my-api")
	t.Setenv("OTEL_ENVIRONMENT", "staging")
	t.Setenv("OTEL_SAMPLE_RATIO", "0.25")
	t.Setenv("OTEL_BATCH_TIMEOUT", "2s")
	t.Setenv("OTEL_EXPORT_TIMEOUT", "10s")
	t.Setenv("OTEL_MAX_QUEUE_SIZE", "100")
	t.Setenv("OTEL_MAX_EXPORT_BATCH_SIZE", "50")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tel := cfg.Telemetry
	if !tel.Enabled {
		t.Fatal("tracing must be enabled")
	}
	if tel.Exporter != "stdout" {
		t.Fatalf("Exporter = %q, want stdout", tel.Exporter)
	}
	if tel.OTLPEndpoint != "http://collector:4318" {
		t.Fatalf("OTLPEndpoint = %q", tel.OTLPEndpoint)
	}
	if tel.ServiceName != "my-api" || tel.Environment != "staging" {
		t.Fatalf("identity = %q/%q, want my-api/staging", tel.ServiceName, tel.Environment)
	}
	if tel.SampleRatio != 0.25 {
		t.Fatalf("SampleRatio = %v, want 0.25", tel.SampleRatio)
	}
	if tel.BatchTimeout != 2*time.Second || tel.ExportTimeout != 10*time.Second {
		t.Fatalf("timeouts = %v/%v, want 2s/10s", tel.BatchTimeout, tel.ExportTimeout)
	}
	if tel.MaxQueueSize != 100 || tel.MaxExportBatchSize != 50 {
		t.Fatalf("batch sizes = %d/%d, want 100/50", tel.MaxQueueSize, tel.MaxExportBatchSize)
	}
}

func TestLoadInvalidTelemetry(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	invalid := []struct{ key, value string }{
		{"OTEL_ENABLED", "not-a-bool"},
		{"OTEL_EXPORTER", "bogus"},
		{"OTEL_SAMPLE_RATIO", "1.5"},
		{"OTEL_SAMPLE_RATIO", "-0.1"},
		{"OTEL_SAMPLE_RATIO", "half"},
		{"OTEL_BATCH_TIMEOUT", "soon"},
		{"OTEL_EXPORT_TIMEOUT", "never"},
		{"OTEL_MAX_QUEUE_SIZE", "0"},
		{"OTEL_MAX_QUEUE_SIZE", "many"},
		{"OTEL_MAX_EXPORT_BATCH_SIZE", "-1"},
	}
	for _, tt := range invalid {
		t.Setenv(tt.key, tt.value)
		if _, err := Load(); err == nil {
			t.Errorf("Load should fail for %s=%q", tt.key, tt.value)
		}
		t.Setenv(tt.key, "")
	}
}

func TestLoadRateLimitBackend(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")
	t.Setenv("RATE_LIMIT_BACKEND", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "")

	// Default backend is memory with zeroed Redis settings.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RateLimitBackend != "memory" {
		t.Fatalf("default RateLimitBackend = %q, want memory", cfg.RateLimitBackend)
	}

	// Redis backend with all settings applied.
	t.Setenv("RATE_LIMIT_BACKEND", "redis")
	t.Setenv("REDIS_ADDR", "cache:6379")
	t.Setenv("REDIS_PASSWORD", "pw")
	t.Setenv("REDIS_DB", "3")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load with redis backend: %v", err)
	}
	if cfg.RateLimitBackend != "redis" || cfg.RedisAddr != "cache:6379" ||
		cfg.RedisPassword != "pw" || cfg.RedisDB != 3 {
		t.Fatalf("redis config not applied: %+v", cfg)
	}
}

func TestLoadRateLimitBackendValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	invalid := []struct{ key, value string }{
		{"RATE_LIMIT_BACKEND", "memcached"},
		{"RATE_LIMIT_BACKEND", "Redis"},
		{"REDIS_DB", "-1"},
		{"REDIS_DB", "x"},
	}
	for _, tt := range invalid {
		t.Setenv(tt.key, tt.value)
		if _, err := Load(); err == nil {
			t.Fatalf("Load should fail for %s=%q", tt.key, tt.value)
		}
		t.Setenv(tt.key, "")
	}

	// A redis backend without an address is a deployment error.
	t.Setenv("RATE_LIMIT_BACKEND", "redis")
	t.Setenv("REDIS_ADDR", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load should fail for RATE_LIMIT_BACKEND=redis without REDIS_ADDR")
	}
}

func TestLoadPSPMasterKeyPrevious(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	// Default: no previous keys.
	t.Setenv("PSP_MASTER_KEY_PREVIOUS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.PSPMasterKeyPrevious) != 0 {
		t.Fatalf("default PSPMasterKeyPrevious = %v, want empty", cfg.PSPMasterKeyPrevious)
	}

	// Single previous key.
	t.Setenv("PSP_MASTER_KEY_PREVIOUS", "aa11bb22cc33dd44ee55ff6677889900aabbccddeeff0011223344556677889900")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.PSPMasterKeyPrevious) != 1 {
		t.Fatalf("PSPMasterKeyPrevious = %v, want 1 entry", cfg.PSPMasterKeyPrevious)
	}

	// Multiple keys with whitespace, trimmed.
	t.Setenv("PSP_MASTER_KEY_PREVIOUS", " k1 , k2 ,k3 ")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.PSPMasterKeyPrevious) != 3 {
		t.Fatalf("PSPMasterKeyPrevious = %v, want 3 entries", cfg.PSPMasterKeyPrevious)
	}
	if cfg.PSPMasterKeyPrevious[1] != "k2" {
		t.Fatalf("PSPMasterKeyPrevious[1] = %q, want k2 (whitespace trimmed)", cfg.PSPMasterKeyPrevious[1])
	}
}

func TestLoadReencryptConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	// Defaults: worker disabled, batch size 100.
	t.Setenv("REENCRYPT_SWEEP_INTERVAL", "")
	t.Setenv("REENCRYPT_BATCH_SIZE", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ReencryptSweepInterval != 0 {
		t.Fatalf("default ReencryptSweepInterval = %v, want 0 (disabled)", cfg.ReencryptSweepInterval)
	}
	if cfg.ReencryptBatchSize != 100 {
		t.Fatalf("default ReencryptBatchSize = %d, want 100", cfg.ReencryptBatchSize)
	}

	// Explicit values parse.
	t.Setenv("REENCRYPT_SWEEP_INTERVAL", "30m")
	t.Setenv("REENCRYPT_BATCH_SIZE", "250")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ReencryptSweepInterval != 30*time.Minute {
		t.Fatalf("ReencryptSweepInterval = %v, want 30m", cfg.ReencryptSweepInterval)
	}
	if cfg.ReencryptBatchSize != 250 {
		t.Fatalf("ReencryptBatchSize = %d, want 250", cfg.ReencryptBatchSize)
	}

	// Invalid values are rejected.
	t.Setenv("REENCRYPT_SWEEP_INTERVAL", "-5s")
	if _, err := Load(); err == nil {
		t.Fatal("negative REENCRYPT_SWEEP_INTERVAL must be rejected")
	}
	t.Setenv("REENCRYPT_SWEEP_INTERVAL", "30m")
	t.Setenv("REENCRYPT_BATCH_SIZE", "0")
	if _, err := Load(); err == nil {
		t.Fatal("REENCRYPT_BATCH_SIZE=0 must be rejected")
	}
}

func TestLoadAuditArchiveConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	// Defaults: worker disabled, batch size 100, SSL on.
	t.Setenv("AUDIT_ARCHIVE_SWEEP_INTERVAL", "")
	t.Setenv("AUDIT_ARCHIVE_BATCH_SIZE", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuditArchiveSweepInterval != 0 {
		t.Fatalf("default AuditArchiveSweepInterval = %v, want 0 (disabled)", cfg.AuditArchiveSweepInterval)
	}
	if cfg.AuditArchiveBatchSize != 100 {
		t.Fatalf("default AuditArchiveBatchSize = %d, want 100", cfg.AuditArchiveBatchSize)
	}
	if !cfg.AuditArchiveObjectStorage.UseSSL {
		t.Fatal("default AuditArchiveObjectStorage.UseSSL must be true")
	}

	// Explicit values parse.
	t.Setenv("AUDIT_ARCHIVE_SWEEP_INTERVAL", "15m")
	t.Setenv("AUDIT_ARCHIVE_BATCH_SIZE", "50")
	t.Setenv("AUDIT_ARCHIVE_S3_ENDPOINT", "https://minio.internal:9000")
	t.Setenv("AUDIT_ARCHIVE_S3_BUCKET", "audit-worm")
	t.Setenv("AUDIT_ARCHIVE_S3_ACCESS_KEY", "ak")
	t.Setenv("AUDIT_ARCHIVE_S3_SECRET_KEY", "sk")
	t.Setenv("AUDIT_ARCHIVE_S3_REGION", "us-east-1")
	t.Setenv("AUDIT_ARCHIVE_S3_USE_SSL", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuditArchiveSweepInterval != 15*time.Minute {
		t.Fatalf("AuditArchiveSweepInterval = %v, want 15m", cfg.AuditArchiveSweepInterval)
	}
	if cfg.AuditArchiveBatchSize != 50 {
		t.Fatalf("AuditArchiveBatchSize = %d, want 50", cfg.AuditArchiveBatchSize)
	}
	o := cfg.AuditArchiveObjectStorage
	if o.Endpoint != "https://minio.internal:9000" || o.Bucket != "audit-worm" ||
		o.AccessKey != "ak" || o.SecretKey != "sk" || o.Region != "us-east-1" || o.UseSSL {
		t.Fatalf("object storage config not applied: %+v", o)
	}
}

func TestLoadAuditArchiveValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	// Enabling the worker without storage config must fail (fail-closed: a
	// silently misconfigured archiver would never publish anchors).
	t.Setenv("AUDIT_ARCHIVE_SWEEP_INTERVAL", "15m")
	t.Setenv("AUDIT_ARCHIVE_S3_ENDPOINT", "")
	if _, err := Load(); err == nil {
		t.Fatal("AUDIT_ARCHIVE_SWEEP_INTERVAL without S3 config must be rejected")
	}

	// Partial storage config must also fail.
	t.Setenv("AUDIT_ARCHIVE_S3_ENDPOINT", "https://minio.internal:9000")
	t.Setenv("AUDIT_ARCHIVE_S3_BUCKET", "audit-worm")
	t.Setenv("AUDIT_ARCHIVE_S3_ACCESS_KEY", "ak")
	t.Setenv("AUDIT_ARCHIVE_S3_SECRET_KEY", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing AUDIT_ARCHIVE_S3_SECRET_KEY must be rejected")
	}

	// Invalid values are rejected.
	t.Setenv("AUDIT_ARCHIVE_SWEEP_INTERVAL", "-5s")
	t.Setenv("AUDIT_ARCHIVE_S3_SECRET_KEY", "sk")
	if _, err := Load(); err == nil {
		t.Fatal("negative AUDIT_ARCHIVE_SWEEP_INTERVAL must be rejected")
	}
	t.Setenv("AUDIT_ARCHIVE_SWEEP_INTERVAL", "15m")
	t.Setenv("AUDIT_ARCHIVE_BATCH_SIZE", "0")
	if _, err := Load(); err == nil {
		t.Fatal("AUDIT_ARCHIVE_BATCH_SIZE=0 must be rejected")
	}
	t.Setenv("AUDIT_ARCHIVE_BATCH_SIZE", "100")
	t.Setenv("AUDIT_ARCHIVE_S3_USE_SSL", "not-a-bool")
	if _, err := Load(); err == nil {
		t.Fatal("invalid AUDIT_ARCHIVE_S3_USE_SSL must be rejected")
	}
}

func TestLoadPortalTokenConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("OPERATOR_TOKEN", "tok")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PortalTokenTTL != 24*time.Hour {
		t.Fatalf("PortalTokenTTL = %v, want 24h", cfg.PortalTokenTTL)
	}

	t.Setenv("PORTAL_TOKEN_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("PORTAL_TOKEN_TTL", "2h")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load override: %v", err)
	}
	if cfg.PortalTokenSecret == "" || cfg.PortalTokenTTL != 2*time.Hour {
		t.Fatalf("portal config not applied: secret=%q ttl=%v", cfg.PortalTokenSecret, cfg.PortalTokenTTL)
	}

	t.Setenv("PORTAL_TOKEN_TTL", "not-a-duration")
	if _, err := Load(); err == nil {
		t.Fatal("invalid PORTAL_TOKEN_TTL must be rejected")
	}
}
