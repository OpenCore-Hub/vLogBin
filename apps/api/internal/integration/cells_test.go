package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func getFirstRegionID(t *testing.T) string {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/operator/regions", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list regions: status %d", status)
	}
	regions := body["regions"].([]any)
	if len(regions) == 0 {
		t.Fatal("no regions found")
	}
	return regions[0].(map[string]any)["id"].(string)
}

func getFirstCellID(t *testing.T) string {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/operator/cells", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list cells: status %d", status)
	}
	cells := body["cells"].([]any)
	if len(cells) == 0 {
		t.Fatal("no cells found")
	}
	return cells[0].(map[string]any)["id"].(string)
}

func TestCreateCell(t *testing.T) {
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id":       regionID,
		"code":            "cell-test-" + uuid.NewString()[:8],
		"cell_type":       "shared",
		"status":          "active",
		"capacity_limits": map[string]any{"max_providers": 100},
	})
	if status != http.StatusCreated {
		t.Fatalf("create cell: status %d, body %v", status, body)
	}
	if body["cell_type"] != "shared" {
		t.Fatalf("cell_type = %v", body["cell_type"])
	}
	if body["status"] != "active" {
		t.Fatalf("status = %v", body["status"])
	}
}

func TestGetCell(t *testing.T) {
	cellID := getFirstCellID(t)

	status, body := apiReq(t, "GET", "/v1/operator/cells/"+cellID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get cell: status %d", status)
	}
	if body["id"] != cellID {
		t.Fatalf("id mismatch: %v vs %v", body["id"], cellID)
	}
}

func TestUpdateCellStatus(t *testing.T) {
	regionID := getFirstRegionID(t)

	// Create a cell.
	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id":       regionID,
		"code":            "cell-stat-" + uuid.NewString()[:8],
		"cell_type":       "shared",
		"status":          "active",
		"capacity_limits": map[string]any{},
	})
	cellID := body["id"].(string)

	// Update to draining.
	status, body = apiReq(t, "PATCH", "/v1/operator/cells/"+cellID, operatorToken, map[string]any{
		"status": "draining",
	})
	if status != http.StatusOK {
		t.Fatalf("update to draining: status %d, body %v", status, body)
	}
	if body["status"] != "draining" {
		t.Fatalf("status = %v, want draining", body["status"])
	}

	// Update to inactive.
	status, body = apiReq(t, "PATCH", "/v1/operator/cells/"+cellID, operatorToken, map[string]any{
		"status": "inactive",
	})
	if status != http.StatusOK {
		t.Fatalf("update to inactive: status %d", status)
	}
	if body["status"] != "inactive" {
		t.Fatalf("status = %v, want inactive", body["status"])
	}

	// Reactivate.
	status, _ = apiReq(t, "PATCH", "/v1/operator/cells/"+cellID, operatorToken, map[string]any{
		"status": "active",
	})
	if status != http.StatusOK {
		t.Fatalf("reactivate: status %d", status)
	}
}

func TestAssignProviderCell(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cell-asg-"+uuid.NewString()[:8])
	cellID := getFirstCellID(t)

	// Assign provider to cell.
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/cell", operatorToken, map[string]any{
		"cell_id": cellID,
	})
	if status != http.StatusOK {
		t.Fatalf("assign cell: status %d, body %v", status, body)
	}
	if body["assigned"] != true {
		t.Fatalf("assigned = %v, want true", body["assigned"])
	}
}

func TestAssignProviderCellNotActive(t *testing.T) {
	providerID, _ := createProviderAPI(t, "cell-na-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	// Create an inactive cell.
	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id":       regionID,
		"code":            "cell-na-" + uuid.NewString()[:8],
		"cell_type":       "shared",
		"status":          "active",
		"capacity_limits": map[string]any{},
	})
	cellID := body["id"].(string)

	// Set to inactive.
	apiReq(t, "PATCH", "/v1/operator/cells/"+cellID, operatorToken, map[string]any{"status": "inactive"})

	// Cannot assign provider to inactive cell.
	status, _ = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/cell", operatorToken, map[string]any{
		"cell_id": cellID,
	})
	if status != http.StatusConflict {
		t.Fatalf("assign to inactive cell: status %d, want 409", status)
	}
}

