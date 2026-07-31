// Package service implements the Phase 0 business operations. Every state
// change writes its outbox event and audit record in the same database
// transaction (transactional outbox).
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/keys"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrValidation    = errors.New("validation error")
	ErrUsageConflict = errors.New("usage conflict")
	// ErrUsageAlreadyInvoiced is returned when a reversal is attempted on
	// usage that has already been included in a finalized invoice. The
	// caller must issue a credit note instead (Testing #6 post-invoice).
	ErrUsageAlreadyInvoiced = errors.New("usage already invoiced")
	// ErrQuotaExceeded is returned when a hard quota reservation would
	// exceed the configured limit (spec Section 11.2, Testing #20).
	ErrQuotaExceeded = errors.New("quota exceeded")
)

type Service struct {
	store      *store.Store
	baseDomain string
	log        *slog.Logger
	// adapter is the billing engine adapter, used for invoice sync. When
	// nil (no billing engine configured), invoice sync is a no-op.
	adapter billing.Adapter
	// encryptor is used for PSP credential encryption. When nil, PSP
	// credential creation returns an error.
	encryptor *crypto.Encryptor
	// zitadelMgmt is the ZITADEL Management API client for Hosted Auth
	// project management. When nil, Hosted Auth setup is unavailable.
	zitadelMgmt *zitadel.ManagementClient
	// zitadelIssuer is the ZITADEL issuer URL (for OIDC discovery).
	zitadelIssuer string
	// usageLateWindow is how far back usage timestamps may lag (config
	// USAGE_LATE_WINDOW_HOURS, default 168h).
	usageLateWindow time.Duration
	// usageFutureSkew is how far in the future a usage timestamp may be
	// (clock skew allowance, default 5min).
	usageFutureSkew time.Duration
	// urlValidator validates webhook endpoint URLs for SSRF safety. Tests
	// may override it to permit loopback (httptest servers).
	urlValidator func(string) error
	// dnsResolver resolves DNS TXT records for custom domain verification.
	// Tests may override it to simulate DNS ownership proof without real
	// DNS records. Production must keep the default net.LookupTXT.
	dnsResolver func(ctx context.Context, name string) ([]string, error)
}

// Option customizes the service (billing core settings).
type Option func(*Service)

// WithUsageLateWindow sets the maximum age of accepted usage timestamps.
func WithUsageLateWindow(d time.Duration) Option {
	return func(s *Service) { s.usageLateWindow = d }
}

// WithUsageFutureSkew sets the accepted future clock skew for usage
// timestamps.
func WithUsageFutureSkew(d time.Duration) Option {
	return func(s *Service) { s.usageFutureSkew = d }
}

// WithBillingAdapter injects the billing adapter used by invoice sync.
// main.go passes the same adapter that feeds the outbox relay.
func WithBillingAdapter(a billing.Adapter) Option {
	return func(s *Service) { s.adapter = a }
}

// WithLogger injects a structured logger. Defaults to slog.Default().
func WithLogger(log *slog.Logger) Option {
	return func(s *Service) { s.log = log }
}

// WithURLValidator overrides the webhook URL validator (SSRF check). Tests
// use this to permit loopback addresses (httptest servers bind to
// 127.0.0.1). Production must keep the default webhook.ValidateURL.
func WithURLValidator(fn func(string) error) Option {
	return func(s *Service) { s.urlValidator = fn }
}

// WithDNSResolver overrides the DNS TXT resolver for custom domain
// verification. Tests use this to simulate DNS ownership proof without
// real DNS records. Production must keep the default net.LookupTXT.
func WithDNSResolver(fn func(ctx context.Context, name string) ([]string, error)) Option {
	return func(s *Service) { s.dnsResolver = fn }
}

