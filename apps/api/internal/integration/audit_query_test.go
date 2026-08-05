package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// injectAuditEvents inserts environment-less audit events with explicit,
// spaced timestamps (base + i minutes) so page order and time-range filters
// are deterministic. All events share actor_type=credential, actor_id=svc-a
// and use the provider as target.
func injectAuditEvents(t *testing.T, providerID uuid.UUID, base time.Time, actions []string) {
	t.Helper()
	for i, action := range actions {
		_, err := superPool.Exec(testCtx,
			`INSERT INTO audit_events (provider_id, actor_type, actor_id, action, target_type, target_id, created_at)
			 VALUES ($1, 'credential', 'svc-a', $2, 'provider', $3, $4)`,
			providerID, action, providerID, base.Add(time.Duration(i)*time.Minute))
		if err != nil {
			t.Fatalf("inject audit event %s: %v", action, err)
		}
	}
}

// createAuditKey mints a child key holding only audit:read from a provider's
// base key, matching the least-privilege scope the audit endpoints require.
func createAuditKey(t *testing.T, baseKey string) string {
	t.Helper()
	status, body := apiReq(t, "POST", "/v1/credentials", baseKey, map[string]any{
		"name": "auditor", "scopes": []string{"audit:read"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create audit key: status %d, body %v", status, body)
	}
	key, _ := body["api_key"].(string)
	if key == "" {
		t.Fatal("audit key empty")
	}
	return key
}

// auditActions extracts the "action" fields of a decoded audit-events
// response in payload order.
func auditActions(body map[string]any) []string {
	raw, _ := body["audit_events"].([]any)
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		out = append(out, e.(map[string]any)["action"].(string))
	}
	return out
}

// TestAuditQueryPagination exercises keyset pagination on the provider
// audit endpoint: fixed page size, newest-first order, no duplicates or
// gaps, and a null next_cursor on the final page.
func TestAuditQueryPagination(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "aq-page-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	// Inject into the future so the injected rows are guaranteed to be the
	// newest ones and the first pages are deterministic.
	base := time.Now().UTC().Add(time.Hour)
	injectAuditEvents(t, uuid.MustParse(providerID), base,
		[]string{"qa.one", "qa.two", "qa.three", "qa.four", "qa.five"})

	var got []string
	cursor := ""
	for page := 0; page < 10; page++ {
		path := "/v1/audit-events?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		status, body := apiReq(t, "GET", path, apiKey, nil)
		if status != http.StatusOK {
			t.Fatalf("page %d: status %d, body %v", page, status, body)
		}
		events := body["audit_events"].([]any)
		if len(events) > 2 {
			t.Fatalf("page %d returned %d events, want <= 2", page, len(events))
		}
		got = append(got, auditActions(body)...)
		if len(events) == 0 {
			t.Fatal("empty page before exhausting the injected rows")
		}
		nc, ok := body["next_cursor"].(float64)
		if !ok {
			break // last page
		}
		cursor = fmt.Sprintf("%d", int64(nc))
	}

	want := []string{"qa.five", "qa.four", "qa.three", "qa.two", "qa.one"}
	var injected []string
	for _, a := range got {
		if len(a) > 3 && a[:3] == "qa." {
			injected = append(injected, a)
		}
	}
	if fmt.Sprint(injected) != fmt.Sprint(want) {
		t.Fatalf("paged order = %v, want %v (full page stream %v)", injected, want, got)
	}
}

// TestAuditQueryFilters verifies action/actor/time filters on the provider
// audit endpoint, both alone and combined.
func TestAuditQueryFilters(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "aq-filt-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	base := time.Now().UTC().Add(time.Hour)
	injectAuditEvents(t, uuid.MustParse(providerID), base,
		[]string{"qa.credential.created", "qa.credential.revoked", "qa.region.assigned"})

	// One extra event from a different actor type.
	_, err := superPool.Exec(testCtx,
		`INSERT INTO audit_events (provider_id, actor_type, actor_id, action, created_at)
		 VALUES ($1, 'operator', 'admin-1', 'qa.operator.override', $2)`,
		providerID, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("inject operator event: %v", err)
	}

	// Action filter.
	status, body := apiReq(t, "GET", "/v1/audit-events?action=qa.credential.created", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("action filter: status %d, body %v", status, body)
	}
	if got := auditActions(body); fmt.Sprint(got) != fmt.Sprint([]string{"qa.credential.created"}) {
		t.Fatalf("action filter returned %v", got)
	}

	// Actor type filter: both our injected operator event and the provider's
	// own activation events share actor_type=operator, so only the injected
	// qa.* rows must come back.
	status, body = apiReq(t, "GET", "/v1/audit-events?actor_type=operator", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("actor filter: status %d, body %v", status, body)
	}
	var qaActors []string
	for _, a := range auditActions(body) {
		if len(a) > 3 && a[:3] == "qa." {
			qaActors = append(qaActors, a)
		}
	}
	if fmt.Sprint(qaActors) != fmt.Sprint([]string{"qa.operator.override"}) {
		t.Fatalf("actor filter qa rows = %v", qaActors)
	}

	// Time window [base, base+1m] excludes the base+2m and base+3m events.
	from := base.Add(-time.Second).Format(time.RFC3339)
	to := base.Add(time.Minute).Format(time.RFC3339)
	status, body = apiReq(t, "GET", "/v1/audit-events?from="+from+"&to="+to, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("time filter: status %d, body %v", status, body)
	}
	if got := auditActions(body); fmt.Sprint(got) != fmt.Sprint([]string{"qa.credential.created"}) {
		t.Fatalf("time filter returned %v", got)
	}

	// Combined: action + from/to returns exactly the matching row.
	status, body = apiReq(t, "GET",
		"/v1/audit-events?action=qa.region.assigned&from="+from+"&to="+base.Add(3*time.Minute).Format(time.RFC3339),
		apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("combined filter: status %d, body %v", status, body)
	}
	if got := auditActions(body); fmt.Sprint(got) != fmt.Sprint([]string{"qa.region.assigned"}) {
		t.Fatalf("combined filter returned %v", got)
	}
}

// TestAuditQueryTenantIsolation verifies a provider cannot see another
// provider's audit events and an operator can, across the same query.
func TestAuditQueryTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "aq-iso-a-"+uuid.NewString()[:8])
	keyA = createAuditKey(t, keyA)
	idB, keyB := createProviderAPI(t, "aq-iso-b-"+uuid.NewString()[:8])
	keyB = createAuditKey(t, keyB)
	injectAuditEvents(t, uuid.MustParse(idB), time.Now().UTC().Add(time.Hour),
		[]string{"qa.secret.of.b"})

	// A cannot see B's events even with an explicit action filter.
	status, body := apiReq(t, "GET", "/v1/audit-events?action=qa.secret.of.b", keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("isolation query: status %d, body %v", status, body)
	}
	if got := auditActions(body); len(got) != 0 {
		t.Fatalf("provider A leaked B's events: %v", got)
	}

	// B can see its own event; the operator can too, and pagination works on
	// the operator endpoint with a page size of 1.
	status, body = apiReq(t, "GET", "/v1/audit-events?action=qa.secret.of.b", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("owner query: status %d, body %v", status, body)
	}
	if got := auditActions(body); fmt.Sprint(got) != fmt.Sprint([]string{"qa.secret.of.b"}) {
		t.Fatalf("owner query returned %v", got)
	}

	status, body = apiReq(t, "GET", "/v1/operator/providers/"+idB+"/audit?action=qa.secret.of.b&limit=1", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("operator query: status %d, body %v", status, body)
	}
	if got := auditActions(body); fmt.Sprint(got) != fmt.Sprint([]string{"qa.secret.of.b"}) {
		t.Fatalf("operator query returned %v", got)
	}
	if _, ok := body["next_cursor"].(float64); ok {
		t.Fatalf("single-row operator page must have a null next_cursor, got %v", body["next_cursor"])
	}
}

// TestAuditQueryParameterValidation verifies malformed query parameters are
// rejected with 400 and a specific error code.
func TestAuditQueryParameterValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "aq-param-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	cases := []struct {
		path, code string
	}{
		{"/v1/audit-events?cursor=abc", "invalid_cursor"},
		{"/v1/audit-events?cursor=-5", "invalid_cursor"},
		{"/v1/audit-events?from=notatime", "invalid_from"},
		{"/v1/audit-events?to=also-bad", "invalid_to"},
	}
	for _, c := range cases {
		status, body := apiReq(t, "GET", c.path, apiKey, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", c.path, status)
		}
		if errObj := body["error"].(map[string]any); errObj["code"] != c.code {
			t.Fatalf("%s: error code = %v, want %s", c.path, errObj["code"], c.code)
		}
	}
}
