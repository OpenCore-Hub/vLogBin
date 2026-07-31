package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/jackc/pgx/v5"
)

// ReconciliationCheck is the result of a single consistency check.
type ReconciliationCheck struct {
	Name          string
	Status        string // "ok", "drift", "error"
	ExpectedCount int64
	ActualCount   int64
	DriftCount    int64
	Details       map[string]any
}

// RunReconciliation executes all consistency checks and stores results.
// It uses the operator context to see across all tenants.
func (s *Service) RunReconciliation(ctx context.Context) ([]ReconciliationCheck, error) {
	var results []ReconciliationCheck
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		// Check 1: Active subscriptions without a recent entitlement snapshot.
		staleSubs, err := q.CountActiveSubsWithoutSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("check stale subscriptions: %w", err)
		}
		r1 := ReconciliationCheck{
			Name: "subscription_snapshot_freshness", Status: "ok",
			ExpectedCount: 0, ActualCount: 0, DriftCount: staleSubs,
			Details: map[string]any{"description": "active subscriptions without a snapshot in the last 24h"},
		}
		if staleSubs > 0 {
			r1.Status = "drift"
		}
		results = append(results, r1)
		s.storeReconciliationResult(ctx, q, r1)

		// Check 2: Usage outbox events stuck in 'failed' for > 1 hour.
		stuckOutbox, err := q.CountStuckUsageOutbox(ctx)
		if err != nil {
			return fmt.Errorf("check stuck outbox: %w", err)
		}
		r2 := ReconciliationCheck{
			Name: "usage_outbox_stuck", Status: "ok",
			ExpectedCount: 0, ActualCount: 0, DriftCount: stuckOutbox,
			Details: map[string]any{"description": "usage.accepted outbox events stuck in failed > 1h"},
		}
		if stuckOutbox > 0 {
			r2.Status = "drift"
		}
		results = append(results, r2)
		s.storeReconciliationResult(ctx, q, r2)

		// Check 3: Invoices with subscription but without catalog_version_id.
		missingCV, err := q.CountInvoicesWithoutCatalogVersion(ctx)
		if err != nil {
			return fmt.Errorf("check invoice catalog version: %w", err)
		}
		r3 := ReconciliationCheck{
			Name: "invoice_catalog_traceability", Status: "ok",
			ExpectedCount: 0, ActualCount: 0, DriftCount: missingCV,
			Details: map[string]any{"description": "invoices with subscription but without catalog_version_id"},
		}
		if missingCV > 0 {
			r3.Status = "drift"
		}
		results = append(results, r3)
		s.storeReconciliationResult(ctx, q, r3)

		// Check 4: Dead-lettered outbox events (permanently failed).
		deadLetter, err := q.CountDeadLetterOutbox(ctx)
		if err != nil {
			return fmt.Errorf("check dead letter: %w", err)
		}
		r4 := ReconciliationCheck{
			Name: "outbox_dead_letter", Status: "ok",
			ExpectedCount: 0, ActualCount: 0, DriftCount: deadLetter,
			Details: map[string]any{"description": "outbox events in dead_letter status"},
		}
		if deadLetter > 0 {
			r4.Status = "drift"
		}
		results = append(results, r4)
		s.storeReconciliationResult(ctx, q, r4)

		// Check 5: Orphaned usage events (customer account deleted).
		orphaned, err := q.CountOrphanedUsageEvents(ctx)
		if err != nil {
			return fmt.Errorf("check orphaned usage: %w", err)
		}
		r5 := ReconciliationCheck{
			Name: "usage_event_orphans", Status: "ok",
			ExpectedCount: 0, ActualCount: 0, DriftCount: orphaned,
			Details: map[string]any{"description": "usage events referencing non-existent customer accounts"},
		}
		if orphaned > 0 {
			r5.Status = "drift"
		}
		results = append(results, r5)
		s.storeReconciliationResult(ctx, q, r5)

		return nil
	})
	if err != nil {
		s.log.Error("reconciliation failed", "error", err)
		return nil, err
	}
	return results, nil
}

// storeReconciliationResult persists a single check result. Errors are
// logged but do not abort the reconciliation run.
func (s *Service) storeReconciliationResult(ctx context.Context, q *store.Queries, r ReconciliationCheck) {
	details, _ := json.Marshal(r.Details)
	if err := q.InsertReconciliationResult(ctx, storegen.InsertReconciliationResultParams{
		CheckName:     r.Name,
		Status:        r.Status,
		ExpectedCount: r.ExpectedCount,
		ActualCount:   r.ActualCount,
		DriftCount:    r.DriftCount,
		Details:       details,
	}); err != nil {
		s.log.Error("reconciliation result store failed", "check", r.Name, "error", err)
	}
}

// ListReconciliationResults returns the most recent check results.
func (s *Service) ListReconciliationResults(ctx context.Context, limit int32) ([]storegen.ReconciliationResult, error) {
	var results []storegen.ReconciliationResult
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		r, err := q.ListRecentReconciliationResults(ctx, limit)
		if err != nil {
			return mapErr(err, "reconciliation results")
		}
		results = r
		return nil
	})
	return results, err
}

// ReconciliationWorker runs reconciliation checks on a fixed interval.
type ReconciliationWorker struct {
	svc      *Service
	interval time.Duration
	log      *slog.Logger
}

func NewReconciliationWorker(svc *Service, interval time.Duration, log *slog.Logger) *ReconciliationWorker {
	if interval <= 0 {
		interval = time.Hour
	}
	if log == nil {
		log = slog.Default()
	}
	return &ReconciliationWorker{svc: svc, interval: interval, log: log}
}

func (w *ReconciliationWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	// Run once immediately on startup.
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			w.log.Info("reconciliation worker shutting down")
			return nil
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *ReconciliationWorker) runOnce(ctx context.Context) {
	start := time.Now()
	results, err := w.svc.RunReconciliation(ctx)
	if err != nil {
		w.log.Error("reconciliation run failed", "error", err, "duration", time.Since(start))
		return
	}
	driftCount := 0
	for _, r := range results {
		if r.Status == "drift" {
			driftCount++
			w.log.Warn("reconciliation drift detected",
				"check", r.Name, "drift_count", r.DriftCount, "details", r.Details)
		}
	}
	w.log.Info("reconciliation run completed",
		"checks", len(results), "drifts", driftCount, "duration", time.Since(start))
}

// RunOnce processes one reconciliation cycle synchronously. Exported for tests.
func (w *ReconciliationWorker) RunOnce(ctx context.Context) error {
	w.runOnce(ctx)
	return nil
}
