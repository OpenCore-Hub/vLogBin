package integration

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// setupQuotaTestProvider creates a provider, publishes a catalog, and creates
// a customer + subscription. Returns (apiKey, subscriptionID).
func setupQuotaTestProvider(t *testing.T, prefix string) (string, string) {
	t.Helper()
	_, apiKey := createProviderAPI(t, prefix+"-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	_, subID := createCustomerAndSubscription(t, apiKey, versionID)
	return apiKey, subID
}

func TestQuotaReserveCommitRelease(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-rcr")

	// Set a quota limit of 1000.
	status, body := apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})
	if status != http.StatusOK {
		t.Fatalf("set quota limit: status %d, body %v", status, body)
	}

	// Reserve 300.
	status, body = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":          "api_calls",
		"amount":             300,
		"reservation_id":     "res-1",
		"expires_in_seconds": 0,
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve 300: status %d, body %v", status, body)
	}
	reservationID := body["id"].(string)
	if body["status"] != "reserved" {
		t.Fatalf("status = %v, want reserved", body["status"])
	}

	// Check usage: reserved=300, committed=0.
	status, body = apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/usage?key=api_calls", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get usage: status %d, body %v", status, body)
	}
	if int64(body["committed"].(float64)) != 0 || int64(body["reserved"].(float64)) != 300 {
		t.Fatalf("usage = committed=%v reserved=%v, want 0/300", body["committed"], body["reserved"])
	}

	// Commit the reservation.
	status, body = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/commit", apiKey, map[string]any{
		"reservation_id": reservationID,
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status %d, body %v", status, body)
	}
	if body["status"] != "committed" {
		t.Fatalf("status = %v, want committed", body["status"])
	}

	// Check usage: reserved=0, committed=300.
	status, body = apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/usage?key=api_calls", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get usage after commit: status %d", status)
	}
	if int64(body["committed"].(float64)) != 300 || int64(body["reserved"].(float64)) != 0 {
		t.Fatalf("usage = committed=%v reserved=%v, want 300/0", body["committed"], body["reserved"])
	}

	// Reserve another 500 and release it.
	status, body = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         500,
		"reservation_id": "res-2",
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve 500: status %d, body %v", status, body)
	}
	res2ID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/release", apiKey, map[string]any{
		"reservation_id": res2ID,
	})
	if status != http.StatusOK {
		t.Fatalf("release: status %d, body %v", status, body)
	}
	if body["status"] != "released" {
		t.Fatalf("status = %v, want released", body["status"])
	}

	// Usage: committed=300, reserved=0 (released reservation freed).
	status, body = apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/usage?key=api_calls", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get usage after release: status %d", status)
	}
	if int64(body["committed"].(float64)) != 300 || int64(body["reserved"].(float64)) != 0 {
		t.Fatalf("usage = committed=%v reserved=%v, want 300/0", body["committed"], body["reserved"])
	}
}

func TestQuotaExceeded(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-exc")

	// Set limit of 100.
	status, _ := apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 100,
		"period_type": "monthly",
	})
	if status != http.StatusOK {
		t.Fatalf("set limit: status %d", status)
	}

	// Reserve 80 — OK.
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         80,
		"reservation_id": "res-ok",
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve 80: status %d, want 201", status)
	}

	// Reserve 30 more — exceeds limit (80+30=110 > 100).
	status, body := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         30,
		"reservation_id": "res-exceed",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("reserve 30 (exceed): status %d, want 422, body %v", status, body)
	}
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "quota_exceeded" {
		t.Fatalf("error code = %v, want quota_exceeded", errObj["code"])
	}

	// Reserve 20 more — OK (80+20=100 <= 100).
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         20,
		"reservation_id": "res-ok2",
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve 20: status %d, want 201", status)
	}
}

func TestQuotaIdempotency(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-idm")

	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})

	// Reserve with reservation_id "idem-1".
	status, body1 := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         100,
		"reservation_id": "idem-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("first reserve: status %d", status)
	}
	id1 := body1["id"].(string)

	// Retry with the same reservation_id — must return the same reservation.
	status, body2 := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         100,
		"reservation_id": "idem-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("idempotent reserve: status %d, want 201", status)
	}
	if body2["id"] != id1 {
		t.Fatalf("idempotent reserve returned different id: %v vs %v", body2["id"], id1)
	}

	// Usage must show only 100 reserved (not 200).
	status, body := apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/usage?key=api_calls", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("usage: status %d", status)
	}
	if int64(body["reserved"].(float64)) != 100 {
		t.Fatalf("reserved = %v, want 100 (idempotent)", body["reserved"])
	}
}

