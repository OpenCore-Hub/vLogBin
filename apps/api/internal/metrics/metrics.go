// Package metrics owns the platform's Prometheus instrumentation.
//
// A private registry (rather than the global default registry) is used on
// purpose: integration tests create servers against the same process, and a
// shared default registry would cause duplicate-registration panics. Each
// httpapi.Server gets its own Metrics instance so /metrics output is fully
// deterministic per instance.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics bundles the platform metric families and their private registry.
type Metrics struct {
	registry *prometheus.Registry

	// HTTPRequestsTotal counts API requests by method, route pattern and
	// status class. Route is the chi pattern (not the raw path) so UUID
	// segments do not explode cardinality.
	HTTPRequestsTotal *prometheus.CounterVec
	// HTTPRequestSeconds is a histogram of API request latency.
	HTTPRequestSeconds *prometheus.HistogramVec
	// ReadyChecksTotal counts readiness probe dependency checks by dependency
	// name and result (up / down), so readiness flapping can be alerted on
	// and attributed to the exact dependency that is failing.
	ReadyChecksTotal *prometheus.CounterVec

	// WebhookDeliveriesTotal counts webhook delivery outcomes
	// (delivered / retry / dead_letter).
	WebhookDeliveriesTotal *prometheus.CounterVec
	// WebhookDeliverySeconds is a histogram of the webhook HTTP POST itself.
	WebhookDeliverySeconds prometheus.Histogram

	// OutboxEventsTotal is a gauge of the current outbox_events backlog
	// per status, refreshed periodically by the backlog reporter.
	OutboxEventsTotal *prometheus.GaugeVec
	// OutboxDeadLetterTotal counts outbox events permanently failed
	// (dead-lettered) by the relay: max attempts reached or unparseable
	// payload. A non-zero rate means events are dying without delivery.
	OutboxDeadLetterTotal prometheus.Counter
	// WebhookDeliveriesGauge is a gauge of the current webhook_deliveries
	// backlog per status, refreshed by the same reporter.
	WebhookDeliveriesGauge *prometheus.GaugeVec

	// SweepDeletedTotal counts rows purged by each background sweeper
	// (quota, support, webhook_retention, ...).
	SweepDeletedTotal *prometheus.CounterVec

	// StoreErrorsTotal counts classified store errors by operation and
	// error class (timeout, not_found, conflict, connection, other). Wired by
	// main through store.SetErrorObserver, so DB failure modes are alertable.
	StoreErrorsTotal *prometheus.CounterVec

	// Connection pool health. The four gauges are refreshed by the
	// pool-reporter worker; the counters accumulate deltas from the pgxpool
	// cumulative stats so capacity exhaustion (acquired == max, rising
	// empty-acquire waits) is alertable from the first scrape.
	DBPoolMaxConns          prometheus.Gauge
	DBPoolAcquiredConns     prometheus.Gauge
	DBPoolIdleConns         prometheus.Gauge
	DBPoolConstructingConns prometheus.Gauge
	DBPoolAcquireTotal      prometheus.Counter
	DBPoolAcquireSeconds    prometheus.Counter
	DBPoolEmptyAcquireTotal prometheus.Counter

	// DBQuerySlowTotal counts statements that exceeded the configured
	// slow-query threshold, reported by the pgx QueryTracer. Wired by main
	// through store.SetSlowQueryObserver, so SQL-level latency regressions are
	// alertable even when no HTTP request is slow (e.g. background workers).
	DBQuerySlowTotal prometheus.Counter

	// HTTPRateLimitedTotal counts requests rejected with 429 by the rate
	// limiter, labelled by the limiting level so DoS pressure at any layer
	// (per-IP before auth, or provider/env/credential/endpoint after auth)
	// is visible without grepping access logs.
	HTTPRateLimitedTotal *prometheus.CounterVec
	// HTTPDeprecatedUsageTotal counts requests to deprecated API endpoints,
	// labelled by raw path. A non-zero rate means clients still depend on
	// endpoints scheduled for sunset.
	HTTPDeprecatedUsageTotal *prometheus.CounterVec
	// RateLimiterBackendErrorsTotal counts rate-limiter backend (Redis) call
	// failures. The limiter fails open on backend errors, so this counter is
	// the only signal that distributed rate limiting silently degraded to
	// unthrottled.
	RateLimiterBackendErrorsTotal prometheus.Counter

	// CircuitBreakerState is a gauge of the current circuit-breaker state per
	// protected downstream (0=closed, 1=half_open, 2=open), updated on every
	// transition so a dependency that is tripped open is immediately visible
	// to alerting.
	CircuitBreakerState *prometheus.GaugeVec
	// CircuitBreakerRequestsTotal counts circuit-breaker decisions and
	// outcomes per downstream: allowed, denied (fast-failed while open),
	// success and failure.
	CircuitBreakerRequestsTotal *prometheus.CounterVec

	// WorkerRestartsTotal counts supervised background-worker restarts by
	// worker name and exit reason (panic, exit). Wired by main through the
	// worker supervisor's OnRestart hook; a worker that dies silently and is
	// never restarted is exactly what this family surfaces.
	WorkerRestartsTotal *prometheus.CounterVec
	// WorkerPanicsTotal counts supervised background-worker panics by worker
	// name. Wired by main through the supervisor's OnPanic hook; distinct
	// from WorkerRestartsTotal so panic-led restarts are separately alertable.
	WorkerPanicsTotal *prometheus.CounterVec

	// CredentialDecryptFallbackTotal counts ciphertexts that were
	// successfully decrypted with a previous master key after a PSP master
	// key rotation. Wired by main through the encryptor's fallback observer;
	// a sustained non-zero rate means legacy credentials are still being read
	// with rotated-out keys and should be re-encrypted to converge.
	CredentialDecryptFallbackTotal prometheus.Counter
	// CredentialsReencryptedTotal counts rows whose ciphertext was re-sealed
	// with the active master key by the re-encryption worker, by table. A
	// sustained non-zero rate means the rotation is converging; when it drops
	// to zero and stays there, old keys can be dropped from
	// PSP_MASTER_KEY_PREVIOUS.
	CredentialsReencryptedTotal *prometheus.CounterVec
	// CredentialsReencryptErrorsTotal counts rows the re-encryption worker
	// could not converge because no configured key could open the ciphertext
	// (corrupt data, or keys lost outside rotation). Such rows need operator
	// attention — they are the only ones that block a full rotation
	// convergence.
	CredentialsReencryptErrorsTotal *prometheus.CounterVec
	// AuditAnchorsPublishedTotal counts audit hash chain anchors the archiver
	// published to WORM object storage, by result ("published" or
	// "already_published"). The published count is the completeness signal
	// for the external anchoring pipeline: when it stops growing while
	// anchors keep being created, the archiver is falling behind or the
	// object store is unavailable.
	AuditAnchorsPublishedTotal *prometheus.CounterVec
	// AuditArchiveErrorsTotal counts audit archiver failures by operation
	// ("list" | "upload" | "mark"). upload errors surface object-store
	// problems (credentials, bucket lock policy, network); mark errors
	// surface DB write problems. A non-zero upload rate with a stable
	// published rate means anchors are being created faster than the WORM
	// copy completes — a backlog to alert on.
	AuditArchiveErrorsTotal *prometheus.CounterVec
	// ReconciliationDrift is a gauge of the current drift count per
	// reconciliation check, refreshed by the reconciliation worker on every
	// run (default hourly). Any series above zero means a platform
	// consistency or financial invariant is currently violated; alerting on
	// per-check >0 is the primary consumer of this family.
	ReconciliationDrift *prometheus.GaugeVec
	// AuthVaultOperationsTotal counts server-side OIDC token vault operations
	// by operation and result so vault abuse/outage is alertable.
	AuthVaultOperationsTotal *prometheus.CounterVec
}

