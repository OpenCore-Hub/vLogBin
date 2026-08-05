package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ListCustomDomainsByProviderEnv lists the custom domains of one provider
// environment (operator path, for the Console Settings page).
func (s *Service) ListCustomDomainsByProviderEnv(
	ctx context.Context,
	providerID, envID uuid.UUID,
) ([]storegen.CustomDomain, error) {
	var out []storegen.CustomDomain
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		domains, err := q.ListCustomDomains(ctx, storegen.ListCustomDomainsParams{
			ProviderID: providerID, EnvironmentID: envID,
		})
		out = domains
		return err
	})
	return out, err
}

// RegisterCustomDomainByProvider registers a domain for DNS verification from
// the operator console. The audit actor is the operator identity instead of a
// provider credential.
func (s *Service) RegisterCustomDomainByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	domain string,
	actor string,
) (*storegen.CustomDomain, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if err := validateDomain(domain); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if actor == "" {
		actor = "operator"
	}
	token := generateVerificationToken()

	var cd storegen.CustomDomain
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		created, err := q.CreateCustomDomain(ctx, storegen.CreateCustomDomainParams{
			ProviderID:        providerID,
			EnvironmentID:     env.ID,
			Domain:            domain,
			VerificationToken: token,
		})
		if err != nil {
			mappedErr := mapErr(err, "custom domain %q", domain)
			if errors.Is(mappedErr, ErrConflict) {
				return fmt.Errorf("%w: domain %q is already registered", ErrDomainTaken, domain)
			}
			return mappedErr
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "custom_domain", created.ID.String(), "custom_domain.registered", map[string]any{
			"domain": domain,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "custom_domain.register", "custom_domain", created.ID.String(),
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

// VerifyCustomDomainByProvider verifies DNS ownership from the operator
// console using the same TXT token contract as the provider-domain path.
func (s *Service) VerifyCustomDomainByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	domainID uuid.UUID,
	actor string,
) (*storegen.CustomDomain, error) {
	if actor == "" {
		actor = "operator"
	}
	var cd storegen.CustomDomain
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if d.ProviderID != providerID || d.EnvironmentID != env.ID {
			return fmt.Errorf("%w: custom domain %s", ErrNotFound, domainID)
		}
		if d.Status != domain.CustomDomainPending {
			return fmt.Errorf("%w: domain %s is not pending (status=%s)", ErrConflict, domainID, d.Status)
		}
		txtName := fmt.Sprintf("%s.%s", domain.DNSVerificationPrefix, d.Domain)
		records, err := s.dnsResolver(ctx, txtName)
		if err != nil {
			return fmt.Errorf("%w: DNS lookup failed for %s: %v", ErrValidation, txtName, err)
		}
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
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "custom_domain", domainID.String(), "custom_domain.verified", map[string]any{
			"domain": d.Domain,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "custom_domain.verify", "custom_domain", domainID.String(),
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

// RevokeCustomDomainByProvider revokes a verified/pending custom domain from
// the operator console.
func (s *Service) RevokeCustomDomainByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	domainID uuid.UUID,
	actor string,
) (*storegen.CustomDomain, error) {
	if actor == "" {
		actor = "operator"
	}
	var cd storegen.CustomDomain
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if d.ProviderID != providerID || d.EnvironmentID != env.ID {
			return fmt.Errorf("%w: custom domain %s", ErrNotFound, domainID)
		}
		if d.Status == domain.CustomDomainRevoked {
			return fmt.Errorf("%w: domain %s already revoked", ErrConflict, domainID)
		}
		updated, err := q.RevokeCustomDomain(ctx, domainID)
		if err != nil {
			return mapErr(err, "revoke custom domain %s", domainID)
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "custom_domain", domainID.String(), "custom_domain.revoked", map[string]any{
			"domain": d.Domain,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "custom_domain.revoke", "custom_domain", domainID.String(),
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

// DeleteCustomDomainByProvider removes a pending/revoked custom domain from
// the operator console. Verified domains must be revoked first.
func (s *Service) DeleteCustomDomainByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	domainID uuid.UUID,
	actor string,
) error {
	if actor == "" {
		actor = "operator"
	}
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		d, err := q.GetCustomDomainByID(ctx, domainID)
		if err != nil {
			return mapErr(err, "custom domain %s", domainID)
		}
		if d.ProviderID != providerID || d.EnvironmentID != env.ID {
			return fmt.Errorf("%w: custom domain %s", ErrNotFound, domainID)
		}
		if d.Status == domain.CustomDomainVerified {
			return fmt.Errorf("%w: cannot delete verified domain %s (revoke first)", ErrConflict, domainID)
		}
		rows, err := q.DeleteCustomDomain(ctx, storegen.DeleteCustomDomainParams{
			ProviderID: providerID, EnvironmentID: env.ID, ID: domainID,
		})
		if err != nil {
			return mapErr(err, "delete custom domain %s", domainID)
		}
		if rows == 0 {
			return fmt.Errorf("%w: custom domain %s", ErrNotFound, domainID)
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "custom_domain", domainID.String(), "custom_domain.deleted", map[string]any{
			"domain": d.Domain, "status": d.Status,
		}); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "custom_domain.delete", "custom_domain", domainID.String(),
			map[string]any{"domain": d.Domain})
	})
}

// ListNotificationConfigsByProviderEnv lists decrypted notification configs
// for one provider environment (operator path).
func (s *Service) ListNotificationConfigsByProviderEnv(
	ctx context.Context,
	providerID, envID uuid.UUID,
) ([]NotificationConfigResult, error) {
	var out []NotificationConfigResult
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		configs, err := q.ListNotificationConfigs(ctx, storegen.ListNotificationConfigsParams{
			ProviderID: providerID, EnvironmentID: envID,
		})
		if err != nil {
			return err
		}
		for _, nc := range configs {
			plaintext, err := s.encryptor.Decrypt(string(nc.ConfigEnc))
			if err != nil {
				slog.Error("failed to decrypt notification config", "config_id", nc.ID, "error", err)
				continue
			}
			var cfg map[string]any
			if err := json.Unmarshal([]byte(plaintext), &cfg); err != nil {
				slog.Error("invalid stored notification config", "config_id", nc.ID, "error", err)
				continue
			}
			out = append(out, NotificationConfigResult{NotificationConfig: nc, Config: cfg})
		}
		return nil
	})
	return out, err
}

