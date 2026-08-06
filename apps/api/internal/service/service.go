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
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/metrics"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/reqid"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
	ErrForbidden  = errors.New("forbidden")
	ErrValidation = errors.New("validation error")
	// ErrUsageConflict is returned when a usage adjustment conflicts with
	// an already-established reservation.
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
	// authVaultEncryptor is used for server-side OIDC token vault encryption.
	// Separate from PSP encryptor so vault key rotation is independent.
	authVaultEncryptor *crypto.Encryptor
	// reencryptReporter is notified by the credential re-encryption worker
	// after each per-table batch with (table, reencrypted, errors) counts.
	// Wired by main to credentials_reencrypted_total /
	// credentials_reencrypt_errors_total.
	reencryptReporter func(table string, reencrypted, errors int64)
	// auditArchiver publishes audit hash chain anchors to WORM object
	// storage. When nil, the audit archiver worker is disabled. Wired by
	// main after construction.
	auditArchiver AnchorArchiver
	// auditArchiveReporter is notified by the audit archiver after each
	// sweep with (published, alreadyPublished, listErrors, uploadErrors,
	// markErrors) counts. Wired by main to audit_anchors_published_total /
	// audit_archive_errors_total.
	auditArchiveReporter func(published, alreadyPublished, listErrors, uploadErrors, markErrors int64)
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

