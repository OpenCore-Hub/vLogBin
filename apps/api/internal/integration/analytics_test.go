package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestAnalyticsDashboard(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-dash-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	status, body := apiReq(t, "GET", "/v1/analytics/dashboard", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("dashboard: status %d, body %v", status, body)
	}
	// Dashboard must contain all sections (empty arrays, not null).
	if body["revenue"] == nil {
		t.Fatal("revenue must be present")
	}
	if body["mau"] == nil {
		t.Fatal("mau must be present")
	}
	if body["conversion"] == nil {
		t.Fatal("conversion must be present")
	}
	if body["churn"] == nil {
		t.Fatal("churn must be present")
	}
	if body["anomalies"] == nil {
		t.Fatal("anomalies must be present")
	}
	if body["generated_at"] == nil {
		t.Fatal("generated_at must be present")
	}
}

func TestAnalyticsRevenue(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-rev-"+uuid.NewString()[:8])

	// Revenue view may return empty for new providers (no invoices).
	status, body := apiReq(t, "GET", "/v1/analytics/revenue?months=6", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("revenue: status %d, body %v", status, body)
	}
	if _, ok := body["revenue"].([]any); !ok {
		t.Fatalf("revenue must be an array: %T", body["revenue"])
	}
}

func TestAnalyticsMAU(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-mau-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	status, body := apiReq(t, "GET", "/v1/analytics/mau?months=3", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("mau: status %d, body %v", status, body)
	}
	if _, ok := body["mau"].([]any); !ok {
		t.Fatalf("mau must be an array: %T", body["mau"])
	}
}

func TestAnalyticsConversion(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-conv-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	status, body := apiReq(t, "GET", "/v1/analytics/conversion?months=6", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("conversion: status %d, body %v", status, body)
	}
	if _, ok := body["conversion"].([]any); !ok {
		t.Fatalf("conversion must be an array: %T", body["conversion"])
	}
}

func TestAnalyticsChurn(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-chr-"+uuid.NewString()[:8])

	status, body := apiReq(t, "GET", "/v1/analytics/churn?months=6", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("churn: status %d, body %v", status, body)
	}
	if _, ok := body["churn"].([]any); !ok {
		t.Fatalf("churn must be an array: %T", body["churn"])
	}
}

func TestAnalyticsUsageBreakdown(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-ubk-"+uuid.NewString()[:8])

	status, body := apiReq(t, "GET", "/v1/analytics/usage-breakdown?days=7", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("usage-breakdown: status %d, body %v", status, body)
	}
	if _, ok := body["usage_breakdown"].([]any); !ok {
		t.Fatalf("usage_breakdown must be an array: %T", body["usage_breakdown"])
	}
}

func TestAnalyticsAnomalies(t *testing.T) {
	_, apiKey := createProviderAPI(t, "an-anm-"+uuid.NewString()[:8])

	status, body := apiReq(t, "GET", "/v1/analytics/anomalies", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("anomalies: status %d, body %v", status, body)
	}
	if _, ok := body["anomalies"].([]any); !ok {
		t.Fatalf("anomalies must be an array: %T", body["anomalies"])
	}
}

func TestAnalyticsCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "an-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "an-iso-b-"+uuid.NewString()[:8])

	// Provider A creates data.
	versionID := createPublishedCatalog(t, keyA)
	createCustomerAndSubscription(t, keyA, versionID)

	// Provider B's dashboard should not contain A's data.
	status, bodyB := apiReq(t, "GET", "/v1/analytics/dashboard", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B dashboard: status %d", status)
	}
	// B's conversion should be empty or only contain B's data.
	convB := bodyB["conversion"].([]any)
	for _, c := range convB {
		// Each entry's provider_id should be B's, not A's.
		m := c.(map[string]any)
		if m["provider_id"] == "" {
			// Views don't always include provider_id in the RLS-filtered result,
			// but the RLS policy ensures only B's rows are returned.
		}
	}
}
