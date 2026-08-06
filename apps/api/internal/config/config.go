// Package config loads platform API configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/circuitbreaker"
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
	// CircuitBreakers configures the circuit breakers protecting outbound
	// dependencies: per-endpoint webhook delivery and the billing engine.
	CircuitBreakers CircuitBreakerConfig
	// Telemetry configures OpenTelemetry distributed tracing. Tracing is
	// opt-in (OTEL_ENABLED, default false) so deployments without a collector
	// keep the no-op tracer and pay negligible overhead.
	Telemetry TelemetryConfig
	// PSPMasterKey is the 64-char hex string (32 bytes) used for
	// AES-256-GCM encryption of PSP credentials (PSP_MASTER_KEY).
	PSPMasterKey string
	// PSPMasterKeyPrevious lists 64-char hex master keys that were active in
	// the past (PSP_MASTER_KEY_PREVIOUS, comma separated). They are retained
	// solely to decrypt ciphertext written before the last rotation; new
	// credentials are always encrypted with PSPMasterKey.
	PSPMasterKeyPrevious []string
	// PortalTokenSecret is the HMAC secret used to sign customer portal
	// tokens (PORTAL_TOKEN_SECRET). When empty, portal endpoints are disabled.
	PortalTokenSecret string
	// PortalTokenTTL controls how long a customer portal token stays valid
	// (PORTAL_TOKEN_TTL, default 24h).
	PortalTokenTTL time.Duration
	// ZITADELURL is the base URL of the ZITADEL instance for OIDC
	// verification (ZITADEL_URL). When empty, operator auth falls back
	// to the simple OPERATOR_TOKEN comparison.
	ZITADELURL string
	// ZITADELPAT is a Personal Access Token for the ZITADEL Management
	// API (ZITADEL_PAT). Required for Hosted Auth project management.
	ZITADELPAT string
	// AuthVaultServiceToken is the shared secret used by the vLogBin web
	// backend to create/read/delete server-side OIDC token vault entries.
	AuthVaultServiceToken string
	// AuthVaultPublicKey is the PEM-encoded RSA public key used to verify
	// short-lived JWTs signed by the vLogBin web backend. Prefer over the
	// static shared secret.
	AuthVaultPublicKey string
	// AuthVaultAudience is the audience required in web-signed vault JWTs.
	AuthVaultAudience string
	// AuthVaultMasterKey is the AES-GCM key used to encrypt OIDC tokens in the
	// server-side vault. Independent from PSP_MASTER_KEY for separate rotation.
	AuthVaultMasterKey string
	// AuthVaultMasterKeyPrevious lists previous vault keys for decryption-only
	// fallback during rotation.
	AuthVaultMasterKeyPrevious []string
	// AuthVaultSweepInterval controls how often expired vault rows are purged.
	AuthVaultSweepInterval time.Duration
	// SupportSweepInterval controls how often the JIT support session
	// expiry sweeper runs (SUPPORT_SWEEP_INTERVAL, default 30s).
	SupportSweepInterval time.Duration
	// QuotaSweepInterval controls how often the hard quota reservation
	// expiry sweeper runs (QUOTA_SWEEP_INTERVAL, default 15s).
	QuotaSweepInterval time.Duration
	// WebhookRetentionDays controls how long terminal webhook deliveries and
	// terminal outbox events are kept before the retention sweeper purges
	// them (WEBHOOK_RETENTION_DAYS, default 30d). Non-terminal rows (pending,
	// or failed rows still inside their retry window) are never purged.
	WebhookRetentionDays int
	// WebhookRetentionSweepInterval controls how often the webhook retention
	// sweeper runs (WEBHOOK_RETENTION_SWEEP_INTERVAL, default 1h).
	WebhookRetentionSweepInterval time.Duration
	// AuditRetentionDays controls how long audit events are kept before the
	// audit retention sweeper purges them (AUDIT_RETENTION_DAYS, default 0 =
	// disabled). Audit events are compliance evidence, so retention defaults
	// to off and the sweeper only runs when this is explicitly set.
	AuditRetentionDays int
	// AuditRetentionSweepInterval controls how often the audit retention
	// sweeper runs (AUDIT_RETENTION_SWEEP_INTERVAL, default 1h).
	AuditRetentionSweepInterval time.Duration
	// AuditChainAnchorInterval controls how often the audit hash chain
	// anchor sweeper runs (AUDIT_CHAIN_ANCHOR_INTERVAL, default 24h). Anchors
	// are operator-created checkpoints of the tamper-evident chain tail; 0
	// disables automatic anchoring (manual anchors via the operator API only).
	AuditChainAnchorInterval time.Duration
	// AuditArchiveSweepInterval controls how often the WORM audit anchor
	// archiver runs (AUDIT_ARCHIVE_SWEEP_INTERVAL, default 0 = disabled).
	// When enabled, every anchor in audit_chain_anchors is published to the
	// configured S3-compatible object store (see AuditArchiveObjectStorage)
	// and marked published, turning the DB-internal tamper-evident chain into
	// an externally anchored, tamper-proof one: a DB superuser rewriting the
	// chain would diverge from the immutable archived copies.
	AuditArchiveSweepInterval time.Duration
	// AuditArchiveBatchSize bounds how many anchors the archiver publishes
	// per sweep (AUDIT_ARCHIVE_BATCH_SIZE, default 100). Publishing is
	// resumable (see MarkAuditAnchorPublished), so batches advance
	// monotonically across sweeps.
	AuditArchiveBatchSize int
	// AuditArchiveObjectStorage configures the S3-compatible WORM bucket
	// (MinIO or AWS S3 with object lock enabled) that audit anchors are
	// published to. Object lock / retention must be enabled on the bucket so
	// archived anchors cannot be deleted or overwritten.
	AuditArchiveObjectStorage ObjectStorageConfig
	// WorkerBackoffMax caps the exponential restart backoff applied by the
	// worker supervisor to background workers that panic or exit unexpectedly
	// (WORKER_BACKOFF_MAX, default 30s). 0 uses the built-in default.
	WorkerBackoffMax time.Duration
	// ReencryptSweepInterval controls how often the credential re-encryption
	// worker sweeps encrypted rows and re-seals ciphertext written under a
	// previous master key with the active key (REENCRYPT_SWEEP_INTERVAL,
	// default 0 = disabled). It is only meaningful after a PSP master key
	// rotation, while PSP_MASTER_KEY_PREVIOUS is set; the worker converges
	// legacy ciphertext so the old key can eventually be dropped.
	ReencryptSweepInterval time.Duration
	// ReencryptBatchSize bounds how many rows the re-encryption worker
	// processes per table per transaction (REENCRYPT_BATCH_SIZE, default 100).
	ReencryptBatchSize int
	// MigrationScheduleInterval controls how often the cell migration
	// scheduler checks for ready migrations (MIGRATION_SCHEDULE_INTERVAL,
	// default 5m).
	MigrationScheduleInterval time.Duration
	// MetricsEnabled controls whether the Prometheus /metrics endpoint and
	// instrumentation are active (METRICS_ENABLED, default true).
	MetricsEnabled bool
	// MetricsSweepInterval controls how often the backlog reporter refreshes
	// outbox/delivery gauges (METRICS_SWEEP_INTERVAL, default 30s).
	MetricsSweepInterval time.Duration
	// DBMaxConns bounds the PostgreSQL connection pool size
	// (DB_MAX_CONNS, default 10). Explicit sizing prevents surprising
	// defaults (pgxpool otherwise scales with GOMAXPROCS).
	DBMaxConns int
	// DBMinConns keeps at least this many idle connections warm
	// (DB_MIN_CONNS, default 1). Reduces first-request latency spikes.
	DBMinConns int
	// DBMaxConnLifetime caps how long a pooled connection is reused before
	// it is cycled out (DB_MAX_CONN_LIFETIME, default 30m). Guards against
	// stale state after Postgres restarts or failover.
	DBMaxConnLifetime time.Duration
	// DBMaxConnIdleTime closes connections idle beyond this window
	// (DB_MAX_CONN_IDLE_TIME, default 5m).
	DBMaxConnIdleTime time.Duration
	// DBHealthCheckPeriod is how often the pool pings idle connections
	// (DB_HEALTH_CHECK_PERIOD, default 1m).
	DBHealthCheckPeriod time.Duration
	// DBQueryTimeout caps a single store operation (transaction or query)
	// executed through store.WithQueryTimeout — used by background workers
	// so a hung query cannot stall a poll loop forever
	// (DB_QUERY_TIMEOUT, default 10s).
	DBQueryTimeout time.Duration
	// HTTPRequestTimeout caps the whole HTTP handler lifetime. Handlers
	// that outlive it are answered with 504 upstream_timeout
	// (HTTP_REQUEST_TIMEOUT, default 30s).
	HTTPRequestTimeout time.Duration
	// HTTPReadTimeout bounds reading the request, including the body. A slow
	// or stalled client cannot hold a connection open indefinitely
	// (HTTP_READ_TIMEOUT, default 30s). Guards against slowloris and slow
	// body attacks.
	HTTPReadTimeout time.Duration
	// HTTPWriteTimeout bounds writing the response. Must be >=
	// HTTPRequestTimeout or the connection is torn down before handlers can
	// emit their 504 upstream_timeout (HTTP_WRITE_TIMEOUT, default 30s).
	HTTPWriteTimeout time.Duration
	// HTTPIdleTimeout closes keep-alive connections idle beyond this window
	// (HTTP_IDLE_TIMEOUT, default 120s). Reclaims sockets held by clients
	// that connect once and never talk again.
	HTTPIdleTimeout time.Duration
	// HTTPMaxHeaderBytes caps the request header size, protecting against
	// oversized-header DoS (HTTP_MAX_HEADER_BYTES, default 1 MiB).
	HTTPMaxHeaderBytes int
	// ShutdownGracePeriod is the total budget for graceful shutdown:
	// draining in-flight HTTP requests and stopping background workers.
	// Workers that outlive it are abandoned with a warning
	// (SHUTDOWN_GRACE_PERIOD, default 20s).
	ShutdownGracePeriod time.Duration
	// ReadyTimeout bounds the /ready database ping
	// (READY_DB_TIMEOUT, default 2s).
	ReadyTimeout time.Duration
	// PprofEnabled exposes net/http/pprof endpoints on PprofAddr when true.
	// Production default is disabled; enable temporarily to capture live CPU
	// and heap profiles (PPROF_ENABLED, default false).
	PprofEnabled bool
	// PprofAddr is the listen address of the pprof server when enabled
	// (PPROF_ADDR, default ":6060"). It is kept off the main API listener so
	// profiling never widens the attack surface of the public endpoint.
	PprofAddr string
	// SlowRequestThreshold escalates request logs to Warn with slow=true
	// once a request exceeds it. Zero disables escalation
	// (SLOW_REQUEST_THRESHOLD, default 0 = disabled).
	SlowRequestThreshold time.Duration
	// DBQuerySlowThreshold reports SQL statements slower than the threshold
	// to the slow-query observer (db_query_slow_total + Warn log). Zero
	// disables slow-query tracing entirely, so no per-query overhead exists
	// (DB_SLOW_QUERY_THRESHOLD, default 0 = disabled).
	DBQuerySlowThreshold time.Duration
	// ConfigReloadInterval controls how often the config watcher re-reads
	// the environment and applies hot-reloadable settings (rate limits, slow
	// request threshold, CORS origins, log level) without a restart. Zero
	// disables periodic reloading; a SIGHUP signal always triggers an
	// immediate reload (CONFIG_RELOAD_INTERVAL, default 0 = disabled).
	ConfigReloadInterval time.Duration
	// IdempotencyTTL controls how long completed Idempotency-Key responses are
	// retained and replayed before the sweeper purges them (IDEMPOTENCY_TTL,
	// default 24h). Keys are scoped per authenticated identity, method and
	// path, so a longer TTL widens the replay window for clients retrying
	// mutating requests (see internal/httpapi/idempotency.go).
	IdempotencyTTL time.Duration
	// IdempotencySweepInterval controls how often the idempotency record
	// sweeper runs (IDEMPOTENCY_SWEEP_INTERVAL, default 1h).
	IdempotencySweepInterval time.Duration
	// RateLimitBackend selects the rate-limiter backend: "memory" (default;
	// per-process counters, correct for single-instance deployments) or
	// "redis" (counters shared across instances via Redis, required for
	// multi-instance deployments; RATE_LIMIT_BACKEND).
	RateLimitBackend string
	// Redis connection settings, used when RATE_LIMIT_BACKEND=redis and by
	// future Redis-backed components.
	// RedisAddr is the host:port of the Redis server (REDIS_ADDR).
	RedisAddr string
	// RedisPassword authenticates against the Redis server (REDIS_PASSWORD).
	RedisPassword string
	// RedisDB selects the logical database (REDIS_DB, default 0).
	RedisDB int
}

