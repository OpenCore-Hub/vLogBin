package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestFailoverFullLifecycle(t *testing.T) {
	providerID, _ := createProviderAPI(t, "fo-full-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	// Create two cells in the same region.
	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id":       regionID,
		"code":            "fo-from-" + uuid.NewString()[:8],
		"cell_type":       "shared",
		"status":          "active",
		"capacity_limits": map[string]any{},
	})
	if status != http.StatusCreated {
		t.Fatalf("create from cell: status %d", status)
	}
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id":       regionID,
		"code":            "fo-to-" + uuid.NewString()[:8],
		"cell_type":       "shared",
		"status":          "active",
		"capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Initiate failover.
	status, body = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id":  providerID,
		"from_cell_id": fromCellID,
		"to_cell_id":   toCellID,
		"reason":       "planned maintenance",
		"initiated_by": "operator",
	})
	if status != http.StatusCreated {
		t.Fatalf("initiate failover: status %d, body %v", status, body)
	}
	failoverID := body["id"].(string)
	if body["status"] != "initiated" {
		t.Fatalf("status = %v, want initiated", body["status"])
	}
	if body["fencing_token"] == "" {
		t.Fatal("fencing_token must be generated")
	}

	// Fence (write fencing on source cell).
	status, body = apiReq(t, "POST", "/v1/operator/failovers/"+failoverID+"/fence", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("fence: status %d, body %v", status, body)
	}
	if body["status"] != "fenced" {
		t.Fatalf("status = %v, want fenced", body["status"])
	}

	// Switch (reassign provider to target cell).
	status, body = apiReq(t, "POST", "/v1/operator/failovers/"+failoverID+"/switch", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("switch: status %d, body %v", status, body)
	}
	if body["status"] != "switched" {
		t.Fatalf("status = %v, want switched", body["status"])
	}

	// Complete — replay counts are now computed automatically.
	status, body = apiReq(t, "POST", "/v1/operator/failovers/"+failoverID+"/complete", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("complete: status %d, body %v", status, body)
	}
	if body["status"] != "completed" {
		t.Fatalf("status = %v, want completed", body["status"])
	}
	// replayed_usage and replayed_outbox are computed from the database.
	if body["replayed_usage"] == nil {
		t.Fatal("replayed_usage must be set")
	}
	if body["replayed_outbox"] == nil {
		t.Fatal("replayed_outbox must be set")
	}
}

func TestFailoverAbort(t *testing.T) {
	providerID, _ := createProviderAPI(t, "fo-abrt-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	// Create two cells.
	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "ab-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "ab-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Initiate + fence.
	status, body = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "test abort", "initiated_by": "operator",
	})
	failoverID := body["id"].(string)

	apiReq(t, "POST", "/v1/operator/failovers/"+failoverID+"/fence", operatorToken, nil)

	// Abort.
	status, body = apiReq(t, "POST", "/v1/operator/failovers/"+failoverID+"/abort", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("abort: status %d, body %v", status, body)
	}
	if body["status"] != "aborted" {
		t.Fatalf("status = %v, want aborted", body["status"])
	}

	// Cannot abort again.
	status, _ = apiReq(t, "POST", "/v1/operator/failovers/"+failoverID+"/abort", operatorToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("double abort: status %d, want 409", status)
	}
}

func TestFailoverCrossRegionRejected(t *testing.T) {
	providerID, _ := createProviderAPI(t, "fo-xreg-"+uuid.NewString()[:8])

	// List all cells and find cells in different regions.
	status, body := apiReq(t, "GET", "/v1/operator/cells", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list cells: status %d", status)
	}
	cells := body["cells"].([]any)
	if len(cells) < 2 {
		t.Skip("need at least 2 cells to test cross-region rejection")
	}

	// Find two cells with different region_ids.
	cell0 := cells[0].(map[string]any)
	cell1 := cells[1].(map[string]any)
	if cell0["region_id"] == cell1["region_id"] {
		t.Skip("both cells are in the same region; cannot test cross-region rejection")
	}

	// Attempt failover across regions — should fail.
	status, body = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id":  providerID,
		"from_cell_id": cell0["id"].(string),
		"to_cell_id":   cell1["id"].(string),
		"reason":       "cross-region attempt",
		"initiated_by": "operator",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("cross-region failover: status %d, want 400, body %v", status, body)
	}
}

func TestFailoverDuplicateActive(t *testing.T) {
	providerID, _ := createProviderAPI(t, "fo-dup-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "dup-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "dup-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Initiate first failover.
	apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "first", "initiated_by": "operator",
	})

	// Attempt second failover — should be rejected.
	status, _ = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "second", "initiated_by": "operator",
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate failover: status %d, want 409", status)
	}
}

func TestFailoverValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "fo-val-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "val-cell-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	cellID := body["id"].(string)

	// Missing reason.
	status, _ = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": cellID, "to_cell_id": cellID,
		"initiated_by": "operator",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing reason: status %d, want 400", status)
	}

	// Same from/to cell.
	status, _ = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": cellID, "to_cell_id": cellID,
		"reason": "same cell", "initiated_by": "operator",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("same cell: status %d, want 400", status)
	}
}

func TestFailoverListAndGet(t *testing.T) {
	providerID, _ := createProviderAPI(t, "fo-lst-"+uuid.NewString()[:8])
	regionID := getFirstRegionID(t)

	status, body := apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "lst-from-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	fromCellID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/operator/cells", operatorToken, map[string]any{
		"region_id": regionID, "code": "lst-to-" + uuid.NewString()[:8], "cell_type": "shared", "status": "active", "capacity_limits": map[string]any{},
	})
	toCellID := body["id"].(string)

	// Initiate failover.
	status, body = apiReq(t, "POST", "/v1/operator/failovers", operatorToken, map[string]any{
		"provider_id": providerID, "from_cell_id": fromCellID, "to_cell_id": toCellID,
		"reason": "list test", "initiated_by": "operator",
	})
	failoverID := body["id"].(string)

	// List failovers.
	status, body = apiReq(t, "GET", "/v1/operator/failovers?provider_id="+providerID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	failovers := body["failovers"].([]any)
	if len(failovers) != 1 {
		t.Fatalf("expected 1 failover, got %d", len(failovers))
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/v1/operator/failovers/"+failoverID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get: status %d", status)
	}
	if body["id"] != failoverID {
		t.Fatalf("id mismatch")
	}
}
