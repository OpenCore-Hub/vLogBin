package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestOperatorQueueOverview(t *testing.T) {
	a := createProvider(t, "qov-"+uuid.NewString()[:8])
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)
	eventType := "queue.overview"
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if _, err := tx.Exec(testCtx, `
			INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id, status)
			VALUES ($1, $2, 'provider', $3, $4, '{}', 'qov-hash', $5, 'pending')`,
			a.Provider.ID, a.Environments[0].ID, uuid.New(), eventType, "qov-"+uuid.NewString()[:8]); err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
	})

	status, body := apiReq(t, "GET", "/v1/operator/queues/overview", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("queue overview: status %d, body %v", status, body)
	}
	outbox := body["outbox"].(map[string]any)
	if int64(outbox["pending"].(float64)) < 1 {
		t.Fatalf("outbox pending = %v, want >= 1", outbox["pending"])
	}
	webhooks := body["webhook_deliveries"].(map[string]any)
	if webhooks == nil {
		t.Fatal("webhook_deliveries missing from queue overview")
	}
	recent := body["recent_outbox"].([]any)
	found := false
	for _, item := range recent {
		if item.(map[string]any)["event_type"] == eventType {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("recent_outbox missing %q: %v", eventType, recent)
	}
}
