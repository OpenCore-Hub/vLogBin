package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSLATierCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sla-crud-"+uuid.NewString()[:8])

	// Create SLA tier.
	status, body := apiReq(t, "POST", "/v1/sla-tiers", apiKey, map[string]any{
		"code":              "enterprise",
		"name":              "Enterprise SLA",
		"uptime_sla":        99.99,
		"priority_level":    5,
		"reserved_capacity": map[string]any{"max_qps": 10000, "max_users": 50000},
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d, body %v", status, body)
	}
	tierID := body["id"].(string)
	if body["code"] != "enterprise" {
		t.Fatalf("code = %v", body["code"])
	}
	if body["uptime_sla"] != 99.99 {
		t.Fatalf("uptime_sla = %v, want 99.99", body["uptime_sla"])
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/v1/sla-tiers/"+tierID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get: status %d", status)
	}
	if body["name"] != "Enterprise SLA" {
		t.Fatalf("name = %v", body["name"])
	}

	// List.
	status, body = apiReq(t, "GET", "/v1/sla-tiers", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	tiers := body["sla_tiers"].([]any)
	if len(tiers) != 1 {
		t.Fatalf("expected 1 tier, got %d", len(tiers))
	}

	// Update.
	status, body = apiReq(t, "PATCH", "/v1/sla-tiers/"+tierID, apiKey, map[string]any{
		"code":              "enterprise",
		"name":              "Enterprise SLA Plus",
		"uptime_sla":        99.95,
		"priority_level":    5,
		"reserved_capacity": map[string]any{"max_qps": 15000},
	})
	if status != http.StatusOK {
		t.Fatalf("update: status %d, body %v", status, body)
	}
	if body["name"] != "Enterprise SLA Plus" {
		t.Fatalf("name = %v, want Enterprise SLA Plus", body["name"])
	}

	// Delete.
	status, _ = apiReq(t, "DELETE", "/v1/sla-tiers/"+tierID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", status)
	}
}

func TestSLATierValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sla-val-"+uuid.NewString()[:8])

	// Invalid uptime_sla.
	status, _ := apiReq(t, "POST", "/v1/sla-tiers", apiKey, map[string]any{
		"code":           "bad",
		"name":           "Bad",
		"uptime_sla":     150.0,
		"priority_level": 1,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid uptime: status %d, want 400", status)
	}

	// Invalid priority_level.
	status, _ = apiReq(t, "POST", "/v1/sla-tiers", apiKey, map[string]any{
		"code":           "bad2",
		"name":           "Bad2",
		"uptime_sla":     99.9,
		"priority_level": 6,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid priority: status %d, want 400", status)
	}

	// Missing code.
	status, _ = apiReq(t, "POST", "/v1/sla-tiers", apiKey, map[string]any{
		"name":           "NoCode",
		"uptime_sla":     99.9,
		"priority_level": 1,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing code: status %d, want 400", status)
	}
}

func TestSLATierCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "sla-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "sla-iso-b-"+uuid.NewString()[:8])

	apiReq(t, "POST", "/v1/sla-tiers", keyA, map[string]any{
		"code":           "pro",
		"name":           "Pro",
		"uptime_sla":     99.9,
		"priority_level": 3,
	})

	// B cannot see A's tiers.
	status, body := apiReq(t, "GET", "/v1/sla-tiers", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	tiers := body["sla_tiers"].([]any)
	if len(tiers) != 0 {
		t.Fatalf("B: expected 0 tiers, got %d (RLS leak)", len(tiers))
	}
}
