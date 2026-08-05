// Package store owns the PostgreSQL connection pool, migrations and the
// tenant-scoped transaction helpers. All tenant queries MUST go through
// WithTenant so that RLS settings are applied; operator queries go through
// WithOperator. The pool must connect as the platform_app role — connecting
// as a superuser would bypass row level security entirely.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/db"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"go.opentelemetry.io/otel/trace"
)

// Queries aliases the sqlc-generated query set.
type Queries = storegen.Queries

// Config tunes the PostgreSQL connection pool. Zero values keep pgxpool
// defaults (backwards compatible with the plain New(ctx, url) call).
type Config struct {
	// MaxConns bounds the pool size (default: pgxpool's GOMAXPROCS-derived
	// default). Explicit sizing prevents surprise connection storms.
	MaxConns int32
	// MinConns keeps idle connections warm, removing first-request latency
	// spikes after idle periods (default 0, lazy pool).
	MinConns int32
	// MaxConnLifetime cycles connections out after the given age, guarding
	// against stale state after Postgres restarts or failover.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime closes connections idle beyond this window.
	MaxConnIdleTime time.Duration
	// HealthCheckPeriod is how often the pool pings idle connections.
	HealthCheckPeriod time.Duration
	// QueryTimeout caps a single store operation executed through
	// WithQueryTimeout. Zero disables it. Background workers should always
	// use WithQueryTimeout so a hung query cannot stall a poll loop forever.
	QueryTimeout time.Duration
	// SlowQueryThreshold reports queries that outlive it to the slow-query
	// observer. Zero disables slow-query tracing entirely (the tracer is not
	// installed, so no per-query overhead exists).
	SlowQueryThreshold time.Duration
	// QueryTracerProvider, when non-nil, instruments every pool query with an
	// OpenTelemetry span. main wires this only when distributed tracing is
	// enabled, so pools without tracing pay zero per-query overhead.
	QueryTracerProvider trace.TracerProvider
}

// Option mutates a Config. Pass zero or more to New.
type Option func(*Config)

// WithConfig applies the given pool configuration wholesale.
func WithConfig(c Config) Option {
	return func(cfg *Config) { *cfg = c }
}

type Store struct {
	pool          *pgxpool.Pool
	queryTimeout  time.Duration
	errorObserver atomic.Pointer[ErrorObserver]
	slowObserver  atomic.Pointer[SlowQueryObserver]
}

// ErrorClass classifies a store operation error for observability. Operators
// can alert on failure modes (timeouts, conflicts, connectivity) instead of
// grepping logs for individual messages.
type ErrorClass string

const (
	// ErrorClassTimeout covers context deadlines/cancellations and slow
	// queries killed by WithQueryTimeout.
	ErrorClassTimeout ErrorClass = "timeout"
	// ErrorClassNotFound covers empty result sets (pgx.ErrNoRows).
	ErrorClassNotFound ErrorClass = "not_found"
	// ErrorClassConflict covers integrity constraint violations (SQLSTATE
	// class 23): unique, foreign-key and not-null violations usually surface
	// as retriable business conflicts.
	ErrorClassConflict ErrorClass = "conflict"
	// ErrorClassConnection covers network/connection failures that are safe
	// to retry and may indicate DB restarts, failover or pool exhaustion.
	ErrorClassConnection ErrorClass = "connection"
	// ErrorClassOther is the fallback for anything unclassified.
	ErrorClassOther ErrorClass = "other"
)

// ClassifyError maps an error to an ErrorClass. Unknown errors fall back to
// ErrorClassOther.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassOther
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorClassTimeout
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return ErrorClassNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE class 23 — integrity constraint violation.
		if len(pgErr.Code) >= 2 && pgErr.Code[:2] == "23" {
			return ErrorClassConflict
		}
	}
	if pgconn.SafeToRetry(err) {
		return ErrorClassConnection
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return ErrorClassConnection
	}
	return ErrorClassOther
}

// ErrorObserver receives classified store errors. main wires it to a
// Prometheus counter (store_errors_total), keeping the store package free of
// metrics dependencies.
type ErrorObserver func(op string, class ErrorClass)

// SetErrorObserver installs the error observer. Call once during startup
// before workers spawn; observeError uses atomic loads so concurrent readers
// are race-free.
func (s *Store) SetErrorObserver(fn ErrorObserver) {
	if fn != nil {
		s.errorObserver.Store(&fn)
	}
}

func (s *Store) observeError(op string, err error) {
	if err == nil {
		return
	}
	if p := s.errorObserver.Load(); p != nil {
		(*p)(op, ClassifyError(err))
	}
}

// SetSlowQueryObserver installs the slow-query observer. Call once during
// startup; the observer is only invoked for queries that exceeded
// SlowQueryThreshold. Pass nil to disable reporting.
func (s *Store) SetSlowQueryObserver(fn SlowQueryObserver) {
	if fn != nil {
		s.slowObserver.Store(&fn)
	}
}