// SetReencryptReporter wires a callback that receives per-table counts from
// the credential re-encryption worker after each batch: how many rows were
// re-sealed with the active master key and how many ciphertexts no
// configured key could open (undecryptable, need operator attention). Main
// uses it to feed credentials_reencrypted_total and
// credentials_reencrypt_errors_total. May be set after construction (the
// worker is started later, after the metrics registry exists).
func (s *Service) SetReencryptReporter(fn func(table string, reencrypted, errors int64)) {
	s.reencryptReporter = fn
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
			HomeRegionID:   uuid.NullUUID{UUID: region.ID, Valid: true},
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

// ActivateProvider moves a signup-created REGISTERED provider to TEST_ACTIVE.
// It assigns the operator-chosen home region and shared cell, then provisions
// the test environment with its stable issuer, an initial test API key and
// the platform-domain commerce account — mirroring CreateProvider but on the
// existing record (design baseline §2.1: region and cell are assigned by the
// operator at activation). It is the only path out of REGISTERED: the generic
// lifecycle transition rejects REGISTERED → TEST_ACTIVE.
func (s *Service) ActivateProvider(ctx context.Context, providerID uuid.UUID, homeRegionCode, reason, actor string) (*CreatedProvider, error) {
	if homeRegionCode == "" {
		return nil, fmt.Errorf("%w: home_region_code is required", ErrValidation)
	}
	if actor == "" {
		actor = "operator"
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
		provider, err := q.ActivateProvider(ctx, storegen.ActivateProviderParams{
			ID:             providerID,
			HomeRegionID:   uuid.NullUUID{UUID: region.ID, Valid: true},
			CellID:         cellID,
			LifecycleState: string(domain.StateTestActive),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Either the provider does not exist or it is no longer
				// REGISTERED (already activated or in another state).
				if _, gerr := q.GetProviderByID(ctx, providerID); gerr != nil {
					return mapErr(gerr, "provider %s", providerID)
				}
				return fmt.Errorf("%w: provider %s is not REGISTERED (already activated or in another state)", ErrConflict, providerID)
			}
			return err
		}
		env, err := q.CreateEnvironment(ctx, storegen.CreateEnvironmentParams{
			ProviderID: provider.ID,
			Kind:       domain.EnvKindTest,
			Issuer:     s.issuer(provider.Slug, domain.EnvKindTest),
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
			DisplayName:   provider.Slug,
		}); err != nil {
			return err
		}
		evt := map[string]any{
			"provider_id": provider.ID.String(), "slug": provider.Slug, "lifecycle_state": string(domain.StateTestActive),
		}
		var auditMeta map[string]any
		if reason != "" {
			evt["reason"] = reason
			auditMeta = map[string]any{"reason": reason}
		}
		if err := emitOutboxTx(ctx, q, provider.ID, env.ID, "provider", provider.ID.String(), "provider.activated", evt); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: provider.ID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "provider.activate", "provider", provider.ID.String(), auditMeta); err != nil {
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

// LifecycleTransitionInput describes an intended lifecycle transition and its
// audit context. Actor defaults to "operator" when empty; Reason is recorded in
// the audit trail and the outbound event for traceability.
type LifecycleTransitionInput struct {
	To     domain.LifecycleState
	Reason string
	Actor  string
}

// TransitionLifecycle moves a provider through the lifecycle state machine.
// Transitioning to LIVE_ACTIVE (only valid from LIVE_REVIEW) creates the
// live environment with its stable issuer and an initial live API key.
// The transition is recorded end-to-end: the audit event carries from/to
// plus the operator-provided reason and actor, and the outbox event mirrors
// the same context for downstream consumers.
func (s *Service) TransitionLifecycle(ctx context.Context, providerID uuid.UUID, in LifecycleTransitionInput) (*LifecycleResult, error) {
	var out LifecycleResult
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		p, err := q.GetProviderByID(ctx, providerID)
		if err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		from := domain.LifecycleState(p.LifecycleState)
		next, err := domain.Transition(from, in.To)
		if err != nil {
			return err
		}
		// Go-live gate (architecture §15): a provider may enter LIVE_ACTIVE
		// only after the operator records an approved risk review. This is
		// enforced on the first go-live (LIVE_REVIEW → LIVE_ACTIVE) only;
		// re-activation from RESTRICTED/SUSPENDED keeps the previous approval
		// on record and does not require a fresh review.
		if from == domain.StateLiveReview && next == domain.StateLiveActive {
			if err := requireApprovedReview(ctx, q, providerID); err != nil {
				return err
			}
		}
		actor := in.Actor
		if actor == "" {
			actor = "operator"
		}
		affected, err := q.UpdateProviderLifecycle(ctx, storegen.UpdateProviderLifecycleParams{
			ID:        providerID,
			FromState: string(from),
			ToState:   string(next),
		})
		if err != nil {
			return err
		}
		if affected == 0 {
			// Optimistic guard: another transition committed between our read
			// and this UPDATE, so no row matched the observed source state.
			// Do not record an audit/outbox event for a transition that did
			// not happen; surface the race so the caller can retry.
			current, err := q.GetProviderByID(ctx, providerID)
			if err != nil {
				return mapErr(err, "provider %s", providerID)
			}
			return fmt.Errorf("%w: provider %s observed %s -> %s, current state is %s",
				ErrLifecycleConflict, providerID, from, next, current.LifecycleState)
		}
		p.LifecycleState = string(next)
		var auditEnv uuid.NullUUID
		if next == domain.StateLiveActive {
			// Transitioning to LIVE_ACTIVE creates (or reuses) the live
			// environment. When reactivating from RESTRICTED/SUSPENDED,
			// the live environment already exists — reusing it avoids a
			// unique-constraint violation. A fresh live API key is
			// issued on every activation so the operator gets a usable
			// credential each time.
			env, err := getOrCreateLiveEnvironment(ctx, q, providerID, p.Slug, s.issuer)
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
		evt := map[string]any{"provider_id": providerID.String(), "from": string(from), "to": string(next)}
		meta := map[string]any{"from": string(from), "to": string(next)}
		if in.Reason != "" {
			evt["reason"] = in.Reason
			meta["reason"] = in.Reason
		}
		if err := emitOutboxTx(ctx, q, providerID, envOrTest(ctx, q, providerID, auditEnv), "provider", providerID.String(), "provider.lifecycle_changed", evt); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, auditEnv,
			"operator", actor, "provider.lifecycle", "provider", providerID.String(), meta); err != nil {
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

// getOrCreateLiveEnvironment returns the provider's existing live
// environment, creating one if none exists yet. This supports
// reactivation from RESTRICTED/SUSPENDED back to LIVE_ACTIVE — the
// live environment persists across those transitions and must be
// reused rather than re-created (which would violate the unique
// constraint on (provider_id, kind)).
func getOrCreateLiveEnvironment(ctx context.Context, q *store.Queries, providerID uuid.UUID, slug string, issuerFn func(string, string) string) (storegen.Environment, error) {
	envs, err := q.ListEnvironmentsByProvider(ctx, providerID)
	if err != nil {
		return storegen.Environment{}, err
	}
	for _, e := range envs {
		if e.Kind == domain.EnvKindLive {
			return e, nil
		}
	}
	return q.CreateEnvironment(ctx, storegen.CreateEnvironmentParams{
		ProviderID: providerID,
		Kind:       domain.EnvKindLive,
		Issuer:     issuerFn(slug, domain.EnvKindLive),
	})
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

// AuditQuery carries pagination and filter parameters for audit log queries.
// Empty strings skip the corresponding filter; nil From/To leave the time
// bounds open. A zero Cursor starts from the newest row; otherwise it is the
// primary key of the last row from the previous page.
type AuditQuery struct {
	Limit      int32
	Cursor     int64
	Action     string
	ActorType  string
	ActorID    string
	TargetType string
	TargetID   string
	From, To   *time.Time
}

// auditParams maps an AuditQuery to storegen params. The page limit is
// fetched one row ahead so callers can report whether another page exists.
func (q AuditQuery) auditParams(providerID uuid.NullUUID) storegen.ListAuditEventsFilteredParams {
	return storegen.ListAuditEventsFilteredParams{
		ProviderID: providerID,
		CursorID:   q.Cursor,
		Action:     q.Action,
		ActorType:  q.ActorType,
		ActorID:    q.ActorID,
		TargetType: q.TargetType,
		TargetID:   q.TargetID,
		FromTime:   tsParam(q.From),
		ToTime:     tsParam(q.To),
		PageLimit:  q.Limit + 1, // peek one row to detect the next page
	}
}

// tsParam formats an optional time bound as RFC3339Nano (UTC); an empty
// string leaves the bound open in the query.
func tsParam(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// trimAuditPage returns the visible page and its next-page cursor. The store
// query fetches limit+1 rows to detect a following page; the peek row is
// dropped here and the cursor points at the last returned row, so the next
// query resumes right after it and no row is skipped. A nil cursor means the
// page is the last.
func trimAuditPage(rows []storegen.AuditEvent, limit int32) ([]storegen.AuditEvent, *int64) {
	if limit <= 0 || int32(len(rows)) <= limit {
		return rows, nil
	}
	last := rows[limit-1].ID
	return rows[:limit], &last
}

func (s *Service) ListAuditEvents(ctx context.Context, tc tenant.Ctx, q AuditQuery) ([]storegen.AuditEvent, *int64, error) {
	var out []storegen.AuditEvent
	var next *int64
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, qr *store.Queries) error {
		evs, err := qr.ListAuditEventsFiltered(ctx, q.auditParams(tc.ProviderNullUUID()))
		if err != nil {
			return err
		}
		out, next = trimAuditPage(evs, q.Limit)
		return nil
	})
	return out, next, err
}

// ListProviderAuditEvents returns a provider's audit trail across all
// environments (operator view, newest first). Unknown providers yield
// ErrNotFound so typos surface immediately in the console.
func (s *Service) ListProviderAuditEvents(ctx context.Context, providerID uuid.UUID, q AuditQuery) ([]storegen.AuditEvent, *int64, error) {
	var out []storegen.AuditEvent
	var next *int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
		if _, err := qr.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		evs, err := qr.ListAuditEventsFiltered(ctx, q.auditParams(uuid.NullUUID{UUID: providerID, Valid: true}))
		if err != nil {
			return err
		}
		out, next = trimAuditPage(evs, q.Limit)
		return nil
	})
	return out, next, err
}

// AuditExportQuery carries the filter set for a full audit export. Unlike the
// list endpoints there is no pagination: an export always walks the entire
// matching window. The HTTP layer requires bounded From/To as a guard against
// exporting the whole trail by accident; the service keeps open-bound
// semantics so the SQL filter set stays identical to the list queries.
type AuditExportQuery struct {
	Action     string
	ActorType  string
	ActorID    string
	TargetType string
	TargetID   string
	From, To   *time.Time
}

// auditExportPageSize bounds each keyset page an export walks. A page is
// small enough to keep memory flat but large enough to amortize round trips.
const auditExportPageSize = 1000

// ExportAuditEvents streams the tenant's audit events matching q to emit, in
// newest-first order. emit is invoked once per event; returning an error
// aborts the export early (e.g. a client that disconnected mid-download).
func (s *Service) ExportAuditEvents(ctx context.Context, tc tenant.Ctx, q AuditExportQuery, emit func(storegen.AuditEvent) error) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, qr *store.Queries) error {
		return streamAuditEvents(ctx, qr, tc.ProviderNullUUID(), q, emit)
	})
}