// New builds the metric registry and registers the go runtime + process
// collectors so /metrics exposes go_goroutines and friends out of the box.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	m := &Metrics{
		registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP API requests by method, route pattern and status class.",
			},
			[]string{"method", "route", "status"},
		),
		HTTPRequestSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP API request latency by method and route pattern.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "route"},
		),
		ReadyChecksTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "readiness_checks_total",
				Help: "Readiness probe dependency checks by dependency and result.",
			},
			[]string{"dependency", "status"},
		),
		WebhookDeliveriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "webhook_deliveries_total",
				Help: "Total webhook deliveries by outcome (delivered, retry, dead_letter).",
			},
			[]string{"status"},
		),
		WebhookDeliverySeconds: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "webhook_delivery_duration_seconds",
				Help:    "Latency of the webhook HTTP POST call.",
				Buckets: prometheus.DefBuckets,
			},
		),
		OutboxEventsTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "outbox_events_total",
				Help: "Current outbox_events backlog per status.",
			},
			[]string{"status"},
		),
		WebhookDeliveriesGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "webhook_deliveries",
				Help: "Current webhook_deliveries backlog per status.",
			},
			[]string{"status"},
		),
		SweepDeletedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "sweep_deleted_total",
				Help: "Total rows purged per background sweeper name.",
			},
			[]string{"name"},
		),
		StoreErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "store_errors_total",
				Help: "Store errors by operation and error class (timeout, not_found, conflict, connection, other).",
			},
			[]string{"op", "class"},
		),
		DBPoolMaxConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "db_pool_max_conns",
			Help: "Maximum number of connections the pgxpool is allowed to hold.",
		}),
		DBPoolAcquiredConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "db_pool_acquired_conns",
			Help: "Number of connections currently checked out from the pgxpool.",
		}),
		DBPoolIdleConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "db_pool_idle_conns",
			Help: "Number of idle connections currently held by the pgxpool.",
		}),
		DBPoolConstructingConns: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "db_pool_constructing_conns",
			Help: "Number of connections currently being established by the pgxpool.",
		}),
		DBPoolAcquireTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "db_pool_acquire_total",
			Help: "Cumulative number of successful acquire attempts from the pgxpool.",
		}),
		DBPoolAcquireSeconds: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "db_pool_acquire_seconds_total",
			Help: "Cumulative wall-clock time spent waiting to acquire connections from the pgxpool.",
		}),
		DBPoolEmptyAcquireTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "db_pool_empty_acquire_total",
			Help: "Cumulative number of acquire attempts that had to wait for a connection because the pool was empty.",
		}),
		DBQuerySlowTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "db_query_slow_total",
			Help: "Cumulative number of SQL statements that exceeded the configured slow-query threshold.",
		}),
		HTTPRateLimitedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_rate_limited_total",
				Help: "Requests rejected with 429 by the rate limiter, by limiting level (ip, provider, environment, credential, endpoint).",
			},
			[]string{"level"},
		),
		HTTPDeprecatedUsageTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_api_deprecated_usage_total",
				Help: "Requests to deprecated API endpoints by path, so migration progress and sunset risk are observable.",
			},
			[]string{"path"},
		),
		RateLimiterBackendErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "rate_limiter_backend_errors_total",
			Help: "Cumulative number of rate-limiter backend (Redis) call failures; the limiter fails open on backend errors.",
		}),
		CircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "circuit_breaker_state",
				Help: "Current circuit breaker state per downstream dependency (0=closed, 1=half_open, 2=open).",
			},
			[]string{"name"},
		),
		CircuitBreakerRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "circuit_breaker_requests_total",
				Help: "Circuit breaker decisions and outcomes per downstream dependency (result: allowed, denied, success, failure).",
			},
			[]string{"name", "result"},
		),
		WorkerRestartsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "worker_restarts_total",
				Help: "Supervised background worker restarts by worker name and exit reason (panic, exit).",
			},
			[]string{"name", "reason"},
		),
		WorkerPanicsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "worker_panics_total",
				Help: "Supervised background worker panics by worker name.",
			},
			[]string{"name"},
		),
		CredentialDecryptFallbackTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "credential_decrypt_fallback_total",
			Help: "Ciphertexts decrypted with a previous PSP master key after rotation; a non-zero rate means legacy ciphertext is still being read.",
		}),
		CredentialsReencryptedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "credentials_reencrypted_total",
				Help: "Rows re-sealed with the active PSP master key by the re-encryption worker, by table.",
			},
			[]string{"table"},
		),
		CredentialsReencryptErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "credentials_reencrypt_errors_total",
				Help: "Rows the re-encryption worker could not converge (no configured key opens the ciphertext), by table.",
			},
			[]string{"table"},
		),
		AuditAnchorsPublishedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audit_anchors_published_total",
				Help: "Audit hash chain anchors published to WORM object storage, by result (published, already_published).",
			},
			[]string{"result"},
		),
		AuditArchiveErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "audit_archive_errors_total",
				Help: "Audit archiver failures by operation (list, upload, mark).",
			},
			[]string{"op"},
		),
		OutboxDeadLetterTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "outbox_dead_letter_total",
			Help: "Cumulative number of outbox events permanently failed and moved to the dead-letter queue (status='dead_letter').",
		}),
		ReconciliationDrift: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "reconciliation_drift",
				Help: "Current drift count per reconciliation check, refreshed every reconciliation run.",
			},
			[]string{"check"},
		),
		AuthVaultOperationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "auth_vault_operations_total",
				Help: "Server-side OIDC token vault operations by operation and result.",
			},
			[]string{"operation", "result"},
		),
	}
	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestSeconds,
		m.ReadyChecksTotal,
		m.WebhookDeliveriesTotal,
		m.WebhookDeliverySeconds,
		m.OutboxEventsTotal,
		m.OutboxDeadLetterTotal,
		m.WebhookDeliveriesGauge,
		m.SweepDeletedTotal,
		m.StoreErrorsTotal,
		m.DBPoolMaxConns,
		m.DBPoolAcquiredConns,
		m.DBPoolIdleConns,
		m.DBPoolConstructingConns,
		m.DBPoolAcquireTotal,
		m.DBPoolAcquireSeconds,
		m.DBPoolEmptyAcquireTotal,
		m.DBQuerySlowTotal,
		m.HTTPRateLimitedTotal,
		m.HTTPDeprecatedUsageTotal,
		m.RateLimiterBackendErrorsTotal,
		m.CircuitBreakerState,
		m.CircuitBreakerRequestsTotal,
		m.WorkerRestartsTotal,
		m.WorkerPanicsTotal,
		m.CredentialDecryptFallbackTotal,
		m.CredentialsReencryptedTotal,
		m.CredentialsReencryptErrorsTotal,
		m.AuditAnchorsPublishedTotal,
		m.AuditArchiveErrorsTotal,
		m.ReconciliationDrift,
		m.AuthVaultOperationsTotal,
	)

	// Pre-initialize the label sets the reporters and workers write to, so
	// the metric families always appear in /metrics output even with zero
	// traffic. This keeps Prometheus recording rules and alerts (e.g. outbox
	// backlog > 0) matchable from the very first scrape.
	for _, status := range []string{"pending", "published", "failed", "dead_letter"} {
		m.OutboxEventsTotal.WithLabelValues(status).Set(0)
	}
	for _, status := range []string{"pending", "delivered", "failed", "dead_letter"} {
		m.WebhookDeliveriesGauge.WithLabelValues(status).Set(0)
	}
	for _, status := range []string{"delivered", "retry", "dead_letter"} {
		m.WebhookDeliveriesTotal.WithLabelValues(status)
	}
	for _, name := range []string{"support_session", "quota_reservation", "webhook_retention"} {
		m.SweepDeletedTotal.WithLabelValues(name)
	}
	// Pre-initialize the pool counters so the families appear in /metrics
	// even before the pool-reporter's first sweep.
	m.DBPoolAcquireTotal.Add(0)
	m.DBPoolAcquireSeconds.Add(0)
	m.DBPoolEmptyAcquireTotal.Add(0)
	// Pre-initialize the rate-limit rejection levels so the family appears
	// with all levels from the first scrape, even before any 429 occurs.
	for _, level := range []string{"ip", "provider", "environment", "credential", "endpoint"} {
		m.HTTPRateLimitedTotal.WithLabelValues(level)
	}
	m.RateLimiterBackendErrorsTotal.Add(0)
	// Pre-initialize the credential decrypt fallback counter so the family
	// appears in /metrics even before the first post-rotation read.
	m.CredentialDecryptFallbackTotal.Add(0)
	// Pre-initialize re-encryption counters per table so they are visible in
	// /metrics before the worker's first sweep.
	for _, table := range []string{"psp_credentials", "notification_configs", "provider_auth_configs"} {
		m.CredentialsReencryptedTotal.WithLabelValues(table).Add(0)
		m.CredentialsReencryptErrorsTotal.WithLabelValues(table).Add(0)
	}
	// Pre-initialize the audit archiver families per operation / result so
	// they render from the first scrape, even before the archiver's first
	// sweep (or while it is disabled and thus never runs).
	for _, op := range []string{"list", "upload", "mark"} {
		m.AuditArchiveErrorsTotal.WithLabelValues(op).Add(0)
	}
	for _, result := range []string{"published", "already_published"} {
		m.AuditAnchorsPublishedTotal.WithLabelValues(result).Add(0)
	}
	// Seed the circuit-breaker families so they render from the very first
	// scrape, even before any downstream breaker exists. The placeholder
	// (empty name) stays at zero; real breakers seed their own name via the
	// initial closed-state transition on creation.
	m.CircuitBreakerState.WithLabelValues("")
	for _, result := range []string{"allowed", "denied", "success", "failure"} {
		m.CircuitBreakerRequestsTotal.WithLabelValues("", result)
	}
	// Pre-initialize the reconciliation drift family per check name (kept in
	// sync with the check names in service/reconciliation.go) so every check
	// is matchable from the first scrape, before the worker's first run.
	for _, check := range []string{
		"subscription_snapshot_freshness",
		"usage_outbox_stuck",
		"invoice_catalog_traceability",
		"outbox_dead_letter",
		"usage_event_orphans",
		"invoice_amount_consistency",
		"invoice_lines_total_match",
		"unpaid_finalized_overdue",
	} {
		m.ReconciliationDrift.WithLabelValues(check).Set(0)
	}
	return m
}

// Handler returns an http.Handler that serves the Prometheus text
// exposition format for this instance's private registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