// rollbackTx abandons a transaction with an independent context so a caller
// whose context was cancelled (deadline hit, shutdown) still gets its
// connection released back to the pool. Safe to call after a successful
// Commit — pgx reports ErrTxClosed, which is ignored. Rollback failures are
// classified into metrics so release problems become visible.
func (s *Store) rollbackTx(tx pgx.Tx) {
	// WithoutCancel: the caller's ctx may already be done; the rollback must
	// still reach the server to avoid leaking the pooled connection.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 5*time.Second)
	defer cancel()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		s.observeError("rollback", err)
	}
}

// Ping verifies that the database is reachable. Used by the /ready
// health-check endpoint for Kubernetes readiness probes.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func New(ctx context.Context, databaseURL string, opts ...Option) (*Store, error) {
	var cfg Config
	for _, o := range opts {
		o(&cfg)
	}

	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	applyPoolConfig(poolCfg, cfg)

	s := &Store{queryTimeout: cfg.QueryTimeout}
	// Install the slow-query tracer and the OpenTelemetry query tracer only
	// when configured, so pools without tracing pay zero per-query overhead.
	// The observer pointer is shared with the Store so SetSlowQueryObserver
	// updates are visible to every pooled connection. Multiple tracers are
	// chained so both observations coexist.
	var tracers []pgxTracer
	if cfg.SlowQueryThreshold > 0 {
		tracers = append(tracers, &queryTracer{
			threshold: cfg.SlowQueryThreshold,
			observer:  &s.slowObserver,
		})
	}
	if cfg.QueryTracerProvider != nil {
		tracers = append(tracers, &otelQueryTracer{
			tracer: cfg.QueryTracerProvider.Tracer("vlogbin.store"),
		})
	}
	switch len(tracers) {
	case 1:
		poolCfg.ConnConfig.Tracer = tracers[0]
	case 2:
		poolCfg.ConnConfig.Tracer = &multiQueryTracer{tracers: tracers}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s.pool = pool
	return s, nil
}

// applyPoolConfig overlays non-zero Config values onto the parsed pool
// config. Zero values leave the pgxpool default in place.
func applyPoolConfig(poolCfg *pgxpool.Config, cfg Config) {
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if cfg.HealthCheckPeriod > 0 {
		poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod
	}
}

func (s *Store) Close() { s.pool.Close() }

// Pool exposes the raw pool for infrastructure code (tests, relay).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// PoolStats returns a snapshot of the underlying pgxpool statistics. The
// pool-reporter worker consumes it to refresh the db_pool_* Prometheus
// gauges/counters so capacity exhaustion is visible and alertable.
func (s *Store) PoolStats() *pgxpool.Stat {
	return s.pool.Stat()
}

// WithQueryTimeout returns a context that expires after the configured query
// timeout (if any). When no timeout is configured the original context is
// returned unchanged with a no-op cancel. Callers running long-lived loops
// (webhook worker, sweepers, relay) should derive one ctx per operation so a
// hung query cannot stall the loop forever.
func (s *Store) WithQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.queryTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, s.queryTimeout)
}

// Migrate applies embedded goose migrations using the given DSN (typically a
// superuser DSN; the runtime DSN's platform_app role has no DDL rights).
func Migrate(ctx context.Context, migrationURL string) error {
	sqlDB, err := sql.Open("pgx", migrationURL)
	if err != nil {
		return fmt.Errorf("open migration db: %w", err)
	}
	defer sqlDB.Close()

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.UpContext(ctx, sqlDB, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// WithTenant runs fn inside a transaction with app.provider_id and
// app.environment_id set via SET LOCAL (transaction-scoped). It never sets
// app.is_operator: tenant queries cannot escalate. set_config passes the
// values as parameters, so no quoting/escaping hazards exist.
func (s *Store) WithTenant(ctx context.Context, tc tenant.Ctx, fn func(tx pgx.Tx, q *Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		s.observeError("tenant.begin", err)
		return err
	}
	// rollbackTx releases the connection even if fn panics or the caller's
	// context is already cancelled.
	defer s.rollbackTx(tx)

	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.provider_id', $1, true), set_config('app.environment_id', $2, true)`,
		tc.ProviderID.String(), tc.EnvironmentID.String()); err != nil {
		s.observeError("tenant", err)
		return fmt.Errorf("set tenant context: %w", err)
	}
	if err := fn(tx, storegen.New(tx)); err != nil {
		s.observeError("tenant", err)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		s.observeError("tenant.commit", err)
		return err
	}
	return nil
}

// WithOperator runs fn inside a transaction with the operator RLS bypass
// enabled (app.is_operator = 'on'). Only the operator-authenticated code
// paths may call this.
func (s *Store) WithOperator(ctx context.Context, fn func(tx pgx.Tx, q *Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		s.observeError("operator.begin", err)
		return err
	}
	// rollbackTx releases the connection even if fn panics or the caller's
	// context is already cancelled.
	defer s.rollbackTx(tx)

	if _, err := tx.Exec(ctx, `SELECT set_config('app.is_operator', 'on', true)`); err != nil {
		s.observeError("operator", err)
		return fmt.Errorf("set operator context: %w", err)
	}
	if err := fn(tx, storegen.New(tx)); err != nil {
		s.observeError("operator", err)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		s.observeError("operator.commit", err)
		return err
	}
	return nil
}