// ExportProviderAuditEvents streams a provider's audit trail across all
// environments (operator view, newest first). Unknown providers yield
// ErrNotFound so typos surface immediately in the console.
func (s *Service) ExportProviderAuditEvents(ctx context.Context, providerID uuid.UUID, q AuditExportQuery, emit func(storegen.AuditEvent) error) error {
	return s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
		if _, err := qr.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		return streamAuditEvents(ctx, qr, uuid.NullUUID{UUID: providerID, Valid: true}, q, emit)
	})
}

// auditPurgeBatchSize bounds each DELETE batch issued by the audit retention
// sweeper. Batches keep row locks (and the trigger disable window inside
// purge_audit_events) short while still making progress on a large backlog.
const auditPurgeBatchSize = 50000

// PurgeExpiredAuditEvents permanently deletes audit events older than cutoff
// and returns the total number of events deleted. The append-only invariant
// is preserved because deletion goes through the operator-only
// purge_audit_events function (migration 0030); each batch runs in its own
// short transaction, and batchSize bounds the work per transaction.
func (s *Service) PurgeExpiredAuditEvents(ctx context.Context, cutoff time.Time, batchSize int64) (int64, error) {
	var total int64
	for {
		var deleted int64
		err := s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
			var err error
			deleted, err = qr.PurgeExpiredAuditEvents(ctx, storegen.PurgeExpiredAuditEventsParams{
				Cutoff:  cutoff,
				MaxRows: batchSize,
			})
			return err
		})
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < batchSize {
			return total, nil
		}
	}
}

