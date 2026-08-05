package integration

import (
	"net/http"
	"strings"
	"testing"
)

// TestBodySizeLimit verifies that requests exceeding the 1 MB body size
// limit are rejected. This prevents DoS via large payload submissions.
func TestBodySizeLimit(t *testing.T) {
	// Build a JSON body slightly over 1 MB.
	largeSlug := strings.Repeat("a", 1<<20) // 1 MB
	status, _ := apiReq(t, "POST", "/v1/operator/providers", operatorToken, map[string]any{
		"slug":         largeSlug,
		"display_name": "DoS attempt",
		"issuer":       "https://example.com",
		"base_domain":  "example.com",
	})
	// MaxBytesReader causes the body read to fail; decodeJSON returns 400.
	if status != http.StatusBadRequest {
		t.Fatalf("large body: status %d, want 400", status)
	}
}

// TestServerSurvivesEdgeCaseInputs verifies that the server stays alive
// after receiving edge-case inputs that could panic unguarded handlers.
// The actual panic→500 mapping is covered by the unit test
// TestRecoverMiddleware in package httpapi; here we only verify the
// server doesn't crash (subsequent requests still succeed).
func TestServerSurvivesEdgeCaseInputs(t *testing.T) {
	_, apiKey := createProviderAPI(t, "edge-"+strings.Repeat("x", 4))

	// Edge case: invalid UUID in a SCIM path that expects a UUID.
	// parseUUIDParam returns 400, not a panic — but this confirms the
	// guard works and the server stays alive.
	status, _ := apiReq(t, "GET", "/scim/v2/Users/not-a-uuid", apiKey, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid UUID: status %d, want 400", status)
	}

	// Server must still be alive after the edge-case request.
	for i := 0; i < 5; i++ {
		status, _ := apiReq(t, "GET", "/v1/catalog/versions", apiKey, nil)
		if status != http.StatusOK {
			t.Fatalf("request %d after edge case: status %d, want 200 (server should be alive)", i, status)
		}
	}
}