func New(st *store.Store, platformBaseDomain string, opts ...Option) *Service {
	s := &Service{
		store:           st,
		baseDomain:      platformBaseDomain,
		log:             slog.Default(),
		adapter:         billing.NewNoop(nil),
		usageLateWindow: 168 * time.Hour,
		usageFutureSkew: 5 * time.Minute,
		urlValidator:    webhook.ValidateURL,
		dnsResolver: func(ctx context.Context, name string) ([]string, error) {
			return net.LookupTXT(name)
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) issuer(slug, envKind string) string {
	return fmt.Sprintf("https://%s.%s.%s", slug, envKind, s.baseDomain)
}

// ---- operator operations ----

type CreatedProvider struct {
	Provider     storegen.Provider
	Environments []storegen.Environment
	TestAPIKey   string // plaintext, returned exactly once
}

// CreateProvider registers a provider: it becomes TEST_ACTIVE, gets a test
// environment with a stable issuer, an initial test API key and a
// platform-domain commerce account — all in one transaction.
func (s *Service) CreateProvider(ctx context.Context, slug, name, homeRegionCode string) (*CreatedProvider, error) {
	if slug == "" || name == "" || homeRegionCode == "" {
		return nil, fmt.Errorf("%w: slug, name and home_region_code are required", ErrValidation)
	}
	var out CreatedProvider
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		region, err := q.GetRegionByCode(ctx, homeRegionCode)
		if err != nil {
			return mapErr(err, "region %q", homeRegionCode)
		}
		var cellID uuid.NullUUID
		if cell, err := q.GetSharedCellByRegion(ctx, region.ID); err == nil {
			cellID = uuid.NullUUID{UUID: cell.ID, Valid: true}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		provider, err := q.CreateProvider(ctx, storegen.CreateProviderParams{
			Slug:           slug,
			Name:           name,
			HomeRegionID:   region.ID,
			CellID:         cellID,
			LifecycleState: string(domain.StateTestActive),
		})
		if err != nil {
			return mapErr(err, "provider slug %q", slug)
		}
		env, err := q.CreateEnvironment(ctx, storegen.CreateEnvironmentParams{
			ProviderID: provider.ID,
			Kind:       domain.EnvKindTest,
			Issuer:     s.issuer(slug, domain.EnvKindTest),
		})
		if err != nil {
			return err
		}
		plaintext, err := createCredentialTx(ctx, q, provider.ID, env.ID, env.Kind, "initial-test-key", domain.AllScopes, nil)
		if err != nil {
			return err
		}
		if _, err := q.InsertCommerceAccount(ctx, storegen.InsertCommerceAccountParams{
			Domain:        domain.CommerceDomainPlatform,
			ProviderID:    uuid.NullUUID{UUID: provider.ID, Valid: true},
			EnvironmentID: uuid.NullUUID{},
			DisplayName:   slug,
		}); err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, provider.ID, env.ID, "provider", provider.ID.String(), "provider.created", map[string]any{
			"provider_id": provider.ID.String(), "slug": slug, "lifecycle_state": string(domain.StateTestActive),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: provider.ID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", "operator", "provider.create", "provider", provider.ID.String(), nil); err != nil {
			return err
		}
		out.Provider = provider
		out.Environments = []storegen.Environment{env}
		out.TestAPIKey = plaintext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type ProviderDetail struct {
	Provider     storegen.Provider
	Environments []storegen.Environment
}

func (s *Service) GetProvider(ctx context.Context, id uuid.UUID) (*ProviderDetail, error) {
	var out ProviderDetail
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		p, err := q.GetProviderByID(ctx, id)
		if err != nil {
			return mapErr(err, "provider %s", id)
		}
		envs, err := q.ListEnvironmentsByProvider(ctx, id)
		if err != nil {
			return err
		}
		out.Provider = p
		out.Environments = envs
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ListProviders(ctx context.Context) ([]storegen.Provider, error) {
	var out []storegen.Provider
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ps, err := q.ListProviders(ctx)
		out = ps
		return err
	})
	return out, err
}

func (s *Service) ListRegions(ctx context.Context) ([]storegen.Region, error) {
	var out []storegen.Region
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		rs, err := q.ListRegions(ctx)
		out = rs
		return err
	})
	return out, err
}

func (s *Service) ListCells(ctx context.Context) ([]storegen.Cell, error) {
	var out []storegen.Cell
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		cs, err := q.ListCells(ctx)
		out = cs
		return err
	})
	return out, err
}