// NewAuditRetentionSweeper creates a sweeper that purges audit events past
// the retention window. Audit events are compliance evidence, so the caller
// only starts this worker when retention is explicitly configured
// (AUDIT_RETENTION_DAYS, default 0 = disabled).
func NewAuditRetentionSweeper(svc *Service, retentionDays int, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("audit_retention", func(ctx context.Context) (int64, error) {
		cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		return svc.PurgeExpiredAuditEvents(ctx, cutoff, auditPurgeBatchSize)
	}, interval, log)
}

// AuditChainState describes the current tamper-evident chain (migration 0031)
// and its most recent anchor checkpoint. Zero-valued anchor fields mean no
// anchor has been created yet.
type AuditChainState struct {
	TotalEvents       int64
	TailHash          *string
	TailEventID       *int64
	LastAnchorID      int64
	LastAnchorEventID int64
	LastAnchorHash    string
	LastAnchorAt      time.Time
}

// AuditChainState reports the tamper-evident chain status. Operator-only.
func (s *Service) AuditChainState(ctx context.Context) (*AuditChainState, error) {
	var out AuditChainState
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
		row, err := qr.AuditChainState(ctx)
		if err != nil {
			return err
		}
		out = AuditChainState{
			TotalEvents: row.TotalEvents,
			TailHash:    textPtr(row.TailHash),
			TailEventID: int64Ptr(row.TailEventID),
		}
		anchor, err := qr.LatestAuditAnchor(ctx)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // no anchor created yet
			}
			return err
		}
		out.LastAnchorID = anchor.ID
		out.LastAnchorEventID = anchor.TailEventID
		out.LastAnchorHash = anchor.TailHash
		out.LastAnchorAt = anchor.CreatedAt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AuditChainVerifyResult reports the outcome of a verification run over the
// tamper-evident audit chain. BrokenAt is nil when the segment is intact.
type AuditChainVerifyResult struct {
	OK            bool
	VerifiedFrom  int64
	VerifiedTo    int64
	VerifiedCount int64
	BrokenAt      *int64
	Reason        string
}

