package service

import (
	"context"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/keys"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreatedProviderCredential pairs the operator credential view with the
// plaintext API key, which is returned exactly once (R17 parity with OIDC
// app secrets).
type CreatedProviderCredential struct {
	Credential storegen.GetCredentialByProviderRow `json:"credential"`
	APIKey     string                              `json:"api_key"`
}

// ListProviderCredentialsByEnv lists the API keys of one provider environment
// (operator path, for the Console API Keys page with the active env).
func (s *Service) ListProviderCredentialsByEnv(
	ctx context.Context,
	providerID, envID uuid.UUID,
) ([]storegen.Credential, error) {
	var out []storegen.Credential
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		creds, err := q.ListCredentialsByEnvironment(ctx, storegen.ListCredentialsByEnvironmentParams{
			ProviderID: providerID, EnvironmentID: envID,
		})
		out = creds
		return err
	})
	return out, err
}

// CreateProviderCredential issues an API key for one provider environment
// from the operator console. Unlike the provider-domain CreateCredential it
// does not apply scope attenuation (operator is privileged), and the audit
// record uses actor_type=operator with the supplied operator identity.
func (s *Service) CreateProviderCredential(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	name string,
	scopes []string,
	expiresAt *time.Time,
	actor string,
) (*CreatedProviderCredential, error) {
	if err := validateCredentialInput(name, scopes, expiresAt); err != nil {
		return nil, err
	}
	if actor == "" {
		actor = "operator"
	}

	var out CreatedProviderCredential
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		plaintext, err := keys.Generate(env.Kind)
		if err != nil {
			return err
		}
		cred, err := q.CreateCredential(ctx, storegen.CreateCredentialParams{
			ProviderID:    providerID,
			EnvironmentID: env.ID,
			Name:          name,
			KeyPrefix:     keys.Prefix(plaintext),
			KeyHash:       keys.Hash(plaintext),
			Scopes:        scopes,
			ExpiresAt:     expiresAt,
		})
		if err != nil {
			return mapErr(err, "credential %q", name)
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "credential", cred.ID.String(), "credential.created", map[string]any{
			"credential_id": cred.ID.String(), "name": name, "scopes": scopes,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "credential.create", "credential", cred.ID.String(),
			map[string]any{"credential_name": name, "scopes": scopes, "key_prefix": cred.KeyPrefix}); err != nil {
			return err
		}
		view, err := q.GetCredentialByProvider(ctx, storegen.GetCredentialByProviderParams{
			ID: cred.ID, ProviderID: providerID,
		})
		if err != nil {
			return err
		}
		out.Credential = view
		out.APIKey = plaintext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RotateProviderCredential atomically issues a replacement API key with the
// same scopes/expiry and revokes the old key in one transaction. The old key
// stops working immediately; the new plaintext is returned exactly once.
func (s *Service) RotateProviderCredential(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	credentialID uuid.UUID,
	actor string,
) (*CreatedProviderCredential, error) {
	if actor == "" {
		actor = "operator"
	}

	var out CreatedProviderCredential
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		old, err := q.GetCredentialByProvider(ctx, storegen.GetCredentialByProviderParams{
			ID: credentialID, ProviderID: providerID,
		})
		if err != nil {
			return mapErr(err, "credential %s", credentialID)
		}
		if old.RevokedAt != nil {
			return fmt.Errorf("%w: credential %s is already revoked", ErrConflict, credentialID)
		}

		plaintext, err := keys.Generate(env.Kind)
		if err != nil {
			return err
		}
		created, err := q.CreateCredential(ctx, storegen.CreateCredentialParams{
			ProviderID:    providerID,
			EnvironmentID: env.ID,
			Name:          old.Name,
			KeyPrefix:     keys.Prefix(plaintext),
			KeyHash:       keys.Hash(plaintext),
			Scopes:        old.Scopes,
			ExpiresAt:     old.ExpiresAt,
		})
		if err != nil {
			return mapErr(err, "credential %q", old.Name)
		}
		if _, err := q.RevokeCredential(ctx, credentialID); err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "credential", created.ID.String(), "credential.created", map[string]any{
			"credential_id": created.ID.String(), "name": created.Name, "scopes": created.Scopes,
		}); err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "credential", credentialID.String(), "credential.revoked", map[string]any{
			"credential_id": credentialID.String(),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "credential.rotate", "credential", credentialID.String(),
			map[string]any{"old_credential_id": credentialID.String(), "new_credential_id": created.ID.String(), "name": old.Name}); err != nil {
			return err
		}
		view, err := q.GetCredentialByProvider(ctx, storegen.GetCredentialByProviderParams{
			ID: created.ID, ProviderID: providerID,
		})
		if err != nil {
			return err
		}
		out.Credential = view
		out.APIKey = plaintext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func validateCredentialInput(name string, scopes []string, expiresAt *time.Time) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if len(scopes) == 0 {
		return fmt.Errorf("%w: at least one scope is required", ErrValidation)
	}
	for _, sc := range scopes {
		if !domain.ValidScope(sc) {
			return fmt.Errorf("%w: unknown scope %q", ErrValidation, sc)
		}
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return fmt.Errorf("%w: expires_at must be in the future", ErrValidation)
	}
	return nil
}

// CreateWebhookEndpointByProvider registers a webhook endpoint from the
// operator console. The same SSRF validation and auto-generated signing
// secret apply as the provider-domain path; the audit actor is the operator.
func (s *Service) CreateWebhookEndpointByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	env *storegen.Environment,
	in CreateWebhookEndpointInput,
	actor string,
) (*storegen.WebhookEndpoint, error) {
	if in.URL == "" {
		return nil, fmt.Errorf("%w: url is required", ErrValidation)
	}
	if err := s.urlValidator(in.URL); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}
	if actor == "" {
		actor = "operator"
	}
	secret := in.Secret
	if secret == "" {
		var err error
		secret, err = generateSecret()
		if err != nil {
			return nil, fmt.Errorf("generate webhook secret: %w", err)
		}
	}
	events := in.Events
	if events == nil {
		events = []string{}
	}

	var endpoint storegen.WebhookEndpoint
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		ep, err := q.CreateWebhookEndpoint(ctx, storegen.CreateWebhookEndpointParams{
			ProviderID:    providerID,
			EnvironmentID: env.ID,
			Url:           in.URL,
			Secret:        secret,
			Enabled:       true,
			Events:        events,
		})
		if err != nil {
			return mapErr(err, "webhook endpoint url %q", in.URL)
		}
		if err := emitOutboxTx(ctx, q, providerID, env.ID, "webhook", ep.ID.String(), "webhook.endpoint_created", map[string]any{
			"webhook_endpoint_id": ep.ID.String(), "url": in.URL,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: env.ID, Valid: true},
			"operator", actor, "webhook.create", "webhook_endpoint", ep.ID.String(),
			map[string]any{"url": in.URL, "events": events}); err != nil {
			return err
		}
		endpoint = ep
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &endpoint, nil
}

