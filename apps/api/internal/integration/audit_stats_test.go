package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// injectAuditEventAt inserts a single audit event at an exact timestamp with
// explicit action and actor type, for deterministic dashboard assertions.
func injectAuditEventAt(t *testing.T, providerID uuid.UUID, at time.Time, actorType, actorID, action string) {
	t.Helper()
	_, err := superPool.Exec(testCtx,
		`INSERT INTO audit_events (provider_id, actor_type, actor_id, action, target_type, target_id, created_at)
		 VALUES ($1, $2, $3, $4, 'provider', $5, $6)`,
		providerID, actorType, actorID, action, providerID, at)
	if err != nil {
		t.Fatalf("inject audit event %s: %v", action, err)
	}
}

// countOf returns the count for a key in a decoded stats breakdown slice, or
// -1 when the key is absent (so absent != zero is distinguishable).
func countOf(slice []any, key string) int64 {
	for _, c := range slice {
		m := c.(map[string]any)
		if m["key"] == key {
			if n, ok := m["count"].(float64); ok {
				return int64(n)
			}
		}
	}
	return -1
}

// TestAuditStatsAggregates verifies the headline total and the by_action /
// by_actor_type breakdowns against a known dataset. The window is pinned to
// past timestamps so provider-activation noise from the test run cannot leak
// in.
func TestAuditStatsAggregates(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "ast-agg-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	pid := uuid.MustParse(providerID)
	injectAuditEventAt(t, pid, base, "credential", "svc-a", "qa.credential.created")
	injectAuditEventAt(t, pid, base.Add(time.Hour), "credential", "svc-a", "qa.credential.revoked")
	injectAuditEventAt(t, pid, base.Add(2*time.Hour), "operator", "admin-1", "qa.region.assigned")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(3 * time.Hour).Format(time.RFC3339)
	status, body := apiReq(t, "GET", "/v1/audit-events/stats?from="+from+"&to="+to, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("stats: status %d, body %v", status, body)
	}
	if got := int64(body["total"].(float64)); got != 3 {
		t.Fatalf("total = %d, want 3", got)
	}
	byAction := body["by_action"].([]any)
	if got := countOf(byAction, "qa.credential.created"); got != 1 {
		t.Fatalf("by_action credential.created = %d, want 1", got)
	}
	if got := countOf(byAction, "qa.region.assigned"); got != 1 {
		t.Fatalf("by_action region.assigned = %d, want 1", got)
	}
	byActor := body["by_actor_type"].([]any)
	if got := countOf(byActor, "credential"); got != 2 {
		t.Fatalf("by_actor_type credential = %d, want 2", got)
	}
	if got := countOf(byActor, "operator"); got != 1 {
		t.Fatalf("by_actor_type operator = %d, want 1", got)
	}
}