// RateLimitConfig holds per-level rate limit settings. All limits are
// requests per window (default 1 minute).
type RateLimitConfig struct {
	Provider    int // per-provider (default 1000)
	Environment int // per-environment (default 500)
	Credential  int // per-credential/API-key (default 200)
	Endpoint    int // per-credential+endpoint (default 60)
	// IP is a global per-source-IP safety net applied before authentication,
	// so a single client cannot bypass the authenticated limits by rotating
	// credentials. It also protects unauthenticated endpoints (health,
	// metrics) from naive DoS. 0 disables the per-IP layer entirely
	// (RL_IP_LIMIT, default 6000).
	IP     int           // per-source-IP (default 6000)
	Window time.Duration // fixed window duration (default 1m)
}

// CircuitBreakerConfig configures the circuit breakers guarding outbound
// dependencies (webhook endpoints and the billing engine). A breaker trips
// open after FailureThreshold consecutive failures, fast-fails calls while
// open, and re-probes after OpenTimeout; a successful probe closes it.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures that opens the
	// breaker (CB_FAILURE_THRESHOLD, default 5).
	FailureThreshold int
	// OpenTimeout is how long the breaker stays open before probing again
	// (CB_OPEN_TIMEOUT, default 30s).
	OpenTimeout time.Duration
	// HalfOpenMax bounds concurrent probe calls while half-open
	// (CB_HALF_OPEN_MAX, default 1).
	HalfOpenMax int
}