// VerifyAuditChain verifies the hash chain over the event id range (fromID,
// toID]. fromID 0 (or below the pruned head) starts at the first surviving
// row; toID 0 means the current tail. The first broken event id, if any, is
// returned so callers can bound the damage window. Operator-only.
func (s *Service) VerifyAuditChain(ctx context.Context, fromID, toID int64) (*AuditChainVerifyResult, error) {
	if fromID < 0 || toID < 0 {
		return nil, fmt.Errorf("%w: from_id and to_id must be >= 0", ErrValidation)
	}
	var out AuditChainVerifyResult
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
		row, err := qr.VerifyAuditChain(ctx, storegen.VerifyAuditChainParams{
			FromID: fromID,
			ToID:   toID,
		})
		if err != nil {
			return err
		}
		out = AuditChainVerifyResult{
			OK:            row.Ok,
			VerifiedFrom:  row.VerifiedFrom,
			VerifiedTo:    row.VerifiedTo,
			VerifiedCount: row.VerifiedCount,
			Reason:        row.Reason,
		}
		if row.BrokenAt > 0 {
			out.BrokenAt = &row.BrokenAt
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AuditChainAnchor is a checkpoint of (tail_event_id, tail_hash) created by an
// operator. Anchors bound the incremental verification window and are the rows
// external anchoring (WORM object storage, direction backlog) will publish
// outside the database to make the chain tamper-proof rather than merely
// tamper-evident.
type AuditChainAnchor struct {
	AnchorID      int64
	TailEventID   int64
	TailHash      string
	EventsCovered int64
}

// AnchorAuditChain records a checkpoint at the current chain tail. The
// operator label names who/what created the anchor ("manual", an OIDC subject,
// or "sweeper" for the automatic worker). Operator-only.
func (s *Service) AnchorAuditChain(ctx context.Context, operator string) (*AuditChainAnchor, error) {
	var out AuditChainAnchor
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
		row, err := qr.AnchorAuditChain(ctx, operator)
		if err != nil {
			return err
		}
		out = AuditChainAnchor{
			AnchorID:      row.AnchorID,
			TailEventID:   row.TailEventID,
			TailHash:      row.TailHash,
			EventsCovered: row.EventsCovered,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// NewAuditChainAnchorSweeper periodically creates anchor checkpoints on the
// tamper-evident audit chain. Anchoring only appends one row per tick (it never
// mutates audit data), so the worker is safe to run whenever
// AUDIT_CHAIN_ANCHOR_INTERVAL > 0; the swept count is the number of events
// covered since the previous anchor.
func NewAuditChainAnchorSweeper(svc *Service, operator string, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("audit_chain_anchor", func(ctx context.Context) (int64, error) {
		anchor, err := svc.AnchorAuditChain(ctx, operator)
		if err != nil {
			return 0, err
		}
		return anchor.EventsCovered, nil
	}, interval, log)
}

// textPtr / int64Ptr / timePtr convert nullable pgtype scalars into Go
// pointers so JSON output renders absent values as null instead of ""/0.
func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func int64Ptr(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

// streamAuditEvents walks the filtered trail in keyset pages, invoking emit
// once per row so a large export never buffers entirely in memory. Rows come
// back newest-first, matching the list endpoints.
func streamAuditEvents(ctx context.Context, qr *store.Queries, providerID uuid.NullUUID, q AuditExportQuery, emit func(storegen.AuditEvent) error) error {
	var cursor int64
	for {
		params := storegen.ListAuditEventsFilteredParams{
			ProviderID: providerID,
			CursorID:   cursor,
			Action:     q.Action,
			ActorType:  q.ActorType,
			ActorID:    q.ActorID,
			TargetType: q.TargetType,
			TargetID:   q.TargetID,
			FromTime:   tsParam(q.From),
			ToTime:     tsParam(q.To),
			PageLimit:  auditExportPageSize + 1, // peek one row to detect the next page
		}
		rows, err := qr.ListAuditEventsFiltered(ctx, params)
		if err != nil {
			return err
		}
		page, next := trimAuditPage(rows, auditExportPageSize)
		for _, ev := range page {
			if err := emit(ev); err != nil {
				return err
			}
		}
		if next == nil {
			return nil
		}
		cursor = *next
	}
}

// AuditStatsQuery carries the filter and bucketing parameters for the audit
// dashboard aggregations. Unlike AuditQuery it has no pagination: it
// aggregates the whole filtered window. The HTTP layer requires bounded
// From/To as a guard against accidental full-table aggregation; the service
// keeps the same open-bound semantics as the list queries so the SQL filter
// set stays identical. Granularity is one of hour|day|week (UTC-aligned).
type AuditStatsQuery struct {
	Action      string
	ActorType   string
	ActorID     string
	TargetType  string
	TargetID    string
	From, To    *time.Time
	Granularity string
}

// AuditCount is one (key, count) pair of a grouped aggregation.
type AuditCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// AuditSeriesPoint is one bucket of the dashboard time series.
type AuditSeriesPoint struct {
	Bucket time.Time `json:"bucket"`
	Count  int64     `json:"count"`
}

// AuditStats is the dashboard response: a headline total, the two most useful
// breakdowns (by action and by actor type), and a contiguous, zero-filled
// time series.
type AuditStats struct {
	Total       int64              `json:"total"`
	ByAction    []AuditCount       `json:"by_action"`
	ByActorType []AuditCount       `json:"by_actor_type"`
	Series      []AuditSeriesPoint `json:"series"`
}

// AuditEventStats aggregates a provider's audit events (tenant view). All
// four queries run inside one transaction so the response reflects a single
// database snapshot.
func (s *Service) AuditEventStats(ctx context.Context, tc tenant.Ctx, q AuditStatsQuery) (*AuditStats, error) {
	var out AuditStats
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, qr *store.Queries) error {
		st, err := auditStats(ctx, qr, tc.ProviderNullUUID(), q)
		if err != nil {
			return err
		}
		out = st
		return nil
	})
	return &out, err
}

// ProviderAuditEventStats aggregates a provider's audit events across all
// environments (operator view). Unknown providers yield ErrNotFound so typos
// surface immediately in the console.
func (s *Service) ProviderAuditEventStats(ctx context.Context, providerID uuid.UUID, q AuditStatsQuery) (*AuditStats, error) {
	var out AuditStats
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, qr *store.Queries) error {
		if _, err := qr.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		st, err := auditStats(ctx, qr, uuid.NullUUID{UUID: providerID, Valid: true}, q)
		if err != nil {
			return err
		}
		out = st
		return nil
	})
	return &out, err
}

// auditStats runs the four dashboard aggregations with a shared filter set.
func auditStats(ctx context.Context, qr *store.Queries, providerID uuid.NullUUID, q AuditStatsQuery) (AuditStats, error) {
	var out AuditStats
	total, err := qr.CountAuditEventsFiltered(ctx, storegen.CountAuditEventsFilteredParams{
		ProviderID: providerID,
		Action:     q.Action,
		ActorType:  q.ActorType,
		ActorID:    q.ActorID,
		TargetType: q.TargetType,
		TargetID:   q.TargetID,
		FromTime:   tsParam(q.From),
		ToTime:     tsParam(q.To),
	})
	if err != nil {
		return out, err
	}
	actions, err := qr.AuditEventActionCounts(ctx, storegen.AuditEventActionCountsParams{
		ProviderID: providerID,
		Action:     q.Action,
		ActorType:  q.ActorType,
		ActorID:    q.ActorID,
		TargetType: q.TargetType,
		TargetID:   q.TargetID,
		FromTime:   tsParam(q.From),
		ToTime:     tsParam(q.To),
	})
	if err != nil {
		return out, err
	}
	actors, err := qr.AuditEventActorTypeCounts(ctx, storegen.AuditEventActorTypeCountsParams{
		ProviderID: providerID,
		Action:     q.Action,
		ActorType:  q.ActorType,
		ActorID:    q.ActorID,
		TargetType: q.TargetType,
		TargetID:   q.TargetID,
		FromTime:   tsParam(q.From),
		ToTime:     tsParam(q.To),
	})
	if err != nil {
		return out, err
	}
	series, err := qr.AuditEventTimeSeries(ctx, storegen.AuditEventTimeSeriesParams{
		ProviderID:  providerID,
		Granularity: q.Granularity,
		Action:      q.Action,
		ActorType:   q.ActorType,
		ActorID:     q.ActorID,
		TargetType:  q.TargetType,
		TargetID:    q.TargetID,
		FromTime:    tsParam(q.From),
		ToTime:      tsParam(q.To),
	})
	if err != nil {
		return out, err
	}
	out.Total = total
	out.ByAction = make([]AuditCount, 0, len(actions))
	for _, a := range actions {
		out.ByAction = append(out.ByAction, AuditCount{Key: a.Action, Count: a.EventCount})
	}
	out.ByActorType = make([]AuditCount, 0, len(actors))
	for _, a := range actors {
		out.ByActorType = append(out.ByActorType, AuditCount{Key: a.ActorType, Count: a.EventCount})
	}
	out.Series = fillSeries(series, q)
	return out, nil
}

// fillSeries zero-fills the time series between From and To so the dashboard
// renders a contiguous axis. Buckets align with Postgres date_trunc on UTC:
// hour on the hour, day at UTC midnight, week on Mondays. When the bounds are
// absent the DB rows are returned as-is.
func fillSeries(rows []storegen.AuditEventTimeSeriesRow, q AuditStatsQuery) []AuditSeriesPoint {
	if q.From == nil || q.To == nil {
		out := make([]AuditSeriesPoint, 0, len(rows))
		for _, r := range rows {
			out = append(out, AuditSeriesPoint{Bucket: r.Bucket, Count: r.EventCount})
		}
		return out
	}
	step := bucketStep(q.Granularity)
	out := make([]AuditSeriesPoint, 0, 4096)
	idx := 0
	for b := bucketTrunc(*q.From, q.Granularity); !b.After(bucketTrunc(*q.To, q.Granularity)); b = b.Add(step) {
		c := int64(0)
		if idx < len(rows) && rows[idx].Bucket.Equal(b) {
			c = rows[idx].EventCount
			idx++
		}
		out = append(out, AuditSeriesPoint{Bucket: b, Count: c})
	}
	return out
}

func bucketStep(granularity string) time.Duration {
	switch granularity {
	case "hour":
		return time.Hour
	case "week":
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

// bucketTrunc truncates a timestamp to the UTC-aligned bucket boundary used by
// Postgres date_trunc: hour on the hour, day at UTC midnight, week on Monday.
func bucketTrunc(t time.Time, granularity string) time.Time {
	u := t.UTC()
	switch granularity {
	case "hour":
		return u.Truncate(time.Hour)
	case "week":
		u = u.AddDate(0, 0, -int((int(u.Weekday())+6)%7)) // back to Monday
	}
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ListProviderCredentials returns every API key issued to a provider across
// all environments (operator view). The raw key hash is never exposed; keys
// are identified by their key_prefix only. Unknown providers yield
// ErrNotFound so typos surface immediately in the console.
func (s *Service) ListProviderCredentials(ctx context.Context, providerID uuid.UUID) ([]storegen.ListCredentialsByProviderRow, error) {
	var out []storegen.ListCredentialsByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		cs, err := q.ListCredentialsByProvider(ctx, providerID)
		out = cs
		return err
	})
	return out, err
}

// RevokeProviderCredential revokes a provider API key from the operator
// console. Revocation is immediate: the authentication middleware rejects the
// key regardless of any in-flight token. An audit event with
// actor_type=operator records the action on the provider's trail.
func (s *Service) RevokeProviderCredential(ctx context.Context, providerID, credentialID uuid.UUID, revokedBy string) (*storegen.GetCredentialByProviderRow, error) {
	if revokedBy == "" {
		return nil, fmt.Errorf("%w: revoked_by is required", ErrValidation)
	}
	var out storegen.GetCredentialByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		cred, err := q.GetCredentialByProvider(ctx, storegen.GetCredentialByProviderParams{
			ID: credentialID, ProviderID: providerID,
		})
		if err != nil {
			return mapErr(err, "credential %s", credentialID)
		}
		if cred.RevokedAt != nil {
			return fmt.Errorf("%w: credential %s is already revoked", ErrConflict, credentialID)
		}
		if _, err := q.RevokeCredential(ctx, credentialID); err != nil {
			return mapErr(err, "credential %s", credentialID)
		}
		if err := emitOutboxTx(ctx, q, providerID, cred.EnvironmentID, "credential", credentialID.String(), "credential.revoked", map[string]any{
			"credential_id": credentialID.String(),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: cred.EnvironmentID, Valid: true},
			"operator", revokedBy, "credential.revoke", "credential", credentialID.String(),
			map[string]any{"credential_name": cred.Name, "key_prefix": cred.KeyPrefix}); err != nil {
			return err
		}
		after, err := q.GetCredentialByProvider(ctx, storegen.GetCredentialByProviderParams{
			ID: credentialID, ProviderID: providerID,
		})
		if err != nil {
			return err
		}
		out = after
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
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
		RequestID:     pgtype.Text{String: reqid.FromContext(ctx), Valid: reqid.FromContext(ctx) != ""},
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

// ErrProviderNotWritable is returned when a provider attempts a write
// operation while its lifecycle state does not permit writes (REGISTERED,
// SUSPENDED, OFFBOARDING). The middleware rejects these at the gateway;
// this error is the service-layer backstop for paths that bypass it.
var ErrProviderNotWritable = fmt.Errorf("provider is not writable in its current lifecycle state")

// ErrLifecycleConflict is returned when a lifecycle transition loses a race:
// the provider's state changed between the caller's read and the guarded
// UPDATE, so the transition is rejected instead of overwriting a concurrent
// one. Clients should re-read the provider and re-issue the transition.
var ErrLifecycleConflict = fmt.Errorf("provider lifecycle state changed concurrently")

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
	metrics  *metrics.Metrics // optional Prometheus instrumentation
	timeout  time.Duration    // per-run query timeout (0 = disabled)
}

// NewExpirySweeper creates a generic expiry sweeper.
func NewExpirySweeper(name string, sweep func(context.Context) (int64, error), interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return &ExpirySweeper{name: name, sweep: sweep, interval: interval, log: log}
}

// WithMetrics attaches Prometheus instrumentation to the sweeper. Must be
// called before Run starts.
func (sw *ExpirySweeper) WithMetrics(m *metrics.Metrics) *ExpirySweeper {
	sw.metrics = m
	return sw
}

// WithQueryTimeout bounds each sweep run. A hung query then cannot stall the
// sweeper loop past the configured duration. Must be called before Run starts.
func (sw *ExpirySweeper) WithQueryTimeout(d time.Duration) *ExpirySweeper {
	sw.timeout = d
	return sw
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
			runCtx := ctx
			var cancel context.CancelFunc
			if sw.timeout > 0 {
				runCtx, cancel = context.WithTimeout(ctx, sw.timeout)
			}
			n, err := sw.sweep(runCtx)
			if cancel != nil {
				cancel()
			}
			if err != nil {
				sw.log.Error(sw.name+" sweep failed", "error", err)
				continue
			}
			if n > 0 {
				sw.log.Info(sw.name+" expired", "count", n)
			}
			if sw.metrics != nil {
				sw.metrics.SweepDeletedTotal.WithLabelValues(sw.name).Add(float64(n))
			}
		}
	}
}

