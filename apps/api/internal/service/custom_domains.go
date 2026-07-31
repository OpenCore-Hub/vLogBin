package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrDomainTaken is returned when a domain is already registered by
// another provider (takeover protection, spec Section 6.2).
var ErrDomainTaken = fmt.Errorf("domain is already registered")

// RegisterCustomDomain registers a domain for DNS ownership verification.
// The platform generates a verification token that the provider must add
// as a TXT record at _vlogbin-verify.{domain}.
func (s *Service) RegisterCustomDomain(ctx context.Context, tc tenant.Ctx, domain string) (*storegen.CustomDomain, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if err := validateDomain(domain); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	token := generateVerificationToken()

	var cd storegen.CustomDomain
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		created, err := q.CreateCustomDomain(ctx, storegen.CreateCustomDomainParams{
			ProviderID:        tc.ProviderID,
			EnvironmentID:     tc.EnvironmentID,
			Domain:            domain,
			VerificationToken: token,
		})
		if err != nil {
			// mapErr translates unique violations to ErrConflict;
			// we further specialize it to ErrDomainTaken for domain takeover.
			mappedErr := mapErr(err, "custom domain %q", domain)
			if errors.Is(mappedErr, ErrConflict) {
				return fmt.Errorf("%w: domain %q is already registered", ErrDomainTaken, domain)
			}
			return mappedErr
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "custom_domain", created.ID.String(), "custom_domain.registered", map[string]any{
			"domain": domain,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "custom_domain.register",
			"custom_domain", created.ID.String(),
			map[string]any{"domain": domain}); err != nil {
			return err
		}
		cd = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

// VerifyCustomDomain checks the DNS TXT record at _vlogbin-verify.{domain}
// for the verification token. If the token matches, the domain is marked
// as verified.
func (s *Service) VerifyCustomDomain(ctx context.Context, tc tenant.Ctx, domainID uuid.UUID) (*storegen.CustomDomain, error) {
	var cd storegen.CustomDomain
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if err := checkTenantOwnership(d.ProviderID, d.EnvironmentID, tc, "custom domain", domainID); err != nil {
			return err
		}
		if d.Status != domain.CustomDomainPending {
			return fmt.Errorf("%w: domain %s is not pending (status=%s)", ErrConflict, domainID, d.Status)
		}

		// DNS TXT lookup at _vlogbin-verify.{domain}.
		txtName := fmt.Sprintf("%s.%s", domain.DNSVerificationPrefix, d.Domain)
		records, err := s.dnsResolver(ctx, txtName)
		if err != nil {
			return fmt.Errorf("%w: DNS lookup failed for %s: %v", ErrValidation, txtName, err)
		}

		// Check if any TXT record matches the verification token.
		matched := false
		for _, r := range records {
			if strings.TrimSpace(r) == d.VerificationToken {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: DNS verification failed: token not found in TXT records for %s", ErrValidation, txtName)
		}

		updated, err := q.VerifyCustomDomain(ctx, domainID)
		if err != nil {
			return mapErr(err, "verify custom domain %s", domainID)
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "custom_domain", domainID.String(), "custom_domain.verified", map[string]any{
			"domain": d.Domain,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "custom_domain.verify",
			"custom_domain", domainID.String(),
			map[string]any{"domain": d.Domain}); err != nil {
			return err
		}
		cd = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

// GetCustomDomain returns a custom domain by ID.
func (s *Service) GetCustomDomain(ctx context.Context, tc tenant.Ctx, domainID uuid.UUID) (*storegen.CustomDomain, error) {
	var cd storegen.CustomDomain
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if err := checkTenantOwnership(d.ProviderID, d.EnvironmentID, tc, "custom domain", domainID); err != nil {
			return err
		}
		cd = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

// ListCustomDomains returns all custom domains for the caller's tenant.
func (s *Service) ListCustomDomains(ctx context.Context, tc tenant.Ctx) ([]storegen.CustomDomain, error) {
	var out []storegen.CustomDomain
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		domains, err := q.ListCustomDomains(ctx, storegen.ListCustomDomainsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = domains
		return err
	})
	return out, err
}

// RevokeCustomDomain revokes a custom domain. The domain status changes
// to 'revoked' and can no longer be used for auth routing. A revoked
// domain can be deleted.
func (s *Service) RevokeCustomDomain(ctx context.Context, tc tenant.Ctx, domainID uuid.UUID) (*storegen.CustomDomain, error) {
	var cd storegen.CustomDomain
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if err := checkTenantOwnership(d.ProviderID, d.EnvironmentID, tc, "custom domain", domainID); err != nil {
			return err
		}
		if d.Status == domain.CustomDomainRevoked {
			return fmt.Errorf("%w: domain %s already revoked", ErrConflict, domainID)
		}

		updated, err := q.RevokeCustomDomain(ctx, domainID)
		if err != nil {
			return mapErr(err, "revoke custom domain %s", domainID)
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "custom_domain", domainID.String(), "custom_domain.revoked", map[string]any{
			"domain": d.Domain,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "custom_domain.revoke",
			"custom_domain", domainID.String(),
			map[string]any{"domain": d.Domain}); err != nil {
			return err
		}
		cd = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

// DeleteCustomDomain removes a custom domain. Only pending or revoked
// domains can be deleted (verified domains must be revoked first to
// prevent accidental removal of an active auth domain).
func (s *Service) DeleteCustomDomain(ctx context.Context, tc tenant.Ctx, domainID uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if err := checkTenantOwnership(d.ProviderID, d.EnvironmentID, tc, "custom domain", domainID); err != nil {
			return err
		}
		if d.Status == domain.CustomDomainVerified {
			return fmt.Errorf("%w: cannot delete verified domain %s (revoke first)", ErrConflict, domainID)
		}

		rows, err := q.DeleteCustomDomain(ctx, storegen.DeleteCustomDomainParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ID: domainID,
		})
		if err != nil {
			return mapErr(err, "delete custom domain %s", domainID)
		}
		if rows == 0 {
			return fmt.Errorf("%w: custom domain %s", ErrNotFound, domainID)
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "custom_domain.delete",
			"custom_domain", domainID.String(),
			map[string]any{"domain": d.Domain}); err != nil {
			return err
		}
		return nil
	})
}

// GetDomainByHostname looks up a verified custom domain by hostname.
// This is used by the request routing layer to determine which provider
// owns a custom auth domain. Operator-only (not tenant-scoped).
func (s *Service) GetDomainByHostname(ctx context.Context, hostname string) (*storegen.CustomDomain, error) {
	var cd storegen.CustomDomain
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		d, err := q.GetCustomDomainByHostname(ctx, hostname)
		if err != nil {
			return mapErr(err, "custom domain %q", hostname)
		}
		cd = d
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

// --- helpers ---

// validateDomain checks that the domain is a valid hostname.
func validateDomain(d string) error {
	if d == "" {
		return fmt.Errorf("domain is required")
	}
	if len(d) > 253 {
		return fmt.Errorf("domain too long (max 253 characters)")
	}
	// Must not be an IP address.
	if net.ParseIP(d) != nil {
		return fmt.Errorf("domain must not be an IP address")
	}
	// Basic hostname validation: alphanumeric, hyphens, dots.
	for _, c := range d {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '-' && c != '.' {
			return fmt.Errorf("domain contains invalid character %q", c)
		}
	}
	// Must have at least one dot (e.g., "example.com").
	if !strings.Contains(d, ".") {
		return fmt.Errorf("domain must have at least one dot")
	}
	// Must not start or end with a dot or hyphen.
	if strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") ||
		strings.HasPrefix(d, "-") || strings.HasSuffix(d, "-") {
		return fmt.Errorf("domain must not start or end with a dot or hyphen")
	}
	return nil
}

// generateVerificationToken generates a random verification token.
func generateVerificationToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "vlogbin-verify-" + hex.EncodeToString(b)
}