// ToOptions maps the configuration to breaker options. The breaker name is
// assigned by the caller per downstream dependency.
func (c CircuitBreakerConfig) ToOptions() circuitbreaker.Options {
	return circuitbreaker.Options{
		FailureThreshold: c.FailureThreshold,
		OpenTimeout:      c.OpenTimeout,
		HalfOpenMax:      c.HalfOpenMax,
	}
}

// ObjectStorageConfig configures an S3-compatible object store used by the
// WORM audit archiver. All S3-compatible services (MinIO, AWS S3, GCS
// interop) accept the same credentials model; the archiver only ever PUTs
// objects, so no read or list permissions are needed.
type ObjectStorageConfig struct {
	// Endpoint is the S3 API base URL, e.g. "https://s3.example.com" for
	// MinIO or "https://s3.<region>.amazonaws.com" for AWS
	// (AUDIT_ARCHIVE_S3_ENDPOINT).
	Endpoint string
	// Bucket is the WORM bucket to publish anchors into
	// (AUDIT_ARCHIVE_S3_BUCKET). Object lock must be enabled.
	Bucket string
	// AccessKey is the S3 access key ID (AUDIT_ARCHIVE_S3_ACCESS_KEY).
	AccessKey string
	// SecretKey is the S3 secret access key (AUDIT_ARCHIVE_S3_SECRET_KEY).
	SecretKey string
	// Region is the S3 region (AUDIT_ARCHIVE_S3_REGION, default "").
	Region string
	// UseSSL forces HTTPS for the S3 connection
	// (AUDIT_ARCHIVE_S3_USE_SSL, default true).
	UseSSL bool
}

