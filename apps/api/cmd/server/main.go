// Platform API server: applies embedded migrations at startup, connects as
// the platform_app role, serves the v1 HTTP API and runs the outbox relay.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/config"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/httpapi"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/outbox"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	log.Info("starting platform API", "log_level", cfg.LogLevel)

	ctx := context.Background()
	if err := store.Migrate(ctx, cfg.MigrationDatabaseURL); err != nil {
		log.Error("migration failed", "error", err)
		os.Exit(1)
	}
	log.Info("migrations applied")

	st, err := store.New(ctx, cfg.DatabaseURL)
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
	var encryptor *crypto.Encryptor
	if cfg.PSPMasterKey != "" {
		encryptor, err = crypto.NewEncryptor(cfg.PSPMasterKey)
		if err != nil {
			log.Error("invalid PSP master key", "error", err)
			os.Exit(1)
		}
		log.Info("PSP credential encryption enabled")
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
	if oidcVerifier != nil {
		apiServer.SetOIDCVerifier(oidcVerifier)
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		_ = outbox.NewRelay(st, adapter, cfg.OutboxPollInterval, log).Run(relayCtx)
	}()

	// Reconciliation worker: runs hourly consistency checks.
	reconCtx, stopRecon := context.WithCancel(ctx)
	defer stopRecon()
	reconDone := make(chan struct{})
	go func() {
		defer close(reconDone)
		worker := service.NewReconciliationWorker(svc, cfg.ReconciliationInterval, log)
		_ = worker.Run(reconCtx)
	}()

	// Webhook delivery worker: signs and delivers published outbox events
	// to registered provider endpoints (HMAC-SHA256).
	webhookCtx, stopWebhook := context.WithCancel(ctx)
	defer stopWebhook()
	webhookDone := make(chan struct{})
	go func() {
		defer close(webhookDone)
		wk := webhook.NewWorker(st, log, cfg.WebhookPollInterval)
		_ = wk.Run(webhookCtx)
	}()

	// JIT Support Access expiry sweeper: batch-expires past-due sessions.
	supportCtx, stopSupport := context.WithCancel(ctx)
	defer stopSupport()
	supportDone := make(chan struct{})
	go func() {
		defer close(supportDone)
		sweeper := service.NewSupportExpirySweeper(svc, cfg.SupportSweepInterval, log)
		_ = sweeper.Run(supportCtx)
	}()

	// Hard Quota reservation expiry sweeper: reclaims past-due reservations.
	quotaCtx, stopQuota := context.WithCancel(ctx)
	defer stopQuota()
	quotaDone := make(chan struct{})
	go func() {
		defer close(quotaDone)
		sweeper := service.NewQuotaExpirySweeper(svc, cfg.QuotaSweepInterval, log)
		_ = sweeper.Run(quotaCtx)
	}()

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

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	stopRelay()
	<-relayDone
	stopRecon()
	<-reconDone
	stopWebhook()
	<-webhookDone
	stopSupport()
	<-supportDone
	stopQuota()
	<-quotaDone
	log.Info("shutdown complete")
}
