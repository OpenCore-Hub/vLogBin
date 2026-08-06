package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestRateLimiting verifies that the per-endpoint rate limit (60/min)
// rejects the 61st request to the same endpoint with 429.
func TestRateLimiting(t *testing.T) {
	_, apiKey := createProviderAPI(t, "rl-"+uuid.NewString()[:8])

	// The per-endpoint limit is 60 requests per minute. Make 65 requests
	// to the same endpoint and verify the first 60 succeed and 61-65
	// return 429.
	for i := 0; i < 65; i++ {
		status, body := apiReq(t, "GET", "/v1/catalog/versions", apiKey, nil)
		if i < 60 {
			if status != http.StatusOK {
				t.Fatalf("request %d: status %d, want 200, body %v", i+1, status, body)
			}
		} else {
			if status != http.StatusTooManyRequests {
				t.Fatalf("request %d: status %d, want 429 (rate limited)", i+1, status)
			}
			// Verify error response includes request_id and rate_limited code.
			if errObj, ok := body["error"].(map[string]any); ok {
				if errObj["code"] != "rate_limited" {
					t.Fatalf("error code = %v, want rate_limited", errObj["code"])
				}
				if errObj["request_id"] == nil || errObj["request_id"] == "" {
					t.Fatal("rate_limited error should include request_id")
				}
				if errObj["retry_after"] == nil || errObj["retry_after"] == "" {
					t.Fatal("rate_limited error should include retry_after")
				}
			}
		}
	}
}