type LifecycleResult struct {
	Provider        storegen.Provider
	LiveAPIKey      string                // plaintext, set only when this transition created the live environment
	LiveEnvironment *storegen.Environment // set only when this transition created the live environment
}

// TransitionLifecycle moves a provider through the lifecycle state machine.
// Transitioning to LIVE_ACTIVE (only valid from LIVE_REVIEW) creates the
// live environment with its stable issuer and an initial live API key.
func (s *Service) TransitionLifecycle(ctx context.Context, providerID uuid.UUID, to domain.LifecycleState) (*LifecycleResult, error) {
	var out LifecycleResult
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		p, err := q.GetProviderByID(ctx, providerID)
		if err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		next, err := domain.Transition(domain.LifecycleState(p.LifecycleState), to)
		if err != nil {
			return err
		}
		p, err = q.UpdateProviderLifecycle(ctx, storegen.UpdateProviderLifecycleParams{
			ID:             providerID,
			LifecycleState: string(next),
		})
		if err != nil {
			return err
		}
		var auditEnv uuid.NullUUID
		if next == domain.StateLiveActive {
			env, err := q.CreateEnvironment(ctx, storegen.CreateEnvironmentParams{
				ProviderID: providerID,
				Kind:       domain.EnvKindLive,
				Issuer:     s.issuer(p.Slug, domain.EnvKindLive),
			})
			if err != nil {
				return mapErr(err, "live environment")
			}
			plaintext, err := createCredentialTx(ctx, q, providerID, env.ID, env.Kind, "initial-live-key", domain.AllScopes, nil)
			if err != nil {
				return err
			}
			out.LiveAPIKey = plaintext
			out.LiveEnvironment = &env
			auditEnv = uuid.NullUUID{UUID: env.ID, Valid: true}
		}
		if err := emitOutboxTx(ctx, q, providerID, envOrTest(ctx, q, providerID, auditEnv), "provider", providerID.String(), "provider.lifecycle_changed", map[string]any{
			"provider_id": providerID.String(), "to": string(next),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, auditEnv,
			"operator", "operator", "provider.lifecycle", "provider", providerID.String(),
			map[string]any{"to": string(next)}); err != nil {
			return err
		}
		out.Provider = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// envOrTest resolves the environment the outbox event is scoped to: the
// given one, or the provider's test environment as fallback.
func envOrTest(ctx context.Context, q *store.Queries, providerID uuid.UUID, env uuid.NullUUID) uuid.UUID {
	if env.Valid {
		return env.UUID
	}
	envs, err := q.ListEnvironmentsByProvider(ctx, providerID)
	if err == nil {
		for _, e := range envs {
			if e.Kind == domain.EnvKindTest {
				return e.ID
			}
		}
		if len(envs) > 0 {
			return envs[0].ID
		}
	}
	return uuid.Nil
}

// ---- provider (tenant) operations ----

type CreatedCredential struct {
	Credential storegen.Credential
	APIKey     string // plaintext, returned exactly once
}

// CreateCredential issues a new API key inside the caller's tenant context.
// Rotation = create a new key, then revoke the old one.
func (s *Service) CreateCredential(ctx context.Context, tc tenant.Ctx, name string, scopes []string, expiresAt *time.Time) (*CreatedCredential, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("%w: at least one scope is required", ErrValidation)
	}
	for _, sc := range scopes {
		if !domain.ValidScope(sc) {
			return nil, fmt.Errorf("%w: unknown scope %q", ErrValidation, sc)
		}
		// Scope attenuation: a credential can only mint keys within its
		// own scopes, so a restricted key cannot escalate itself.
		if !tc.HasScope(sc) {
			return nil, fmt.Errorf("%w: scope %q exceeds the caller's scopes", ErrValidation, sc)
		}
	}
	var out CreatedCredential
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		plaintext, err := keys.Generate(tc.EnvironmentKind)
		if err != nil {
			return err
		}
		cred, err := q.CreateCredential(ctx, storegen.CreateCredentialParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Name:          name,
			KeyPrefix:     keys.Prefix(plaintext),
			KeyHash:       keys.Hash(plaintext),
			Scopes:        scopes,
			ExpiresAt:     expiresAt,
		})
		if err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "credential", cred.ID.String(), "credential.created", map[string]any{
			"credential_id": cred.ID.String(), "name": name, "scopes": scopes,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "credential.create", "credential", cred.ID.String(), nil); err != nil {
			return err
		}
		out.Credential = cred
		out.APIKey = plaintext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeCredential revokes a credential of the caller's own environment.
// Revocation is immediate.
func (s *Service) RevokeCredential(ctx context.Context, tc tenant.Ctx, credentialID uuid.UUID) (*storegen.Credential, error) {
	var out storegen.Credential
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		cred, err := q.GetCredentialByID(ctx, credentialID)
		if err != nil {
			return mapErr(err, "credential %s", credentialID)
		}
		if cred.RevokedAt != nil {
			return fmt.Errorf("%w: credential already revoked", ErrConflict)
		}
		cred, err = q.RevokeCredential(ctx, credentialID)
		if err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "credential", cred.ID.String(), "credential.revoked", map[string]any{
			"credential_id": cred.ID.String(),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "credential.revoke", "credential", cred.ID.String(), nil); err != nil {
			return err
		}
		out = cred
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ListCredentials(ctx context.Context, tc tenant.Ctx) ([]storegen.Credential, error) {
	var out []storegen.Credential
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		cs, err := q.ListCredentialsByEnvironment(ctx, storegen.ListCredentialsByEnvironmentParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = cs
		return err
	})
	return out, err
}

func (s *Service) ListAuditEvents(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.AuditEvent, error) {
	var out []storegen.AuditEvent
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		evs, err := q.ListAuditEventsByProvider(ctx, storegen.ListAuditEventsByProviderParams{
			ProviderID: tc.ProviderNullUUID(), Limit: limit,
		})
		out = evs
		return err
	})
	return out, err
}