// DeleteWebhookEndpointByProvider removes one webhook endpoint scoped to a
// provider environment. Unknown endpoints yield ErrNotFound; deletion is
// recorded in the outbox and on the provider's audit trail.
func (s *Service) DeleteWebhookEndpointByProvider(
	ctx context.Context,
	providerID uuid.UUID,
	envID, endpointID uuid.UUID,
	actor string,
) error {
	if actor == "" {
		actor = "operator"
	}
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		ep, err := q.GetWebhookEndpointByID(ctx, storegen.GetWebhookEndpointByIDParams{
			ID: endpointID, ProviderID: providerID, EnvironmentID: envID,
		})
		if err != nil {
			return mapErr(err, "webhook endpoint %s", endpointID)
		}
		if err := q.DeleteWebhookEndpoint(ctx, storegen.DeleteWebhookEndpointParams{
			ID: endpointID, ProviderID: providerID, EnvironmentID: envID,
		}); err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, providerID, envID, "webhook", endpointID.String(), "webhook.endpoint_deleted", map[string]any{
			"webhook_endpoint_id": endpointID.String(), "url": ep.Url,
		}); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{UUID: envID, Valid: true},
			"operator", actor, "webhook.delete", "webhook_endpoint", endpointID.String(),
			map[string]any{"url": ep.Url})
	})
}

// StreamEventsByProvider is the operator view of the Enterprise Event Stream:
// the same cursor/type/aggregate_type contract as /v1/events, scoped to one
// provider environment for the Console Events page.
func (s *Service) StreamEventsByProvider(
	ctx context.Context,
	providerID, envID uuid.UUID,
	cursor uuid.UUID,
	eventType, aggregateType string,
	limit int32,
) (*StreamEventsResult, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var result StreamEventsResult
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		events, err := q.StreamOutboxEvents(ctx, storegen.StreamOutboxEventsParams{
			ProviderID:    providerID,
			EnvironmentID: envID,
			Column3:       cursor,
			Column4:       eventType,
			Column5:       aggregateType,
			Limit:         limit + 1,
		})
		if err != nil {
			return err
		}
		result.HasMore = int32(len(events)) > limit
		if result.HasMore {
			events = events[:limit]
		}
		result.Events = events
		if len(events) > 0 {
			lastID := events[len(events)-1].ID
			result.NextCursor = &lastID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