// TelemetryConfig configures OpenTelemetry distributed tracing. All values
// parse from standard OTEL_* env vars; the zero-ish defaults keep tracing
// off unless explicitly enabled.
type TelemetryConfig struct {
	// Enabled turns distributed tracing on (OTEL_ENABLED, default false).
	Enabled bool
	// Exporter selects the span exporter (OTEL_EXPORTER, default "otlp";
	// "stdout" for local debugging, "noop" to force-disable).
	Exporter string
	// OTLPEndpoint is the collector base URL (OTEL_EXPORTER_OTLP_ENDPOINT,
	// default empty = exporter default http://localhost:4318).
	OTLPEndpoint string
	// ServiceName is the service.name resource attribute (OTEL_SERVICE_NAME,
	// default "vlogbin-api").
	ServiceName string
	// Environment is deployment.environment (OTEL_ENVIRONMENT, default
	// "development").
	Environment string
	// SampleRatio in [0,1] (OTEL_SAMPLE_RATIO, default 1): 1 samples every
	// trace, 0 none, fractions use head sampling by trace ID.
	SampleRatio float64
	// BatchTimeout flushes a batch after this idle window (OTEL_BATCH_TIMEOUT).
	BatchTimeout time.Duration
	// ExportTimeout caps a single export attempt (OTEL_EXPORT_TIMEOUT).
	ExportTimeout time.Duration
	// MaxQueueSize bounds the in-memory span queue (OTEL_MAX_QUEUE_SIZE).
	MaxQueueSize int
	// MaxExportBatchSize bounds spans per export batch
	// (OTEL_MAX_EXPORT_BATCH_SIZE).
	MaxExportBatchSize int
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:                   os.Getenv("DATABASE_URL"),
		MigrationDatabaseURL:          os.Getenv("MIGRATION_DATABASE_URL"),
		HTTPAddr:                      envOr("HTTP_ADDR", ":8080"),
		OperatorToken:                 os.Getenv("OPERATOR_TOKEN"),
		PlatformBaseDomain:            envOr("PLATFORM_BASE_DOMAIN", "platform.local"),
		OutboxPollInterval:            time.Second,
		WebhookPollInterval:           5 * time.Second,
		BillingAdapter:                envOr("BILLING_ADAPTER", "noop"),
		LagoAPIURL:                    os.Getenv("LAGO_API_URL"),
		LagoAPIKey:                    os.Getenv("LAGO_API_KEY"),
		UsageLateWindow:               168 * time.Hour,
		ReconciliationInterval:        time.Hour,
		SupportSweepInterval:          30 * time.Second,
		QuotaSweepInterval:            15 * time.Second,
		WebhookRetentionDays:          30,
		WebhookRetentionSweepInterval: time.Hour,
		AuditRetentionDays:            0, // off by default: audit events are compliance evidence
		AuditRetentionSweepInterval:   time.Hour,
		AuditChainAnchorInterval:      24 * time.Hour,
		AuditArchiveSweepInterval:     0, // off by default: requires object storage config
		AuditArchiveBatchSize:         100,
		AuditArchiveObjectStorage:     ObjectStorageConfig{UseSSL: true},
		WorkerBackoffMax:              30 * time.Second,
		ReencryptBatchSize:            100,
		MigrationScheduleInterval:     5 * time.Minute,
		MetricsEnabled:                true,
		MetricsSweepInterval:          30 * time.Second,
		DBMaxConns:                    10,
		IdempotencyTTL:                24 * time.Hour,
		IdempotencySweepInterval:      time.Hour,
		RateLimitBackend:              "memory",
		DBMinConns:                    1,
		DBMaxConnLifetime:             30 * time.Minute,
		DBMaxConnIdleTime:             5 * time.Minute,
		DBHealthCheckPeriod:           time.Minute,
		DBQueryTimeout:                10 * time.Second,
		HTTPRequestTimeout:            30 * time.Second,
		HTTPReadTimeout:               30 * time.Second,
		HTTPWriteTimeout:              30 * time.Second,
		HTTPIdleTimeout:               120 * time.Second,
		HTTPMaxHeaderBytes:            1 << 20,
		ShutdownGracePeriod:           20 * time.Second,
		ReadyTimeout:                  2 * time.Second,
		PprofEnabled:                  false,
		PprofAddr:                     ":6060",
		SlowRequestThreshold:          0,
		CORSAllowedOrigins:            []string{"*"},
		LogLevel:                      "info",
		RateLimits: RateLimitConfig{
			Provider:    1000,
			Environment: 500,
			Credential:  200,
			Endpoint:    60,
			IP:          6000,
			Window:      time.Minute,
		},
		CircuitBreakers: CircuitBreakerConfig{
			FailureThreshold: 5,
			OpenTimeout:      30 * time.Second,
			HalfOpenMax:      1,
		},
		Telemetry: TelemetryConfig{
			Enabled:            false,
			Exporter:           "otlp",
			ServiceName:        "vlogbin-api",
			Environment:        "development",
			SampleRatio:        1,
			BatchTimeout:       5 * time.Second,
			ExportTimeout:      30 * time.Second,
			MaxQueueSize:       2048,
			MaxExportBatchSize: 512,
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
	if v := os.Getenv("WEBHOOK_RETENTION_DAYS"); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 {
			return Config{}, fmt.Errorf("invalid WEBHOOK_RETENTION_DAYS %q: must be a non-negative integer", v)
		}
		cfg.WebhookRetentionDays = days
	}
	if v := os.Getenv("WEBHOOK_RETENTION_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.WebhookRetentionSweepInterval = d
		}
	}
	if v := os.Getenv("AUDIT_RETENTION_DAYS"); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 0 {
			return Config{}, fmt.Errorf("invalid AUDIT_RETENTION_DAYS %q: must be a non-negative integer", v)
		}
		cfg.AuditRetentionDays = days
	}
	if v := os.Getenv("AUDIT_RETENTION_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.AuditRetentionSweepInterval = d
		}
	}
	if v := os.Getenv("AUDIT_CHAIN_ANCHOR_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return Config{}, fmt.Errorf("invalid AUDIT_CHAIN_ANCHOR_INTERVAL %q: must be a non-negative duration", v)
		}
		cfg.AuditChainAnchorInterval = d
	}
	if v := os.Getenv("AUDIT_ARCHIVE_SWEEP_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return Config{}, fmt.Errorf("invalid AUDIT_ARCHIVE_SWEEP_INTERVAL %q: must be a non-negative duration", v)
		}
		cfg.AuditArchiveSweepInterval = d
	}
	if v := os.Getenv("AUDIT_ARCHIVE_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid AUDIT_ARCHIVE_BATCH_SIZE %q: must be a positive integer", v)
		}
		cfg.AuditArchiveBatchSize = n
	}
	cfg.AuditArchiveObjectStorage.Endpoint = os.Getenv("AUDIT_ARCHIVE_S3_ENDPOINT")
	cfg.AuditArchiveObjectStorage.Bucket = os.Getenv("AUDIT_ARCHIVE_S3_BUCKET")
	cfg.AuditArchiveObjectStorage.AccessKey = os.Getenv("AUDIT_ARCHIVE_S3_ACCESS_KEY")
	cfg.AuditArchiveObjectStorage.SecretKey = os.Getenv("AUDIT_ARCHIVE_S3_SECRET_KEY")
	cfg.AuditArchiveObjectStorage.Region = os.Getenv("AUDIT_ARCHIVE_S3_REGION")
	if v := os.Getenv("AUDIT_ARCHIVE_S3_USE_SSL"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid AUDIT_ARCHIVE_S3_USE_SSL %q: must be a boolean", v)
		}
		cfg.AuditArchiveObjectStorage.UseSSL = b
	}
	if v := os.Getenv("WORKER_BACKOFF_MAX"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return Config{}, fmt.Errorf("invalid WORKER_BACKOFF_MAX %q: must be a non-negative duration", v)
		}
		cfg.WorkerBackoffMax = d
	}
	if v := os.Getenv("REENCRYPT_SWEEP_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return Config{}, fmt.Errorf("invalid REENCRYPT_SWEEP_INTERVAL %q: must be a non-negative duration", v)
		}
		cfg.ReencryptSweepInterval = d
	}
	if v := os.Getenv("REENCRYPT_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid REENCRYPT_BATCH_SIZE %q: must be a positive integer", v)
		}
		cfg.ReencryptBatchSize = n
	}
	for _, spec := range []struct {
		env  string
		dest *int
	}{
		{"RL_PROVIDER_LIMIT", &cfg.RateLimits.Provider},
		{"RL_ENVIRONMENT_LIMIT", &cfg.RateLimits.Environment},
		{"RL_CREDENTIAL_LIMIT", &cfg.RateLimits.Credential},
		{"RL_ENDPOINT_LIMIT", &cfg.RateLimits.Endpoint},
		{"RL_IP_LIMIT", &cfg.RateLimits.IP},
	} {
		if v := os.Getenv(spec.env); v != "" {
			limit, err := strconv.Atoi(v)
			if err != nil || limit < 0 {
				return Config{}, fmt.Errorf("invalid %s %q: must be a non-negative integer", spec.env, v)
			}
			*spec.dest = limit
		}
	}
	if v := os.Getenv("RL_WINDOW"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("invalid RL_WINDOW %q: must be a positive duration", v)
		}
		cfg.RateLimits.Window = d
	}
	if v := os.Getenv("CB_FAILURE_THRESHOLD"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid CB_FAILURE_THRESHOLD %q: must be a positive integer", v)
		}
		cfg.CircuitBreakers.FailureThreshold = n
	}
	if err := durationEnv("CB_OPEN_TIMEOUT", &cfg.CircuitBreakers.OpenTimeout); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("CB_HALF_OPEN_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid CB_HALF_OPEN_MAX %q: must be a positive integer", v)
		}
		cfg.CircuitBreakers.HalfOpenMax = n
	}
	if v := os.Getenv("OTEL_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid OTEL_ENABLED %q: must be a boolean", v)
		}
		cfg.Telemetry.Enabled = b
	}
	if v := os.Getenv("OTEL_EXPORTER"); v != "" {
		switch v {
		case "otlp", "stdout", "noop":
			cfg.Telemetry.Exporter = v
		default:
			return Config{}, fmt.Errorf("invalid OTEL_EXPORTER %q: want otlp|stdout|noop", v)
		}
	}
	cfg.Telemetry.OTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	cfg.Telemetry.ServiceName = envOr("OTEL_SERVICE_NAME", cfg.Telemetry.ServiceName)
	cfg.Telemetry.Environment = envOr("OTEL_ENVIRONMENT", cfg.Telemetry.Environment)
	if v := os.Getenv("OTEL_SAMPLE_RATIO"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || f > 1 {
			return Config{}, fmt.Errorf("invalid OTEL_SAMPLE_RATIO %q: must be a float in [0,1]", v)
		}
		cfg.Telemetry.SampleRatio = f
	}
	if err := durationEnv("OTEL_BATCH_TIMEOUT", &cfg.Telemetry.BatchTimeout); err != nil {
		return Config{}, err
	}
	if err := durationEnv("OTEL_EXPORT_TIMEOUT", &cfg.Telemetry.ExportTimeout); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("OTEL_MAX_QUEUE_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid OTEL_MAX_QUEUE_SIZE %q: must be a positive integer", v)
		}
		cfg.Telemetry.MaxQueueSize = n
	}
	if v := os.Getenv("OTEL_MAX_EXPORT_BATCH_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid OTEL_MAX_EXPORT_BATCH_SIZE %q: must be a positive integer", v)
		}
		cfg.Telemetry.MaxExportBatchSize = n
	}
	if v := os.Getenv("MIGRATION_SCHEDULE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.MigrationScheduleInterval = d
		}
	}
	if v := os.Getenv("METRICS_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid METRICS_ENABLED %q: %w", v, err)
		}
		cfg.MetricsEnabled = enabled
	}
	if v := os.Getenv("METRICS_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.MetricsSweepInterval = d
		}
	}
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("invalid DB_MAX_CONNS %q: must be a non-negative integer", v)
		}
		cfg.DBMaxConns = n
	}
	if v := os.Getenv("DB_MIN_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("invalid DB_MIN_CONNS %q: must be a non-negative integer", v)
		}
		cfg.DBMinConns = n
	}
	if cfg.DBMinConns > cfg.DBMaxConns {
		return Config{}, fmt.Errorf("DB_MIN_CONNS (%d) exceeds DB_MAX_CONNS (%d)", cfg.DBMinConns, cfg.DBMaxConns)
	}
	if err := durationEnv("DB_MAX_CONN_LIFETIME", &cfg.DBMaxConnLifetime); err != nil {
		return Config{}, err
	}
	if err := durationEnv("DB_MAX_CONN_IDLE_TIME", &cfg.DBMaxConnIdleTime); err != nil {
		return Config{}, err
	}
	if err := durationEnv("DB_HEALTH_CHECK_PERIOD", &cfg.DBHealthCheckPeriod); err != nil {
		return Config{}, err
	}
	if err := durationEnv("DB_QUERY_TIMEOUT", &cfg.DBQueryTimeout); err != nil {
		return Config{}, err
	}
	if err := durationEnv("HTTP_REQUEST_TIMEOUT", &cfg.HTTPRequestTimeout); err != nil {
		return Config{}, err
	}
	if err := durationEnv("HTTP_READ_TIMEOUT", &cfg.HTTPReadTimeout); err != nil {
		return Config{}, err
	}
	if err := durationEnv("HTTP_WRITE_TIMEOUT", &cfg.HTTPWriteTimeout); err != nil {
		return Config{}, err
	}
	if err := durationEnv("HTTP_IDLE_TIMEOUT", &cfg.HTTPIdleTimeout); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("HTTP_MAX_HEADER_BYTES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid HTTP_MAX_HEADER_BYTES %q: must be a positive integer", v)
		}
		cfg.HTTPMaxHeaderBytes = n
	}
	if err := durationEnv("SHUTDOWN_GRACE_PERIOD", &cfg.ShutdownGracePeriod); err != nil {
		return Config{}, err
	}
	if err := durationEnv("READY_DB_TIMEOUT", &cfg.ReadyTimeout); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("PPROF_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PPROF_ENABLED %q: must be a boolean", v)
		}
		cfg.PprofEnabled = b
	}
	if v := os.Getenv("PPROF_ADDR"); v != "" {
		cfg.PprofAddr = v
	}
	if err := durationEnv("SLOW_REQUEST_THRESHOLD", &cfg.SlowRequestThreshold); err != nil {
		return Config{}, err
	}
	if err := durationEnv("DB_SLOW_QUERY_THRESHOLD", &cfg.DBQuerySlowThreshold); err != nil {
		return Config{}, err
	}
	if err := durationEnv("CONFIG_RELOAD_INTERVAL", &cfg.ConfigReloadInterval); err != nil {
		return Config{}, err
	}
	if err := durationEnv("IDEMPOTENCY_TTL", &cfg.IdempotencyTTL); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencyTTL < 0 {
		return Config{}, fmt.Errorf("invalid IDEMPOTENCY_TTL %q: must be non-negative", cfg.IdempotencyTTL)
	}
	if err := durationEnv("IDEMPOTENCY_SWEEP_INTERVAL", &cfg.IdempotencySweepInterval); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencySweepInterval < 0 {
		return Config{}, fmt.Errorf("invalid IDEMPOTENCY_SWEEP_INTERVAL %q: must be non-negative", cfg.IdempotencySweepInterval)
	}
	if v := os.Getenv("RATE_LIMIT_BACKEND"); v != "" {
		if v != "memory" && v != "redis" {
			return Config{}, fmt.Errorf("invalid RATE_LIMIT_BACKEND %q: must be \"memory\" or \"redis\"", v)
		}
		cfg.RateLimitBackend = v
	}
	cfg.RedisAddr = os.Getenv("REDIS_ADDR")
	cfg.RedisPassword = os.Getenv("REDIS_PASSWORD")
	if v := os.Getenv("REDIS_DB"); v != "" {
		db, err := strconv.Atoi(v)
		if err != nil || db < 0 {
			return Config{}, fmt.Errorf("invalid REDIS_DB %q: must be a non-negative integer", v)
		}
		cfg.RedisDB = db
	}
	if cfg.RateLimitBackend == "redis" && cfg.RedisAddr == "" {
		return Config{}, fmt.Errorf("RATE_LIMIT_BACKEND=redis requires REDIS_ADDR to be set")
	}
	if v := os.Getenv("CORS_ALLOWED_ORIGINS"); v != "" {
		cfg.CORSAllowedOrigins = splitComma(v)
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	cfg.PSPMasterKey = os.Getenv("PSP_MASTER_KEY")
	if v := os.Getenv("PSP_MASTER_KEY_PREVIOUS"); v != "" {
		cfg.PSPMasterKeyPrevious = splitComma(v)
	}
	cfg.PortalTokenSecret = os.Getenv("PORTAL_TOKEN_SECRET")
	cfg.PortalTokenTTL = 24 * time.Hour
	if err := durationEnv("PORTAL_TOKEN_TTL", &cfg.PortalTokenTTL); err != nil {
		return Config{}, err
	}
	cfg.ZITADELURL = os.Getenv("ZITADEL_URL")
	cfg.ZITADELPAT = os.Getenv("ZITADEL_PAT")
	cfg.AuthVaultServiceToken = os.Getenv("AUTH_VAULT_SERVICE_TOKEN")
	cfg.AuthVaultPublicKey = os.Getenv("AUTH_VAULT_PUBLIC_KEY")
	cfg.AuthVaultAudience = os.Getenv("AUTH_VAULT_AUDIENCE")
	if cfg.AuthVaultAudience == "" {
		cfg.AuthVaultAudience = "vlogbin-auth-vault"
	}
	cfg.AuthVaultMasterKey = os.Getenv("AUTH_VAULT_MASTER_KEY")
	cfg.AuthVaultMasterKeyPrevious = splitComma(os.Getenv("AUTH_VAULT_MASTER_KEY_PREVIOUS"))
	cfg.AuthVaultSweepInterval = time.Hour
	if err := durationEnv("AUTH_VAULT_SWEEP_INTERVAL", &cfg.AuthVaultSweepInterval); err != nil {
		return Config{}, err
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
	if cfg.AuditArchiveSweepInterval > 0 {
		o := cfg.AuditArchiveObjectStorage
		if o.Endpoint == "" || o.Bucket == "" || o.AccessKey == "" || o.SecretKey == "" {
			return Config{}, fmt.Errorf("AUDIT_ARCHIVE_SWEEP_INTERVAL requires AUDIT_ARCHIVE_S3_ENDPOINT, AUDIT_ARCHIVE_S3_BUCKET, AUDIT_ARCHIVE_S3_ACCESS_KEY and AUDIT_ARCHIVE_S3_SECRET_KEY to be set")
		}
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

// durationEnv reads an env var as a duration into target. An unset variable
// leaves the target untouched (caller-provided default). An invalid value is
// a hard error: misconfiguration should fail fast at startup, not degrade at
// runtime.
func durationEnv(key string, target *time.Duration) error {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	*target = d
	return nil
}
