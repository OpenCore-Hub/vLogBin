// Package store owns the PostgreSQL connection pool, migrations and the
// tenant-scoped transaction helpers. All tenant queries MUST go through
// WithTenant so that RLS settings are applied; operator queries go through
// WithOperator. The pool must connect as the platform_app role — connecting
// as a superuser would bypass row level security entirely.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/db"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Queries aliases the sqlc-generated query set.
type Queries = storegen.Queries

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Pool exposes the raw pool for infrastructure code (tests, relay).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

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
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.provider_id', $1, true), set_config('app.environment_id', $2, true)`,
		tc.ProviderID.String(), tc.EnvironmentID.String()); err != nil {
		return fmt.Errorf("set tenant context: %w", err)
	}
	if err := fn(tx, storegen.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithOperator runs fn inside a transaction with the operator RLS bypass
// enabled (app.is_operator = 'on'). Only the operator-authenticated code
// paths may call this.
func (s *Store) WithOperator(ctx context.Context, fn func(tx pgx.Tx, q *Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if _, err := tx.Exec(ctx, `SELECT set_config('app.is_operator', 'on', true)`); err != nil {
		return fmt.Errorf("set operator context: %w", err)
	}
	if err := fn(tx, storegen.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
