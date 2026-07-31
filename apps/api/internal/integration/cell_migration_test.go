package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCellMigrationFullLifecycle(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cm-full-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	// Create two cells in the same region.
	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "cm-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "cm-to-" + uuid.NewString()[:8], "cell_type": "dedicated", "status": "active", "capacity_limits": map[string]any{"max_providers": 1},
	})
	toCellID := body["id"].(string)

	// Plan migration.
	scheduledAt := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id":  providerID,
		"from_cell_id": fromCellID,
		"to_cell_id":   toCellID,
		"reason":       "upgrade to dedicated cell",
		"initiated_by": "operator",
		"scheduled_at": scheduledAt,
	})
	if status != http.StatusCreated {
		t.Fatalf("plan migration: status %d, body %v", status, body)
	}
	migrationID := body["id"].(string)
	if body["status"] != "planned" {
		t.Fatalf("status = %v, want planned", body["status"])
	}

	// Precheck.
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations/"+migrationID+"/precheck", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("precheck: status %d, body %v", status, body)
	}
	if body["status"] != "ready" {
		t.Fatalf("status = %v, want ready", body["status"])
	}
	if body["precheck_passed"] != true {
		t.Fatal("precheck_passed must be true")
	}
	if body["data_integrity_hash"] == nil {
		t.Fatal("data_integrity_hash must be set")
	}

	// Execute migration.
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations/"+migrationID+"/execute", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("execute: status %d, body %v", status, body)
	}
	if body["status"] != "completed" {
		t.Fatalf("status = %v, want completed", body["status"])
	}
}

func TestCellMigrationCancel(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cm-cancel-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "cc-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "cc-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Plan migration.
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "test cancel", "initiated_by": "operator",
	})
	migrationID := body["id"].(string)

	// Cancel.
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations/"+migrationID+"/cancel", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("cancel: status %d, body %v", status, body)
	}
	if body["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", body["status"])
	}

	// Cannot cancel again.
	status, _ = apiReq(t, "POST", "/v1/operator/cell-migrations/"+migrationID+"/cancel", operatorToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("double cancel: status %d, want 409", status)
	}
}

func TestCellMigrationExecuteWithoutPrecheck(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cm-nopre-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "np-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "np-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Plan migration (no precheck).
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "no precheck", "initiated_by": "operator",
	})
	migrationID := body["id"].(string)

	// Execute without precheck — should fail.
	status, _ = apiReq(t, "POST", "/v1/operator/cell-migrations/"+migrationID+"/execute", operatorToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("execute without precheck: status %d, want 409", status)
	}
}

func TestCellMigrationDuplicateActive(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cm-dup-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "dup-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "dup-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// First migration.
	apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "first", "initiated_by": "operator",
	})

	// Second migration — should be rejected.
	status, _ = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "second", "initiated_by": "operator",
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate migration: status %d, want 409", status)
	}
}

func TestCellMigrationValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cm-val-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "val-cell-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	cellID := body["id"].(string)

	// Missing reason.
	status, _ = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": cellID, "to_cell_id": cellID,
		"initiated_by": "operator",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing reason: status %d, want 400", status)
	}

	// Same from/to cell.
	status, _ = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": cellID, "to_cell_id": cellID,
		"reason": "same cell", "initiated_by": "operator",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("same cell: status %d, want 400", status)
	}
}

func TestCellMigrationListAndGet(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cm-lst-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "lst-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "lst-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Plan migration.
	status, body = apiReq(t, "POST", "/v1/operator/cell-migrations", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "list test", "initiated_by": "operator",
	})
	migrationID := body["id"].(string)

	// List migrations.
	status, body = apiReq(t, "GET", "/v1/operator/cell-migrations?provider_id="+providerID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	migrations := body["cell_migrations"].([]any)
	if len(migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migrations))
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/v1/operator/cell-migrations/"+migrationID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get: status %d", status)
	}
	if body["id"] != migrationID {
		t.Fatalf("id mismatch")
	}
}