func TestQuotaExpiry(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-exp")

	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})

	// Reserve with 1-second expiry.
	status, body := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":          "api_calls",
		"amount":             200,
		"reservation_id":     "res-expire",
		"expires_in_seconds": 1,
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve with expiry: status %d", status)
	}

	// Verify usage shows 200 reserved.
	status, body = apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/usage?key=api_calls", apiKey, nil)
	if int64(body["reserved"].(float64)) != 200 {
		t.Fatalf("reserved = %v, want 200", body["reserved"])
	}

	// Wait for the 1-second expiry to elapse.
	time.Sleep(2 * time.Second)

	// Run the expiry sweeper to reclaim past-due reservations.
	n, err := svc.RecoverExpiredReservations(testCtx)
	if err != nil {
		t.Fatalf("recover expired: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least 1 expired reservation, got 0")
	}

	// Reserve the full 1000 again — should succeed if the expired 200 was reclaimed.
	status, body = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         1000,
		"reservation_id": "res-after-expiry",
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve after expiry: status %d, body %v — expired reservation may not have been reclaimed yet", status, body)
	}
}

func TestQuotaCrossTenantIsolation(t *testing.T) {
	apiKeyA, subIDA := setupQuotaTestProvider(t, "qta-iso-a")
	apiKeyB, subIDB := setupQuotaTestProvider(t, "qta-iso-b")

	// Provider A sets a quota limit.
	apiReq(t, "PUT", "/v1/subscriptions/"+subIDA+"/quota-limits/api_calls", apiKeyA, map[string]any{
		"limit_value": 500,
		"period_type": "monthly",
	})

	// Provider B cannot see A's quota limits.
	status, body := apiReq(t, "GET", "/v1/subscriptions/"+subIDB+"/quota-limits", apiKeyB, nil)
	if status != http.StatusOK {
		t.Fatalf("provider B list: status %d", status)
	}
	limits := body["quota_limits"].([]any)
	if len(limits) != 0 {
		t.Fatalf("provider B sees %d limits, want 0 (RLS leak)", len(limits))
	}

	// Provider B cannot get A's quota limit by guessing the key.
	status, _ = apiReq(t, "GET", "/v1/subscriptions/"+subIDB+"/quota-limits/api_calls", apiKeyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("provider B get A's key: status %d, want 404", status)
	}

	// Provider B cannot reserve against A's subscription.
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subIDA+"/quota/reserve", apiKeyB, map[string]any{
		"quota_key":      "api_calls",
		"amount":         100,
		"reservation_id": "cross-tenant-evil",
	})
	if status != http.StatusNotFound {
		t.Fatalf("provider B reserve on A's sub: status %d, want 404", status)
	}
}

func TestQuotaConcurrentReserve(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-ccr")

	// Set limit of 1000.
	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})

	// 10 concurrent reservations of 100 each = total 1000 (exactly at limit).
	// All should succeed because 10*100 = 1000 <= 1000.
	var wg sync.WaitGroup
	results := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			status, _ := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
				"quota_key":      "api_calls",
				"amount":         100,
				"reservation_id": "conc-" + uuid.NewString()[:8],
			})
			results <- status
		}(i)
	}
	wg.Wait()
	close(results)

	successCount := 0
	for status := range results {
		if status == http.StatusCreated {
			successCount++
		}
	}
	if successCount != 10 {
		t.Fatalf("expected 10 successful concurrent reserves, got %d", successCount)
	}

	// Verify total reserved = 1000 (at limit).
	status, body := apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/usage?key=api_calls", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("usage: status %d", status)
	}
	if int64(body["reserved"].(float64)) != 1000 {
		t.Fatalf("reserved = %v, want 1000", body["reserved"])
	}

	// One more reserve of 1 must fail (1000+1 > 1000).
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         1,
		"reservation_id": "conc-overflow",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("overflow reserve: status %d, want 422", status)
	}
}

func TestQuotaValidationErrors(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-val")

	// Invalid period_type.
	status, _ := apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 100,
		"period_type": "hourly",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid period: status %d, want 400", status)
	}

	// Negative limit.
	status, _ = apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": -1,
		"period_type": "monthly",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("negative limit: status %d, want 400", status)
	}

	// Reserve with zero amount.
	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 100,
		"period_type": "monthly",
	})
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         0,
		"reservation_id": "zero",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("zero amount: status %d, want 400", status)
	}

	// Reserve without reservation_id.
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key": "api_calls",
		"amount":    10,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing reservation_id: status %d, want 400", status)
	}
}

