package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreateWebhookEndpointInput is the validated request body for creating a
// webhook endpoint. Events is an optional event-type filter; an empty
// slice subscribes the endpoint to all event types.
type CreateWebhookEndpointInput struct {
	URL    string
	Secret string
	Events []string
}

// CreateWebhookEndpoint registers a webhook endpoint for the caller's
// tenant. The URL is validated against SSRF rules (private IPs, cloud
// metadata). When Secret is empty a random 32-byte hex secret is generated.
// The creation is recorded in the audit log and outbox.
func (s *Service) CreateWebhookEndpoint(ctx context.Context, tc tenant.Ctx, in CreateWebhookEndpointInput) (*storegen.WebhookEndpoint, error) {
	if in.URL == "" {
		return nil, fmt.Errorf("%w: url is required", ErrValidation)
	}
	if err := s.urlValidator(in.URL); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
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
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		ep, err := q.CreateWebhookEndpoint(ctx, storegen.CreateWebhookEndpointParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Url:           in.URL,
			Secret:        secret,
			Enabled:       true,
			Events:        events,
		})
		if err != nil {
			return mapErr(err, "webhook endpoint url %q", in.URL)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "webhook", ep.ID.String(), "webhook.endpoint_created", map[string]any{
			"webhook_endpoint_id": ep.ID.String(),
			"url":                 in.URL,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "webhook.create", "webhook_endpoint", ep.ID.String(), nil); err != nil {
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

// ListWebhookEndpoints returns all webhook endpoints for the caller's
// tenant, ordered by creation time.
func (s *Service) ListWebhookEndpoints(ctx context.Context, tc tenant.Ctx) ([]storegen.WebhookEndpoint, error) {
	var out []storegen.WebhookEndpoint
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		eps, err := q.ListWebhookEndpoints(ctx, storegen.ListWebhookEndpointsParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
		})
		out = eps
		return err
	})
	return out, err
}

// DeleteWebhookEndpoint removes a webhook endpoint (tenant-scoped).
func (s *Service) DeleteWebhookEndpoint(ctx context.Context, tc tenant.Ctx, id uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		return q.DeleteWebhookEndpoint(ctx, storegen.DeleteWebhookEndpointParams{
			ID:            id,
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
		})
	})
}

// ListWebhookDeliveries returns recent delivery records for the caller's
// tenant (for monitoring).
func (s *Service) ListWebhookDeliveries(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.WebhookDelivery, error) {
	var out []storegen.WebhookDelivery
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		ds, err := q.ListWebhookDeliveriesByTenant(ctx, storegen.ListWebhookDeliveriesByTenantParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Limit:         limit,
		})
		out = ds
		return err
	})
	return out, err
}

// generateSecret produces a random 32-byte hex string for HMAC signing.
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ListWebhooksByProvider returns all webhook endpoints for a provider
// across all environments (operator view).
func (s *Service) ListWebhooksByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.WebhookEndpoint, error) {
	var out []storegen.WebhookEndpoint
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		eps, err := q.ListWebhookEndpointsByProvider(ctx, providerID)
		out = eps
		return err
	})
	return out, err
}

// ListWebhookDeliveriesByProvider returns recent delivery records for a
// provider across all environments (operator view).
func (s *Service) ListWebhookDeliveriesByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.WebhookDelivery, error) {
	var out []storegen.WebhookDelivery
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ds, err := q.ListWebhookDeliveriesByProvider(ctx, providerID)
		out = ds
		return err
	})
	return out, err
}
