package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

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

// ErrWebhookReplayConflict is returned when a webhook delivery replay is
// attempted on a delivery that is not in a terminal, replayable state
// (dead_letter | failed). Clients should re-read the delivery list to find
// the current status.
var ErrWebhookReplayConflict = fmt.Errorf("webhook delivery is not in a replayable state")

// ReplayWebhookDeliveryByProvider requeues a terminal webhook delivery for
// immediate redelivery (operator view, cross-environment). The delivery is
// reset to 'pending' with a fresh attempt budget (attempts=0, backoff
// cleared, response trace wiped); the worker picks it up on the next drain
// and applies the normal retry/backoff policy again. Unknown deliveries or
// deliveries not owned by the provider yield ErrNotFound; non-terminal
// deliveries yield ErrWebhookReplayConflict. The replay is written to the
// provider's audit trail so incident post-mortems show who requeued what.
func (s *Service) ReplayWebhookDeliveryByProvider(ctx context.Context, providerID, deliveryID uuid.UUID, actor string) (*storegen.WebhookDelivery, error) {
	if actor == "" {
		actor = "operator"
	}
	var out *storegen.WebhookDelivery
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		d, err := q.GetWebhookDelivery(ctx, storegen.GetWebhookDeliveryParams{
			ID:         deliveryID,
			ProviderID: providerID,
		})
		if err != nil {
			return mapErr(err, "webhook delivery %s", deliveryID)
		}
		if d.Status != "dead_letter" && d.Status != "failed" {
			return fmt.Errorf("%w: delivery %s status=%s", ErrWebhookReplayConflict, deliveryID, d.Status)
		}
		upd, err := q.ReplayWebhookDelivery(ctx, storegen.ReplayWebhookDeliveryParams{
			ID:         deliveryID,
			ProviderID: providerID,
		})
		if err != nil {
			return err
		}
		out = &upd
		return insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: providerID, Valid: true},
			uuid.NullUUID{UUID: d.EnvironmentID, Valid: true},
			"operator", actor, "webhook_delivery_replay", "webhook_delivery", deliveryID.String(),
			map[string]any{"from_status": d.Status, "endpoint_id": d.EndpointID})
	})
	return out, err
}

// PurgeExpiredWebhookDeliveries deletes terminal webhook deliveries and
// terminal outbox events older than cutoff. Called by the background
// retention sweeper (NewWebhookRetentionSweeper) so webhook_deliveries and
// outbox_events do not grow without bound. Only terminal rows are removed:
// 'delivered' and 'dead_letter' deliveries, 'failed' deliveries whose retry
// window is exhausted (next_attempt_at IS NULL), and 'published' or
// dead-lettered outbox events. Pending rows and failed rows still inside
// their retry window are never purged. Deliveries are swept first so outbox
// events are only removed after the delivery traces referencing them are
// gone. Runs in the operator context so RLS does not scope the sweep.
func (s *Service) PurgeExpiredWebhookDeliveries(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		deliveries, err := q.DeleteExpiredWebhookDeliveries(ctx, cutoff)
		if err != nil {
			return err
		}
		events, err := q.DeleteExpiredOutboxEvents(ctx, cutoff)
		if err != nil {
			return err
		}
		total = deliveries + events
		return nil
	})
	return total, err
}

// NewWebhookRetentionSweeper creates a background sweeper that purges
// terminal webhook deliveries and outbox events beyond the retention window
// at the given interval. The cutoff is recomputed on every sweep so a long
// shutdown gap does not age the window.
func NewWebhookRetentionSweeper(svc *Service, retentionDays int, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("webhook_retention", func(ctx context.Context) (int64, error) {
		return svc.PurgeExpiredWebhookDeliveries(ctx, time.Now().UTC().Add(-time.Duration(retentionDays)*24*time.Hour))
	}, interval, log)
}