// OutboxBacklog returns the current outbox_events count grouped by status.
// It powers the Prometheus outbox_events_total gauge (docs/DEPLOYMENT.md
// §5.3 outbox backlog alert). Operator-scoped read; cheap GROUP BY.
func (s *Service) OutboxBacklog(ctx context.Context) (map[string]int64, error) {
	var counts map[string]int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.CountOutboxEventsByStatus(ctx)
		if err != nil {
			return err
		}
		counts = make(map[string]int64, len(rows))
		for _, r := range rows {
			counts[r.Status] = r.Count
		}
		return nil
	})
	return counts, err
}

// QueueOverview is the operator queue board payload: outbox and webhook
// delivery counts by status plus recent outbox events for drill-down.
type QueueOverview struct {
	Outbox            map[string]int64       `json:"outbox"`
	WebhookDeliveries map[string]int64       `json:"webhook_deliveries"`
	RecentOutbox      []storegen.OutboxEvent `json:"recent_outbox"`
}

// QueueOverview returns the operator queue board in one transaction.
func (s *Service) QueueOverview(ctx context.Context, limit int32) (*QueueOverview, error) {
	var out QueueOverview
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		outboxRows, err := q.CountOutboxEventsByStatus(ctx)
		if err != nil {
			return err
		}
		webhookRows, err := q.CountWebhookDeliveriesByStatus(ctx)
		if err != nil {
			return err
		}
		events, err := q.ListOutboxEventsFiltered(ctx, storegen.ListOutboxEventsFilteredParams{
			Status: "",
			Limit:  limit,
		})
		if err != nil {
			return err
		}
		out.Outbox = make(map[string]int64, len(outboxRows))
		for _, row := range outboxRows {
			out.Outbox[row.Status] = row.Count
		}
		out.WebhookDeliveries = make(map[string]int64, len(webhookRows))
		for _, row := range webhookRows {
			out.WebhookDeliveries[row.Status] = row.Count
		}
		out.RecentOutbox = events
		if out.RecentOutbox == nil {
			out.RecentOutbox = []storegen.OutboxEvent{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// WebhookBacklog returns the current webhook_deliveries count grouped by
// status. It powers the Prometheus webhook_deliveries gauge.
func (s *Service) WebhookBacklog(ctx context.Context) (map[string]int64, error) {
	var counts map[string]int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.CountWebhookDeliveriesByStatus(ctx)
		if err != nil {
			return err
		}
		counts = make(map[string]int64, len(rows))
		for _, r := range rows {
			counts[r.Status] = r.Count
		}
		return nil
	})
	return counts, err
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