func (s *Service) ListOutboxEvents(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.OutboxEvent, error) {
	var out []storegen.OutboxEvent
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		evs, err := q.ListOutboxEventsByTenant(ctx, storegen.ListOutboxEventsByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		out = evs
		return err
	})
	return out, err
}

// ---- shared tx helpers ----

// createCredentialTx generates a key and inserts its credential row inside
// the current transaction. Returns the plaintext key (shown once).
func createCredentialTx(ctx context.Context, q *store.Queries, providerID, envID uuid.UUID, envKind, name string, scopes []string, expiresAt *time.Time) (string, error) {
	plaintext, err := keys.Generate(envKind)
	if err != nil {
		return "", err
	}
	if _, err := q.CreateCredential(ctx, storegen.CreateCredentialParams{
		ProviderID:    providerID,
		EnvironmentID: envID,
		Name:          name,
		KeyPrefix:     keys.Prefix(plaintext),
		KeyHash:       keys.Hash(plaintext),
		Scopes:        scopes,
		ExpiresAt:     expiresAt,
	}); err != nil {
		return "", err
	}
	return plaintext, nil
}

// emitOutboxTx appends an outbox event in the current transaction.
// payload_hash is the sha256 of the canonical payload. Each business fact
// emitted here gets a fresh transaction_id; the (provider_id,
// environment_id, transaction_id) unique constraint is the idempotency
// seam for callers that supply their own stable IDs (e.g. Phase 1
// metering retries), where a conflicting payload_hash must be rejected.
func emitOutboxTx(ctx context.Context, q *store.Queries, providerID, envID uuid.UUID, aggregateType, aggregateID, eventType string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	_, err = q.InsertOutboxEvent(ctx, storegen.InsertOutboxEventParams{
		ProviderID:    providerID,
		EnvironmentID: envID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       raw,
		PayloadHash:   hex.EncodeToString(sum[:]),
		TransactionID: uuid.NewString(),
	})
	return err
}