func TestQuotaCommitNotReserved(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-cnr")

	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})

	// Reserve and commit.
	status, body := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         100,
		"reservation_id": "res-commit",
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve: status %d", status)
	}
	resID := body["id"].(string)

	// Commit.
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/commit", apiKey, map[string]any{
		"reservation_id": resID,
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status %d", status)
	}

	// Double commit — conflict.
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/commit", apiKey, map[string]any{
		"reservation_id": resID,
	})
	if status != http.StatusConflict {
		t.Fatalf("double commit: status %d, want 409", status)
	}

	// Release a committed reservation — conflict (can only release reserved).
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/release", apiKey, map[string]any{
		"reservation_id": resID,
	})
	if status != http.StatusConflict {
		t.Fatalf("release committed: status %d, want 409", status)
	}
}

func TestQuotaDeleteLimit(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-del")

	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 100,
		"period_type": "monthly",
	})

	// Delete the limit.
	status, _ := apiReq(t, "DELETE", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete limit: status %d, want 204", status)
	}

	// Get deleted limit — not found.
	status, _ = apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted limit: status %d, want 404", status)
	}

	// Reserve without a limit — fails (no limit = CTE returns no rows).
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         10,
		"reservation_id": "after-delete",
	})
	// Without a limit row, the CTE's WHERE clause fails (no limit_value),
	// so the INSERT returns no rows → ErrQuotaExceeded or ErrNotFound.
	if status != http.StatusUnprocessableEntity && status != http.StatusNotFound {
		t.Fatalf("reserve without limit: status %d, want 422 or 404", status)
	}
}

func TestQuotaListReservations(t *testing.T) {
	apiKey, subID := setupQuotaTestProvider(t, "qta-lst")

	apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})

	// Create two reservations.
	for i := 0; i < 2; i++ {
		status, _ := apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
			"quota_key":      "api_calls",
			"amount":         100,
			"reservation_id": "lst-" + uuid.NewString()[:8],
		})
		if status != http.StatusCreated {
			t.Fatalf("reserve %d: status %d", i, status)
		}
	}

	// List active reservations.
	status, body := apiReq(t, "GET", "/v1/subscriptions/"+subID+"/quota/reservations", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list reservations: status %d", status)
	}
	reservations := body["quota_reservations"].([]any)
	if len(reservations) != 2 {
		t.Fatalf("expected 2 reservations, got %d", len(reservations))
	}
}

func TestOperatorQuotaControlPlane(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "opq-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	_, subID := createCustomerAndSubscription(t, apiKey, versionID)
	base := "/v1/operator/providers/" + providerID + "/subscriptions/" + subID

	// Provider configures a hard limit and reserves quota through the API.
	status, body := apiReq(t, "PUT", "/v1/subscriptions/"+subID+"/quota-limits/api_calls", apiKey, map[string]any{
		"limit_value": 1000,
		"period_type": "monthly",
	})
	if status != http.StatusOK {
		t.Fatalf("set limit: status %d, body %v", status, body)
	}
	status, _ = apiReq(t, "POST", "/v1/subscriptions/"+subID+"/quota/reserve", apiKey, map[string]any{
		"quota_key":      "api_calls",
		"amount":         300,
		"reservation_id": "opq-res-1",
	})
	if status != http.StatusCreated {
		t.Fatalf("reserve: status %d", status)
	}

	// Operator overview returns the limit with live usage in one request.
	status, body = apiReq(t, "GET", base+"/quota?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("operator quota overview: status %d, body %v", status, body)
	}
	limits := body["quota_limits"].([]any)
	if len(limits) != 1 {
		t.Fatalf("quota limits = %d, want 1; body %v", len(limits), body)
	}
	limit := limits[0].(map[string]any)
	if limit["quota_key"] != "api_calls" {
		t.Fatalf("quota_key = %v, want api_calls", limit["quota_key"])
	}
	if int64(limit["limit_value"].(float64)) != 1000 ||
		int64(limit["committed"].(float64)) != 0 ||
		int64(limit["reserved"].(float64)) != 300 {
		t.Fatalf("usage = %v, want limit=1000 committed=0 reserved=300", limit)
	}

	// Operator can see the reservation ledger.
	status, body = apiReq(t, "GET", base+"/quota/reservations?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("operator reservations: status %d, body %v", status, body)
	}
	reservations := body["quota_reservations"].([]any)
	if len(reservations) != 1 {
		t.Fatalf("reservations = %d, want 1", len(reservations))
	}

	// Operator can update the limit and delete it.
	status, body = apiReq(t, "PUT", base+"/quota-limits/api_calls?env=test", operatorToken, map[string]any{
		"limit_value": 2000,
		"period_type": "monthly",
	})
	if status != http.StatusOK || int64(body["limit_value"].(float64)) != 2000 {
		t.Fatalf("operator update limit: status %d, body %v", status, body)
	}
	status, _ = apiReq(t, "DELETE", base+"/quota-limits/api_calls?env=test", operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("operator delete limit: status %d, want 204", status)
	}
}