// SetNotificationConfigByProvider saves a notification config from the
// operator console. Secrets are encrypted at rest exactly like the provider
// domain path; the audit actor is the operator identity.
func (s *Service) SetNotificationConfigByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	in NotificationConfigInput,
	actor string,
) (*NotificationConfigResult, error) {
	if !domain.ValidNotificationChannel(in.Channel) {
		return nil, fmt.Errorf("%w: channel must be email or sms", ErrValidation)
	}
	if in.ProviderType == "" {
		return nil, fmt.Errorf("%w: provider_type is required", ErrValidation)
	}
	if in.FromAddress == "" {
		return nil, fmt.Errorf("%w: from_address is required", ErrValidation)
	}
	if len(in.Config) == 0 {
		return nil, fmt.Errorf("%w: config must not be empty", ErrValidation)
	}
	if s.encryptor == nil {
		return nil, fmt.Errorf("%w: notification encryption is not configured", ErrValidation)
	}
	if actor == "" {
		actor = "operator"
	}

	plaintext, err := json.Marshal(in.Config)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid config: %v", ErrValidation, err)
	}
	encrypted, err := s.encryptor.Encrypt(string(plaintext))
	if err != nil {
		return nil, fmt.Errorf("encrypt notification config: %w", err)
	}

	var result NotificationConfigResult
	err = s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		nc, err := q.UpsertNotificationConfig(ctx, storegen.UpsertNotificationConfigParams{
			ProviderID:    providerID,
			EnvironmentID: env.ID,
			Channel:       in.Channel,
			ProviderType:  in.ProviderType,
			ConfigEnc:     []byte(encrypted),
			FromAddress:   in.FromAddress,
			Enabled:       in.Enabled,
		})
		if err != nil {
			return mapErr(err, "notification config %q", in.Channel)
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "notification_config", nc.ID.String(), "notification.config_set", map[string]any{
			"channel": in.Channel, "provider_type": in.ProviderType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "notification.config_set", "notification_config", nc.ID.String(),
			map[string]any{"channel": in.Channel, "provider_type": in.ProviderType}); err != nil {
			return err
		}
		result.NotificationConfig = nc
		result.Config = in.Config
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteNotificationConfigByProvider removes a notification channel config
// from the operator console.
func (s *Service) DeleteNotificationConfigByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	channel string,
	actor string,
) error {
	if actor == "" {
		actor = "operator"
	}
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		rows, err := q.DeleteNotificationConfig(ctx, storegen.DeleteNotificationConfigParams{
			ProviderID: providerID, EnvironmentID: env.ID, Channel: channel,
		})
		if err != nil {
			return mapErr(err, "notification config %q", channel)
		}
		if rows == 0 {
			return fmt.Errorf("%w: notification config %q", ErrNotFound, channel)
		}
		return insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "notification.config_delete", "notification_config", strings.TrimSpace(channel),
			map[string]any{"channel": channel})
	})
}
