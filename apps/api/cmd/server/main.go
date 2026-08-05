// Platform API server: applies embedded migrations at startup, connects as
// the platform_app role, serves the v1 HTTP API and runs the outbox relay.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/archive"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/config"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/httpapi"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/outbox"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/ratelimit"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/telemetry"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/worker"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"go.opentelemetry.io/otel"
)

// bgWorker is a background goroutine that can be stopped by calling stop and
// signals its completion via done. Registering every worker in one slice lets
// shutdown treat them uniformly.
type bgWorker struct {
	name string
	stop func()
	done <-chan struct{}
}

// stopWorkers stops every worker concurrently and waits up to timeout for each
// one to finish. Workers that do not exit within the deadline are returned by
// name so the caller can log which components were force-abandoned. Stopping
// in parallel (instead of sequentially) guarantees no single stuck worker
// starves the others of the remaining grace budget.
func stopWorkers(timeout time.Duration, workers []bgWorker) []string {
	deadline := time.Now().Add(timeout)
	var (
		mu   sync.Mutex
		late []string
		wg   sync.WaitGroup
	)
	for _, w := range workers {
		w.stop()
		wg.Add(1)
		go func(w bgWorker) {
			defer wg.Done()
			remaining := time.Until(deadline)
			if remaining <= 0 {
				mu.Lock()
				late = append(late, w.name)
				mu.Unlock()
				return
			}
			t := time.NewTimer(remaining)
			defer t.Stop()
			select {
			case <-w.done:
			case <-t.C:
				mu.Lock()
				late = append(late, w.name)
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return late
}

// setLogLevel maps a config log-level string onto a slog.LevelVar. Unknown
// values fall back to Info so a bad LOG_LEVEL never silences the process.
func setLogLevel(v *slog.LevelVar, s string) {
	switch s {
	case "debug":
		v.Set(slog.LevelDebug)
	case "warn":
		v.Set(slog.LevelWarn)
	case "error":
		v.Set(slog.LevelError)
	default:
		v.Set(slog.LevelInfo)
	}
}

// pprofMux returns a mux exposing the net/http/pprof handlers (index,
// cmdline, profile, symbol, trace). The handlers are bound to an isolated mux
// instead of http.DefaultServeMux so pprof is only reachable on the dedicated
// pprof listener, never through the public API.
func pprofMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	// The log level lives in a LevelVar so the config watcher can hot-reload
	// LOG_LEVEL without restarting the process.
	var levelVar slog.LevelVar
	setLogLevel(&levelVar, cfg.LogLevel)
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &levelVar}))

	log.Info("starting platform API", "log_level", cfg.LogLevel)

	// OpenTelemetry distributed tracing (opt-in via OTEL_ENABLED): installs
	// the global tracer provider, W3C propagators and the span exporter, and
	// returns a shutdown func that flushes pending spans on exit. Instrumented
	// components (httpapi/webhook/billing/store/outbox) resolve their tracer
	// through the global provider, so this wiring is their single hook.
	otelShutdown, err := telemetry.Setup(telemetry.Config{
		Enabled:            cfg.Telemetry.Enabled,
		Exporter:           cfg.Telemetry.Exporter,
		OTLPEndpoint:       cfg.Telemetry.OTLPEndpoint,
		ServiceName:        cfg.Telemetry.ServiceName,
		Environment:        cfg.Telemetry.Environment,
		SampleRatio:        cfg.Telemetry.SampleRatio,
		BatchTimeout:       cfg.Telemetry.BatchTimeout,
		ExportTimeout:      cfg.Telemetry.ExportTimeout,
		MaxQueueSize:       cfg.Telemetry.MaxQueueSize,
		MaxExportBatchSize: cfg.Telemetry.MaxExportBatchSize,
	})
	if err != nil {
		log.Error("telemetry setup failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			log.Warn("telemetry shutdown failed", "error", err)
		}
	}()
	if cfg.Telemetry.Enabled {
		log.Info("distributed tracing enabled",
			"exporter", cfg.Telemetry.Exporter,
			"endpoint", cfg.Telemetry.OTLPEndpoint,
			"sample_ratio", cfg.Telemetry.SampleRatio,
		)
	}

	ctx := context.Background()
	if err := store.Migrate(ctx, cfg.MigrationDatabaseURL); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}
	log.Info("migrations applied")

	storeOpts := []store.Option{store.WithConfig(store.Config{
		MaxConns:           int32(cfg.DBMaxConns),
		MinConns:           int32(cfg.DBMinConns),
		MaxConnLifetime:    cfg.DBMaxConnLifetime,
		MaxConnIdleTime:    cfg.DBMaxConnIdleTime,
		HealthCheckPeriod:  cfg.DBHealthCheckPeriod,
		QueryTimeout:       cfg.DBQueryTimeout,
		SlowQueryThreshold: cfg.DBQuerySlowThreshold,
	})}
	// Only wire the DB query tracer when distributed tracing is on, so pools
	// without tracing keep zero per-query overhead.
	if cfg.Telemetry.Enabled {
		storeOpts = append(storeOpts, store.WithQueryTracerProvider(otel.GetTracerProvider()))
	}
	st, err := store.New(ctx, cfg.DatabaseURL, storeOpts...)
	if err != nil {
		log.Error("database connect failed", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	adapter, err := billing.New(cfg.BillingAdapter, cfg.LagoAPIURL, cfg.LagoAPIKey)
	if err != nil {
		log.Error("billing adapter init failed", "error", err)
		os.Exit(1)
	}
	log.Info("billing adapter configured", "adapter", adapter.Name())

	// PSP credential encryption (optional — only if PSP_MASTER_KEY is set).
	// Previous keys (PSP_MASTER_KEY_PREVIOUS) are loaded for decrypt-only
	// fallback, enabling zero-downtime master key rotation: new credentials
	// are sealed with the active key while ciphertext written under older
	// keys stays readable.
	var encryptor *crypto.Encryptor
	if cfg.PSPMasterKey != "" {
		encryptor, err = crypto.NewEncryptorWithPrevious(cfg.PSPMasterKey, cfg.PSPMasterKeyPrevious)
		if err != nil {
			log.Error("invalid PSP master key", "error", err)
			os.Exit(1)
		}
		log.Info("PSP credential encryption enabled",
			"previous_keys", len(cfg.PSPMasterKeyPrevious))
	}

	// ZITADEL OIDC + Management API (optional — when ZITADEL_URL is set).
	var zitadelMgmt *zitadel.ManagementClient
	var oidcVerifier *zitadel.Verifier
	if cfg.ZITADELURL != "" {
		v, err := zitadel.NewVerifier(ctx, cfg.ZITADELURL)
		if err != nil {
			log.Error("ZITADEL OIDC verifier init failed", "error", err, "url", cfg.ZITADELURL)
			os.Exit(1)
		}
		oidcVerifier = v
		log.Info("ZITADEL OIDC verification enabled", "issuer", cfg.ZITADELURL)

		if cfg.ZITADELPAT != "" {
			zitadelMgmt = zitadel.NewManagementClient(cfg.ZITADELURL, cfg.ZITADELPAT)
			log.Info("ZITADEL Management API enabled")
		}
	} else {
		log.Info("ZITADEL OIDC not configured; using static OPERATOR_TOKEN")
	}

	svc := service.New(st, cfg.PlatformBaseDomain,
		service.WithLogger(log),
		service.WithBillingAdapter(adapter),
		service.WithCryptoEncryptor(encryptor),
		service.WithZITADELManagement(zitadelMgmt, cfg.ZITADELURL),
		service.WithUsageLateWindow(cfg.UsageLateWindow),
		service.WithUsageFutureSkew(5*time.Minute),
	)
	apiServer := httpapi.NewServer(st, svc, cfg.OperatorToken, log)
	apiServer.SetCORSOrigins(cfg.CORSAllowedOrigins)
	apiServer.SetRateLimits(cfg.RateLimits)

	// WORM audit archiver: publishes audit hash chain anchors to S3-compatible
	// object storage (AUDIT_ARCHIVE_SWEEP_INTERVAL, default 0 = disabled).
	// This is the external-anchoring half of the tamper-proof audit chain: the
	// archived (tail_event_id, tail_hash) checkpoints live outside the DB, so a
	// database superuser rewriting the chain would diverge from the immutable
	// WORM copies and be caught by verification. Credentials never reach the
	// database role — only this process holds them. Enabled when the interval
	// is set AND the storage config is complete (Load rejects partial config).
	if cfg.AuditArchiveSweepInterval > 0 {
		archiver, err := archive.NewArchiver(
			cfg.AuditArchiveObjectStorage.Endpoint,
			cfg.AuditArchiveObjectStorage.Bucket,
			cfg.AuditArchiveObjectStorage.AccessKey,
			cfg.AuditArchiveObjectStorage.SecretKey,
			cfg.AuditArchiveObjectStorage.Region,
			cfg.AuditArchiveObjectStorage.UseSSL,
			log,
		)
		if err != nil {
			log.Error("audit archiver init failed", "error", err)
			os.Exit(1)
		}
		svc.SetAuditArchiver(archiver)
		// Report per-sweep progress so operators can watch the external
		// anchoring converge: audit_anchors_published_total grows until the
		// anchor backlog is drained; failures surface per operation as
		// audit_archive_errors_total{op="list|upload|mark"}.
		svc.SetAuditArchiveReporter(func(published, alreadyPublished, listErrors, uploadErrors, markErrors int64) {
			m := apiServer.Metrics()
			m.AuditAnchorsPublishedTotal.WithLabelValues("published").Add(float64(published))
			m.AuditAnchorsPublishedTotal.WithLabelValues("already_published").Add(float64(alreadyPublished))
			m.AuditArchiveErrorsTotal.WithLabelValues("list").Add(float64(listErrors))
			m.AuditArchiveErrorsTotal.WithLabelValues("upload").Add(float64(uploadErrors))
			m.AuditArchiveErrorsTotal.WithLabelValues("mark").Add(float64(markErrors))
		})
		log.Info("audit archiver enabled",
			"endpoint", cfg.AuditArchiveObjectStorage.Endpoint,
			"bucket", cfg.AuditArchiveObjectStorage.Bucket,
			"sweep_interval", cfg.AuditArchiveSweepInterval.String(),
			"batch_size", cfg.AuditArchiveBatchSize,
		)
	}
	// Count post-rotation reads of legacy ciphertext via the decrypt fallback
	// counter, so operators can see how much credential data is still sealed
	// with rotated-out keys and schedule re-encryption to converge.
	if encryptor != nil {
		encryptor.SetFallbackObserver(func() {
			apiServer.Metrics().CredentialDecryptFallbackTotal.Inc()
		})
		// Report per-table re-encryption progress so operators can watch the
		// rotation converge: credentials_reencrypted_total grows until legacy
		// ciphertext is exhausted, then the old key can be dropped from
		// PSP_MASTER_KEY_PREVIOUS. Rows no key can open are counted as errors
		// — they block full convergence and need manual attention.
		svc.SetReencryptReporter(func(table string, reencrypted, errors int64) {
			m := apiServer.Metrics()
			m.CredentialsReencryptedTotal.WithLabelValues(table).Add(float64(reencrypted))
			m.CredentialsReencryptErrorsTotal.WithLabelValues(table).Add(float64(errors))
		})
	}
	// Rate-limiter backend: memory (default) for single-instance deployments;
	// Redis-backed for multi-instance deployments where per-process counters
	// would not be shared. Redis failures fail open — the request is admitted
	// unthrottled and the outage is surfaced through
	// rate_limiter_backend_errors_total — so a Redis outage degrades rate
	// limiting instead of taking the API down.
	switch cfg.RateLimitBackend {
	case "redis":
		rl, err := ratelimit.NewRedisLimiter(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, ratelimit.RedisLimiterOptions{
			OnError: func(err error) {
				apiServer.Metrics().RateLimiterBackendErrorsTotal.Inc()
				log.Warn("rate limiter backend error; failing open", "error", err)
			},
		})
		if err != nil {
			log.Error("redis rate limiter init failed", "error", err)
			os.Exit(1)
		}
		defer rl.Close()
		apiServer.SetRateLimiter(rl)
		log.Info("rate limiting backend: redis", "addr", cfg.RedisAddr)
	default:
		log.Info("rate limiting backend: memory")
	}
	apiServer.SetRequestTimeout(cfg.HTTPRequestTimeout)
	apiServer.SetReadyTimeout(cfg.ReadyTimeout)
	apiServer.SetSlowRequestThreshold(cfg.SlowRequestThreshold)
	apiServer.SetIdempotencyTTL(cfg.IdempotencyTTL)
	// Classified store errors (timeout / not_found / conflict / connection /
	// other) are exported as store_errors_total so failure modes are alertable
	// and don't require log grepping.
	st.SetErrorObserver(func(op string, cls store.ErrorClass) {
		apiServer.Metrics().StoreErrorsTotal.WithLabelValues(op, string(cls)).Inc()
	})
	// SQL statements that outlive DB_SLOW_QUERY_THRESHOLD are counted and
	// logged at Warn so DB-level latency regressions are alertable even when
	// no HTTP request is slow (background workers poll in the background).
	// The query text is logged without parameters, which may carry secrets.
	if cfg.DBQuerySlowThreshold > 0 {
		st.SetSlowQueryObserver(func(sql string, d time.Duration, err error) {
			apiServer.Metrics().DBQuerySlowTotal.Inc()
			attrs := []any{"sql", sql, "duration_ms", d.Milliseconds(), "threshold_ms", cfg.DBQuerySlowThreshold.Milliseconds()}
			if err != nil {
				attrs = append(attrs, "error", err)
			}
			log.Warn("slow database query", attrs...)
		})
	}
	if oidcVerifier != nil {
		apiServer.SetOIDCVerifier(oidcVerifier)
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		// Hard connection-level bounds: ReadTimeout defeats slowloris and slow
		// body uploads, WriteTimeout bounds response writes, IdleTimeout reaps
		// silent keep-alive sockets, MaxHeaderBytes caps header size. Together
		// with the request-timeout middleware they form defense in depth.
		ReadTimeout:    cfg.HTTPReadTimeout,
		WriteTimeout:   cfg.HTTPWriteTimeout,
		IdleTimeout:    cfg.HTTPIdleTimeout,
		MaxHeaderBytes: cfg.HTTPMaxHeaderBytes,
	}

	// Worker supervisor: every background worker runs under a supervisor that
	// recovers panics and restarts workers that exit unexpectedly, with
	// exponential backoff capped at WORKER_BACKOFF_MAX. A worker that dies
	// silently used to mean silent production impact (stale outbox, expired
	// sessions never purged); now restarts and panics are logged and exported
	// as worker_restarts_total / worker_panics_total so crash-looping is
	// visible to alerting instead of hiding in a quiet goroutine.
	sup := worker.New(log, worker.Options{
		BackoffMax: cfg.WorkerBackoffMax,
		OnPanic: func(name string, _ []byte) {
			apiServer.Metrics().WorkerPanicsTotal.WithLabelValues(name).Inc()
		},
		OnRestart: func(name, reason string) {
			apiServer.Metrics().WorkerRestartsTotal.WithLabelValues(name, reason).Inc()
		},
	})

	// Every background worker is registered here so shutdown can stop them
	// uniformly and concurrently within the grace budget.
	var workers []bgWorker

	// Config watcher: periodically re-reads the environment (and on SIGHUP)
	// and applies hot-reloadable settings — rate limits, slow request
	// threshold, CORS origins and log level — without a restart. Disabled by
	// default (CONFIG_RELOAD_INTERVAL=0); SIGHUP still triggers a reload.
	configWatcher := config.NewWatcher(cfg.ConfigReloadInterval, log, cfg, func(c config.Change) {
		for _, f := range c.Fields {
			switch f {
			case config.FieldRateLimits:
				apiServer.SetRateLimits(c.Config.RateLimits)
				log.Info("hot-reloaded rate limits",
					"provider", c.Config.RateLimits.Provider,
					"environment", c.Config.RateLimits.Environment,
					"credential", c.Config.RateLimits.Credential,
					"endpoint", c.Config.RateLimits.Endpoint,
					"ip", c.Config.RateLimits.IP)
			case config.FieldSlowRequestThreshold:
				apiServer.SetSlowRequestThreshold(c.Config.SlowRequestThreshold)
				log.Info("hot-reloaded slow request threshold",
					"threshold", c.Config.SlowRequestThreshold)
			case config.FieldCORSOrigins:
				apiServer.SetCORSOrigins(c.Config.CORSAllowedOrigins)
				log.Info("hot-reloaded CORS origins",
					"origins", c.Config.CORSAllowedOrigins)
			case config.FieldLogLevel:
				setLogLevel(&levelVar, c.Config.LogLevel)
				log.Info("hot-reloaded log level", "level", c.Config.LogLevel)
			}
		}
	})
	configCtx, stopConfig := context.WithCancel(ctx)
	defer stopConfig()
	// SIGHUP triggers an immediate reload (standard Unix admin reload); the
	// periodic ticker handles CONFIG_RELOAD_INTERVAL>0. The watcher's Reload
	// is mutex-guarded, so concurrent triggers are serialized.
	configDone := sup.Run(configCtx, "config-reload", func(ctx context.Context) error {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		defer signal.Stop(hup)
		done := make(chan struct{})
		go func() {
			defer close(done)
			configWatcher.Run(ctx)
		}()
		for {
			select {
			case <-ctx.Done():
				<-done
				return nil
			case <-hup:
				configWatcher.Reload()
			}
		}
	})
	workers = append(workers, bgWorker{name: "config-reload", stop: stopConfig, done: configDone})

	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	relayDone := sup.Run(relayCtx, "outbox-relay", func(ctx context.Context) error {
		relay := outbox.NewRelay(st, adapter, cfg.OutboxPollInterval, log).
			WithCircuitBreaker(cfg.CircuitBreakers.ToOptions()).
			WithMetrics(apiServer.Metrics())
		return relay.Run(ctx)
	})
	workers = append(workers, bgWorker{name: "outbox-relay", stop: stopRelay, done: relayDone})

	// Reconciliation worker: runs hourly consistency checks.
	reconCtx, stopRecon := context.WithCancel(ctx)
	defer stopRecon()
	reconDone := sup.Run(reconCtx, "reconciliation", func(ctx context.Context) error {
		return service.NewReconciliationWorker(svc, cfg.ReconciliationInterval, log, apiServer.Metrics()).Run(ctx)
	})
	workers = append(workers, bgWorker{name: "reconciliation", stop: stopRecon, done: reconDone})

	// Webhook delivery worker: signs and delivers published outbox events
	// to registered provider endpoints (HMAC-SHA256).
	webhookCtx, stopWebhook := context.WithCancel(ctx)
	defer stopWebhook()
	webhookDone := sup.Run(webhookCtx, "webhook-delivery", func(ctx context.Context) error {
		wk := webhook.NewWorker(st, log, cfg.WebhookPollInterval).
			WithMetrics(apiServer.Metrics()).
			SetBreakerOptions(cfg.CircuitBreakers.ToOptions())
		return wk.Run(ctx)
	})
	workers = append(workers, bgWorker{name: "webhook-delivery", stop: stopWebhook, done: webhookDone})

	// JIT Support Access expiry sweeper: batch-expires past-due sessions.
	supportCtx, stopSupport := context.WithCancel(ctx)
	defer stopSupport()
	supportDone := sup.Run(supportCtx, "support-sweeper", func(ctx context.Context) error {
		return service.NewSupportExpirySweeper(svc, cfg.SupportSweepInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
	})
	workers = append(workers, bgWorker{name: "support-sweeper", stop: stopSupport, done: supportDone})

	// Hard Quota reservation expiry sweeper: reclaims past-due reservations.
	quotaCtx, stopQuota := context.WithCancel(ctx)
	defer stopQuota()
	quotaDone := sup.Run(quotaCtx, "quota-sweeper", func(ctx context.Context) error {
		return service.NewQuotaExpirySweeper(svc, cfg.QuotaSweepInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
	})
	workers = append(workers, bgWorker{name: "quota-sweeper", stop: stopQuota, done: quotaDone})

	// Webhook retention sweeper: purges terminal webhook deliveries and
	// outbox events beyond the retention window (default 30d) so the two
	// tables do not grow without bound.
	retentionCtx, stopRetention := context.WithCancel(ctx)
	defer stopRetention()
	retentionDone := sup.Run(retentionCtx, "retention-sweeper", func(ctx context.Context) error {
		return service.NewWebhookRetentionSweeper(svc, cfg.WebhookRetentionDays, cfg.WebhookRetentionSweepInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
	})
	workers = append(workers, bgWorker{name: "retention-sweeper", stop: stopRetention, done: retentionDone})

	// Audit retention sweeper: purges audit events beyond the retention window
	// (AUDIT_RETENTION_DAYS, default 0 = disabled). Audit events are compliance
	// evidence, so this worker only runs when retention is explicitly enabled;
	// the append-only invariant is preserved because deletion goes through the
	// operator-only purge_audit_events function (migration 0030).
	if cfg.AuditRetentionDays > 0 {
		auditCtx, stopAudit := context.WithCancel(ctx)
		defer stopAudit()
		auditDone := sup.Run(auditCtx, "audit-retention-sweeper", func(ctx context.Context) error {
			return service.NewAuditRetentionSweeper(svc, cfg.AuditRetentionDays, cfg.AuditRetentionSweepInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
		})
		workers = append(workers, bgWorker{name: "audit-retention-sweeper", stop: stopAudit, done: auditDone})
	}

	// Audit hash chain anchor sweeper: periodically checkpoints the
	// tamper-evident audit chain tail (AUDIT_CHAIN_ANCHOR_INTERVAL, default
	// 24h). Anchors bound the incremental verification window and are the rows
	// external anchoring (WORM object storage) will publish outside the DB.
	// Anchoring only appends one row per tick and never mutates audit data, so
	// it is safe to run by default; 0 disables it (manual anchors only).
	if cfg.AuditChainAnchorInterval > 0 {
		chainCtx, stopChain := context.WithCancel(ctx)
		defer stopChain()
		chainDone := sup.Run(chainCtx, "audit-chain-anchor-sweeper", func(ctx context.Context) error {
			return service.NewAuditChainAnchorSweeper(svc, "sweeper", cfg.AuditChainAnchorInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
		})
		workers = append(workers, bgWorker{name: "audit-chain-anchor-sweeper", stop: stopChain, done: chainDone})
	}

	// Audit archiver: publishes audit chain anchors to WORM object storage
	// (AUDIT_ARCHIVE_SWEEP_INTERVAL, default 0 = disabled). This is the
	// external-anchoring half of the tamper-proof chain: each published anchor
	// checkpoints the chain tail outside the DB, so a DB superuser rewriting
	// the chain would diverge from the immutable archived copies. Publishing
	// is idempotent (deterministic object keys + published_at guard), so
	// crashes between upload and mark leave no half state. Requires the
	// archiver to be wired above, which happens exactly when the interval is
	// configured; the worker is otherwise a no-op.
	if cfg.AuditArchiveSweepInterval > 0 {
		archiveCtx, stopArchive := context.WithCancel(ctx)
		defer stopArchive()
		archiveDone := sup.Run(archiveCtx, "audit-archiver", func(ctx context.Context) error {
			return service.NewAuditArchiveSweeper(svc, cfg.AuditArchiveBatchSize, cfg.AuditArchiveSweepInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
		})
		workers = append(workers, bgWorker{name: "audit-archiver", stop: stopArchive, done: archiveDone})
	}

	// Idempotency sweeper: purges Idempotency-Key records past their TTL so
	// the idempotency_keys table does not grow without bound.
	idemCtx, stopIdem := context.WithCancel(ctx)
	defer stopIdem()
	idemDone := sup.Run(idemCtx, "idempotency-sweeper", func(ctx context.Context) error {
		return service.NewIdempotencyKeySweeper(svc, cfg.IdempotencyTTL, cfg.IdempotencySweepInterval, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
	})
	workers = append(workers, bgWorker{name: "idempotency-sweeper", stop: stopIdem, done: idemDone})

	// Credential re-encryption sweeper: re-seals ciphertext written under a
	// rotated-out PSP master key with the active key, converging legacy
	// credentials so credential_decrypt_fallback_total drops to zero and the
	// old key can be dropped from PSP_MASTER_KEY_PREVIOUS. Only meaningful
	// while previous keys are configured, so it is started only then
	// (REENCRYPT_SWEEP_INTERVAL > 0 enables it).
	if cfg.ReencryptSweepInterval > 0 && encryptor != nil && len(cfg.PSPMasterKeyPrevious) > 0 {
		reencryptCtx, stopReencrypt := context.WithCancel(ctx)
		defer stopReencrypt()
		reencryptDone := sup.Run(reencryptCtx, "credential-reencryption", func(ctx context.Context) error {
			return service.NewReencryptionSweeper(svc, cfg.ReencryptSweepInterval, cfg.ReencryptBatchSize, log).WithMetrics(apiServer.Metrics()).WithQueryTimeout(cfg.DBQueryTimeout).Run(ctx)
		})
		workers = append(workers, bgWorker{name: "credential-reencryption", stop: stopReencrypt, done: reencryptDone})
	}

	// Backlog reporter: refreshes the outbox/delivery gauges so Prometheus
	// alerts on pending backlog actually fire. Disabled when METRICS_ENABLED
	// is false (the gauges simply stay at zero).
	backlogCtx, stopBacklog := context.WithCancel(ctx)
	defer stopBacklog()
	backlogDone := sup.Run(backlogCtx, "backlog-reporter", func(ctx context.Context) error {
		m := apiServer.Metrics()
		report := func() {
			ob, err := svc.OutboxBacklog(ctx)
			if err != nil {
				log.Warn("outbox backlog refresh failed", "error", err)
			} else {
				for status, n := range ob {
					m.OutboxEventsTotal.WithLabelValues(status).Set(float64(n))
				}
			}
			wb, err := svc.WebhookBacklog(ctx)
			if err != nil {
				log.Warn("webhook backlog refresh failed", "error", err)
			} else {
				for status, n := range wb {
					m.WebhookDeliveriesGauge.WithLabelValues(status).Set(float64(n))
				}
			}
		}
		report()
		ticker := time.NewTicker(cfg.MetricsSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				report()
			}
		}
	})
	workers = append(workers, bgWorker{name: "backlog-reporter", stop: stopBacklog, done: backlogDone})

	// Cell migration scheduler: auto-prechecks scheduled migrations.
	migSchedCtx, stopMigSched := context.WithCancel(ctx)
	defer stopMigSched()
	migSchedDone := sup.Run(migSchedCtx, "migration-scheduler", func(ctx context.Context) error {
		return service.NewMigrationScheduler(svc, cfg.MigrationScheduleInterval, log).Run(ctx)
	})
	workers = append(workers, bgWorker{name: "migration-scheduler", stop: stopMigSched, done: migSchedDone})

	// Pool reporter: refreshes db_pool_* gauges from pgxpool statistics so
	// connection-pool exhaustion (acquired == max, rising empty-acquire
	// waits) is visible to Prometheus alerting. The cumulative counters are
	// applied as deltas so they behave like proper counters across restarts.
	poolCtx, stopPool := context.WithCancel(ctx)
	defer stopPool()
	poolDone := sup.Run(poolCtx, "pool-reporter", func(ctx context.Context) error {
		m := apiServer.Metrics()
		// Delta baseline. A nil snapshot means "first run": gauges are set and
		// the baseline is recorded, but cumulative counters contribute no
		// delta yet. A zero-value pgxpool.Stat cannot be used here because its
		// internal puddle.Stat pointer is nil and dereferencing it panics.
		var last *pgxpool.Stat
		report := func() {
			poolStat := st.PoolStats()
			m.DBPoolMaxConns.Set(float64(poolStat.MaxConns()))
			m.DBPoolAcquiredConns.Set(float64(poolStat.AcquiredConns()))
			m.DBPoolIdleConns.Set(float64(poolStat.IdleConns()))
			m.DBPoolConstructingConns.Set(float64(poolStat.ConstructingConns()))
			if last != nil {
				if n := poolStat.AcquireCount() - last.AcquireCount(); n > 0 {
					m.DBPoolAcquireTotal.Add(float64(n))
				}
				if d := poolStat.AcquireDuration() - last.AcquireDuration(); d > 0 {
					m.DBPoolAcquireSeconds.Add(d.Seconds())
				}
				if n := poolStat.EmptyAcquireCount() - last.EmptyAcquireCount(); n > 0 {
					m.DBPoolEmptyAcquireTotal.Add(float64(n))
				}
			}
			last = poolStat
		}
		report()
		ticker := time.NewTicker(cfg.MetricsSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				report()
			}
		}
	})
	workers = append(workers, bgWorker{name: "pool-reporter", stop: stopPool, done: poolDone})

	// pprof server: only starts when PPROF_ENABLED=true (production default
	// is off). It runs on a dedicated listener so deep profiling can never
	// widen the attack surface of the public API, and it participates in the
	// same graceful shutdown as every other worker.
	if cfg.PprofEnabled {
		pprofSrv := &http.Server{
			Addr:              cfg.PprofAddr,
			Handler:           pprofMux(),
			ReadHeaderTimeout: 10 * time.Second,
		}
		pprofCtx, stopPprof := context.WithCancel(ctx)
		defer stopPprof()
		// The stop hook cancels the context before Shutdown so the listener
		// returns with a cancelled ctx — the supervisor treats that as a
		// graceful exit and does not restart it. A real serve failure (port
		// conflict, unexpected error) is restarted like any other worker.
		pprofDone := sup.Run(pprofCtx, "pprof", func(srvCtx context.Context) error {
			log.Info("pprof endpoint enabled", "addr", cfg.PprofAddr)
			if err := pprofSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		workers = append(workers, bgWorker{
			name: "pprof",
			stop: func() {
				stopPprof()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = pprofSrv.Shutdown(shutdownCtx)
			},
			done: pprofDone,
		})
	}

	// All migrations, the connection pool, billing, ZITADEL and every
	// background worker are now initialized: the startup probe can flip to
	// ready. Kubernetes startupProbe will stop getting 503s from here on.
	apiServer.SetStartupComplete()

	go func() {
		log.Info("platform API listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Info("shutting down", "grace_period", cfg.ShutdownGracePeriod)
	shutdownCtx, cancel := context.WithTimeout(ctx, cfg.ShutdownGracePeriod)
	defer cancel()

	// Phase 1: stop accepting new connections and drain in-flight HTTP
	// requests. If the grace budget runs out, force-close remaining
	// connections so the process can still exit.
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown incomplete; force closing connections", "error", err)
		_ = srv.Close()
	} else {
		log.Info("http drained")
	}

	// Phase 2: stop every background worker concurrently and wait for each
	// within the remaining grace budget. Workers that don't exit are reported
	// so operators can see which component was force-abandoned.
	if late := stopWorkers(cfg.ShutdownGracePeriod, workers); len(late) > 0 {
		log.Warn("workers did not stop within grace period", "workers", late)
	} else {
		log.Info("workers stopped")
	}
	log.Info("shutdown complete")
}
