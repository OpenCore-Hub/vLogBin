package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorAnalyticsDashboard verifies the Console Analytics dashboard
// endpoint resolves the environment and returns the combined empty payload.
func TestOperatorAnalyticsDashboard(t *testing.T) {
	providerID, _ := createProviderAPI(t, "op-analytics-"+uuid.NewString()[:8])

	status, body := apiReq(
		t,
		http.MethodGet,
		"/v1/operator/providers/"+providerID+"/analytics/dashboard?env=test",
		operatorToken,
		nil,
	)
	if status != http.StatusOK {
		t.Fatalf("dashboard: status %d, body %v", status, body)
	}
	for _, key := range []string{"revenue", "mau", "conversion", "churn", "anomalies", "generated_at"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("dashboard missing %q: %v", key, body)
		}
	}

	if status, body := apiReq(
		t,
		http.MethodGet,
		"/v1/operator/providers/"+providerID+"/analytics/dashboard",
		operatorToken,
		nil,
	); status != http.StatusBadRequest {
		t.Fatalf("missing env: status %d, body %v", status, body)
	}
}