// TestAuditStatsSeriesZeroFill verifies the time series renders a contiguous
// axis: buckets with no events are zero-filled between from and to, and
// bucket labels align to UTC midnight (day) / UTC hour (hour).
func TestAuditStatsSeriesZeroFill(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "ast-srs-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	pid := uuid.MustParse(providerID)
	day := func(d, h int) time.Time { return time.Date(2026, 2, d, h, 0, 0, 0, time.UTC) }
	injectAuditEventAt(t, pid, day(1, 8), "credential", "svc-a", "qa.one")
	injectAuditEventAt(t, pid, day(1, 20), "credential", "svc-a", "qa.two")
	injectAuditEventAt(t, pid, day(2, 9), "credential", "svc-a", "qa.three")
	injectAuditEventAt(t, pid, day(4, 7), "credential", "svc-a", "qa.four") // day 3 empty

	// Day granularity across four days with day 3 empty.
	from := day(1, 0).Format(time.RFC3339)
	to := day(4, 23).Format(time.RFC3339)
	status, body := apiReq(t, "GET", "/v1/audit-events/stats?from="+from+"&to="+to+"&granularity=day", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("day series: status %d, body %v", status, body)
	}
	series := body["series"].([]any)
	if len(series) != 4 {
		t.Fatalf("day series has %d buckets, want 4: %v", len(series), series)
	}
	for i, w := range []int64{2, 1, 0, 1} {
		if got := int64(series[i].(map[string]any)["count"].(float64)); got != w {
			t.Fatalf("day bucket %d count = %d, want %d", i, got, w)
		}
	}
	if got := series[0].(map[string]any)["bucket"].(string); got != day(1, 0).Format(time.RFC3339) {
		t.Fatalf("first bucket = %s, want %s", got, day(1, 0).Format(time.RFC3339))
	}

	// Hour granularity: 08:00 holds qa.one, 09:00 and 10:00 are empty.
	status, body = apiReq(t, "GET",
		"/v1/audit-events/stats?from="+day(1, 8).Format(time.RFC3339)+"&to="+day(1, 10).Format(time.RFC3339)+"&granularity=hour",
		apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("hour series: status %d, body %v", status, body)
	}
	series = body["series"].([]any)
	if len(series) != 3 {
		t.Fatalf("hour series has %d buckets, want 3: %v", len(series), series)
	}
	for i, w := range []int64{1, 0, 0} {
		if got := int64(series[i].(map[string]any)["count"].(float64)); got != w {
			t.Fatalf("hour bucket %d count = %d, want %d", i, got, w)
		}
	}
	if got := series[0].(map[string]any)["bucket"].(string); got != day(1, 8).Format(time.RFC3339) {
		t.Fatalf("first hour bucket = %s, want %s", got, day(1, 8).Format(time.RFC3339))
	}
}

// TestAuditStatsFilters verifies the shared filter set applies to the
// aggregates: filtering by action narrows both the total and the breakdown.
func TestAuditStatsFilters(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "ast-flt-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	pid := uuid.MustParse(providerID)
	base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	injectAuditEventAt(t, pid, base, "credential", "svc-a", "qa.credential.created")
	injectAuditEventAt(t, pid, base.Add(time.Hour), "credential", "svc-a", "qa.credential.revoked")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(2 * time.Hour).Format(time.RFC3339)
	status, body := apiReq(t, "GET",
		"/v1/audit-events/stats?from="+from+"&to="+to+"&action=qa.credential.revoked", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("filtered stats: status %d, body %v", status, body)
	}
	if got := int64(body["total"].(float64)); got != 1 {
		t.Fatalf("filtered total = %d, want 1", got)
	}
	if got := len(body["by_action"].([]any)); got != 1 {
		t.Fatalf("filtered by_action has %d entries, want 1", got)
	}
}

// TestAuditStatsParameterValidation verifies the dashboard-specific guards:
// from/to are required, the range is bounded, and granularity is an enum.
func TestAuditStatsParameterValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "ast-prm-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	now := time.Now().UTC()
	from := now.Format(time.RFC3339)
	to := now.Add(time.Hour).Format(time.RFC3339)
	cases := []struct{ path, code string }{
		{"/v1/audit-events/stats?to=" + to, "missing_from"},
		{"/v1/audit-events/stats?from=" + from, "missing_to"},
		{"/v1/audit-events/stats?from=" + to + "&to=" + from, "invalid_range"},
		{"/v1/audit-events/stats?from=" + from + "&to=" + to + "&granularity=month", "invalid_granularity"},
		{"/v1/audit-events/stats?from=" + from + "&to=" + now.Add(400*24*time.Hour).Format(time.RFC3339), "range_too_wide"},
	}
	for _, c := range cases {
		status, body := apiReq(t, "GET", c.path, apiKey, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400 (body %v)", c.path, status, body)
		}
		if errObj := body["error"].(map[string]any); errObj["code"] != c.code {
			t.Fatalf("%s: error code = %v, want %s", c.path, errObj["code"], c.code)
		}
	}
}