func TestCellValidation(t *testing.T) {
	regionID := getFirstRegionID(t)

	// Missing code.
	status, _ := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID,
		"cell_type": "shared",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing code: status %d, want 400", status)
	}

	// Invalid cell_type.
	status, _ = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID,
		"code":      "bad-type",
		"cell_type": "invalid",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid cell_type: status %d, want 400", status)
	}

	// Invalid status.
	status, _ = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID,
		"code":      "bad-status",
		"cell_type": "shared",
		"status":    "unknown",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid status: status %d, want 400", status)
	}
}

// TestCellDrainingWriteFencing verifies that when a provider's cell is
// set to 'draining', billing write operations (subscription creation,
// usage ingest) are rejected with 409 cell_draining (spec Section 14:
// "写 fencing"). This is the P0 safety guarantee for failover/migration.
func TestCellDrainingWriteFencing(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "drain-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	// Create a dedicated cell and assign the provider to it.
	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id":       regionID,
		"code":            "cell-drain-" + uuid.NewString()[:8],
		"cell_type":       "shared",
		"status":          "active",
		"capacity_limits": map[string]any{},
	})
	if status != http.StatusCreated {
		t.Fatalf("create cell: status %d, body %v", status, body)
	}
	cellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/cell", operatorToken, map[string]any{
		"cell_id": cellID,
	})
	if status != http.StatusOK {
		t.Fatalf("assign cell: status %d, body %v", status, body)
	}

	// Set up billing prerequisites (catalog + customer) while cell is active.
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Sanity check: with cell active, usage ingest succeeds (201 on first ingest).
	status, _ = ingestUsage(t, apiKey, "tx-ok-"+uuid.NewString()[:8], custExt, "api_calls", now, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("usage ingest with active cell: status %d, want 201", status)
	}

	// Flip the cell to draining — this activates write fencing.
	status, body = apiReq(t, "PATCH", "/v1/operator/cells/"+cellID, operatorToken, map[string]any{
		"status": "draining",
	})
	if status != http.StatusOK {
		t.Fatalf("update to draining: status %d, body %v", status, body)
	}
	if body["status"] != "draining" {
		t.Fatalf("cell status = %v, want draining", body["status"])
	}

	// --- Write fencing: subscription creation must be rejected. ---
	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":          "sub-blocked-" + uuid.NewString()[:8],
		"customer_external_id": custExt,
		"catalog_version_id":   versionID,
		"plan_code":            "starter",
	})
	if status != http.StatusConflict {
		t.Fatalf("create subscription during draining: status %d, want 409", status)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "cell_draining" {
		t.Fatalf("error.code = %v, want cell_draining (body %v)", errObj, body)
	}

	// --- Write fencing: usage ingest must be rejected. ---
	status, body = ingestUsage(t, apiKey, "tx-blocked-"+uuid.NewString()[:8], custExt, "api_calls", now, map[string]any{"count": 1})
	if status != http.StatusConflict {
		t.Fatalf("ingest usage during draining: status %d, want 409", status)
	}
	errObj, _ = body["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "cell_draining" {
		t.Fatalf("error.code = %v, want cell_draining (body %v)", errObj, body)
	}

	// Restore the cell to active — writes must succeed again.
	status, _ = apiReq(t, "PATCH", "/v1/operator/cells/"+cellID, operatorToken, map[string]any{
		"status": "active",
	})
	if status != http.StatusOK {
		t.Fatalf("restore to active: status %d", status)
	}

	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":          "sub-restored-" + uuid.NewString()[:8],
		"customer_external_id": custExt,
		"catalog_version_id":   versionID,
		"plan_code":            "starter",
	})
	if status != http.StatusCreated {
		t.Fatalf("create subscription after restore: status %d, want 201", status)
	}

	status, _ = ingestUsage(t, apiKey, "tx-restored-"+uuid.NewString()[:8], custExt, "api_calls", now, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("usage ingest after restore: status %d, want 201", status)
	}
}