// emitOutboxWithTxIDTx appends an outbox event carrying a caller-supplied
// stable transaction_id (the usage idempotency seam): the outbox row shares
// the usage event's transaction_id, and a unique-conflict retry is a no-op
// (the event is already durable from the first attempt).
func emitOutboxWithTxIDTx(ctx context.Context, q *store.Queries, providerID, envID uuid.UUID, aggregateType, aggregateID, eventType, txID string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	return q.InsertOutboxEventIdempotent(ctx, storegen.InsertOutboxEventIdempotentParams{
		ProviderID:    providerID,
		EnvironmentID: envID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       raw,
		PayloadHash:   hex.EncodeToString(sum[:]),
		TransactionID: txID,
	})
}

func insertAuditTx(ctx context.Context, q *store.Queries, providerID, envID uuid.NullUUID, actorType, actorID, action, targetType, targetID string, metadata map[string]any) error {
	var raw []byte
	var err error
	if metadata != nil {
		raw, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	} else {
		raw = []byte(`{}`)
	}
	_, err = q.InsertAuditEvent(ctx, storegen.InsertAuditEventParams{
		ProviderID:    providerID,
		EnvironmentID: envID,
		ActorType:     actorType,
		ActorID:       actorID,
		Action:        action,
		TargetType:    pgtype.Text{String: targetType, Valid: targetType != ""},
		TargetID:      pgtype.Text{String: targetID, Valid: targetID != ""},
		Metadata:      raw,
	})
	return err
}

// mapErr translates pgx errors into domain errors.
func mapErr(err error, whatFmt string, args ...any) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, fmt.Sprintf(whatFmt, args...))
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(whatFmt, args...))
	}
	return err
}

// checkTenantOwnership verifies that an entity belongs to the caller's
// tenant. Returns ErrNotFound if the provider_id or environment_id does
// not match. This eliminates the duplicated ownership-check pattern
// across 10+ service methods.
func checkTenantOwnership(providerID, environmentID uuid.UUID, tc tenant.Ctx, entity string, id any) error {
	if providerID != tc.ProviderID || environmentID != tc.EnvironmentID {
		return fmt.Errorf("%w: %s %v", ErrNotFound, entity, id)
	}
	return nil
}

// ErrCellDraining is returned when a billing operation is attempted
// on a provider whose cell is in 'draining' status (write fencing
// during failover/migration, spec Section 14).
var ErrCellDraining = fmt.Errorf("cell is draining: writes are suspended during failover/migration")

// requireStatus checks that the current status matches one of the allowed
// statuses. Returns ErrConflict if not. This eliminates the duplicated
// `if status != X` pattern across failover and migration methods.
func requireStatus(current string, allowed []string, entity string, id any) error {
	for _, s := range allowed {
		if current == s {
			return nil
		}
	}
	return fmt.Errorf("%w: %s %v must be %v (status=%s)", ErrConflict, entity, id, allowed, current)
}

// ExpirySweeper is a generic background sweeper that periodically calls
// a sweep function to expire past-due resources. Used by support session
// expiry and quota reservation expiry (eliminates duplicated sweeper structs).
type ExpirySweeper struct {
	name     string
	sweep    func(context.Context) (int64, error)
	interval time.Duration
	log      *slog.Logger
}

// NewExpirySweeper creates a generic expiry sweeper.
func NewExpirySweeper(name string, sweep func(context.Context) (int64, error), interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return &ExpirySweeper{name: name, sweep: sweep, interval: interval, log: log}
}

// Run blocks until ctx is cancelled, periodically calling the sweep function.
func (sw *ExpirySweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(sw.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := sw.sweep(ctx)
			if err != nil {
				sw.log.Error(sw.name+" sweep failed", "error", err)
				continue
			}
			if n > 0 {
				sw.log.Info(sw.name+" expired", "count", n)
			}
		}
	}
}

// RecordTenantOverrideAttempt audits a rejected attempt to override the
// credential-derived tenant context via request body or query parameters.
func (s *Service) RecordTenantOverrideAttempt(ctx context.Context, tc tenant.Ctx, field, presented string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		return insertAuditTx(ctx, q,
			tc.ProviderNullUUID(),
			tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "tenant.context_override_attempt",
			"credential", tc.CredentialID.String(),
			map[string]any{"field": field, "presented": presented,
				"expected_provider_id": tc.ProviderID.String(), "expected_environment_id": tc.EnvironmentID.String()})
	})
}