// TestAuditStatsTenantIsolation verifies a provider's aggregates only cover
// its own events while the operator view spans all environments.
func TestAuditStatsTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "ast-iso-a-"+uuid.NewString()[:8])
	keyA = createAuditKey(t, keyA)
	idB, keyB := createProviderAPI(t, "ast-iso-b-"+uuid.NewString()[:8])
	keyB = createAuditKey(t, keyB)
	pidB := uuid.MustParse(idB)
	base := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	injectAuditEventAt(t, pidB, base, "credential", "svc-b", "qa.secret.of.b")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(time.Hour).Format(time.RFC3339)

	// A must see zero events from B.
	status, body := apiReq(t, "GET", "/v1/audit-events/stats?from="+from+"&to="+to, keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("isolation stats: status %d, body %v", status, body)
	}
	if got := int64(body["total"].(float64)); got != 0 {
		t.Fatalf("provider A total = %d, want 0", got)
	}

	// B sees its own event.
	status, body = apiReq(t, "GET", "/v1/audit-events/stats?from="+from+"&to="+to, keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("owner stats: status %d, body %v", status, body)
	}
	if got := int64(body["total"].(float64)); got != 1 {
		t.Fatalf("provider B total = %d, want 1", got)
	}

	// The operator sees B's event across environments too.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+idB+"/audit/stats?from="+from+"&to="+to, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("operator stats: status %d, body %v", status, body)
	}
	if got := int64(body["total"].(float64)); got != 1 {
		t.Fatalf("operator total = %d, want 1", got)
	}
}

// TestAuditStatsRequiresScope verifies the stats endpoint honors the
// audit:read scope like the list endpoint does. The base provider key carries
// the full scope set, so we mint a read-only child key and expect 403.
func TestAuditStatsRequiresScope(t *testing.T) {
	_, baseKey := createProviderAPI(t, "ast-scope-"+uuid.NewString()[:8])
	status, body := apiReq(t, "POST", "/v1/credentials", baseKey, map[string]any{
		"name": "read-only", "scopes": []string{"read"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create read-only key: status %d, body %v", status, body)
	}
	readOnlyKey := body["api_key"].(string)
	now := time.Now().UTC()
	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)
	status, body = apiReq(t, "GET", "/v1/audit-events/stats?from="+from+"&to="+to, readOnlyKey, nil)
	if status != http.StatusForbidden {
		t.Fatalf("stats without audit:read: status %d, want 403 (body %v)", status, body)
	}
}

// TestAuditStatsConsistency cross-checks the aggregates against the list
// endpoint: total must equal the rows the matching list query returns, and
// the series counts must sum to the total.
func TestAuditStatsConsistency(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "ast-cns-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	pid := uuid.MustParse(providerID)
	base := time.Date(2026, 5, 1, 6, 0, 0, 0, time.UTC)
	injectAuditEventAt(t, pid, base, "credential", "svc-a", "qa.one")
	injectAuditEventAt(t, pid, base.Add(time.Hour), "credential", "svc-a", "qa.two")
	injectAuditEventAt(t, pid, base.Add(2*time.Hour), "operator", "admin-1", "qa.three")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(3 * time.Hour).Format(time.RFC3339)

	status, body := apiReq(t, "GET", "/v1/audit-events?limit=100&from="+from+"&to="+to, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	want := int64(len(body["audit_events"].([]any)))

	status, body = apiReq(t, "GET", "/v1/audit-events/stats?from="+from+"&to="+to, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("stats: status %d, body %v", status, body)
	}
	if got := int64(body["total"].(float64)); got != want {
		t.Fatalf("total = %d, want list count %d", got, want)
	}
	sum := int64(0)
	for _, p := range body["series"].([]any) {
		sum += int64(p.(map[string]any)["count"].(float64))
	}
	if sum != want {
		t.Fatalf("series sum = %d, want %d", sum, want)
	}
}
