package integration

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// apiRawReq performs a request against the test server and returns the status
// plus the raw response body and headers. Export endpoints return CSV or a bare
// JSON array, neither of which apiReq (JSON-object only) can decode.
func apiRawReq(t *testing.T, method, path, token string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, httpServer.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, raw
}

// injectAuditEventDetailed inserts an audit event with optional metadata and
// request ID, mirroring what real request handlers record.
func injectAuditEventDetailed(t *testing.T, providerID uuid.UUID, at time.Time, actorType, actorID, action string, metadata map[string]any, requestID string) {
	t.Helper()
	meta := "{}"
	if metadata != nil {
		raw, err := json.Marshal(metadata)
		if err != nil {
			t.Fatalf("marshal metadata: %v", err)
		}
		meta = string(raw)
	}
	_, err := superPool.Exec(testCtx,
		`INSERT INTO audit_events (provider_id, actor_type, actor_id, action, target_type, target_id, metadata, request_id, created_at)
		 VALUES ($1, $2, $3, $4, 'provider', $5, $6::jsonb, $7, $8)`,
		providerID, actorType, actorID, action, providerID, meta, requestID, at)
	if err != nil {
		t.Fatalf("inject audit event %s: %v", action, err)
	}
}

// csvRecords parses a CSV export body into records, asserting the Content-Type
// and download disposition headers.
func csvRecords(t *testing.T, raw []byte, header http.Header) [][]string {
	t.Helper()
	if ct := header.Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	if cd := header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="audit-export-`) || !strings.HasSuffix(cd, `.csv"`) {
		t.Fatalf("Content-Disposition = %q, want audit-export attachment", cd)
	}
	records, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v (body %q)", err, raw)
	}
	return records
}

// TestAuditExportCSV verifies the CSV stream: fixed documented header, one row
// per event in newest-first order, metadata embedded verbatim, and download
// response headers.
func TestAuditExportCSV(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "aex-csv-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	pid := uuid.MustParse(providerID)
	base := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	injectAuditEventAt(t, pid, base, "credential", "svc-a", "qa.one")
	injectAuditEventDetailed(t, pid, base.Add(time.Hour), "operator", "admin-1", "qa.two",
		map[string]any{"reason": "qa"}, "req-123")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(2 * time.Hour).Format(time.RFC3339)
	status, header, raw := apiRawReq(t, "GET", "/v1/audit-events/export?from="+from+"&to="+to, apiKey)
	if status != http.StatusOK {
		t.Fatalf("export: status %d, body %q", status, raw)
	}
	records := csvRecords(t, raw, header)

	// Header row: the documented 9-column schema.
	wantHeader := []string{"id", "created_at", "actor_type", "actor_id", "action", "target_type", "target_id", "request_id", "metadata"}
	if len(records) != 3 {
		t.Fatalf("got %d records, want header + 2 events: %v", len(records), records)
	}
	if fmt.Sprint(records[0]) != fmt.Sprint(wantHeader) {
		t.Fatalf("header = %v, want %v", records[0], wantHeader)
	}

	// Newest first: qa.two precedes qa.one.
	rowTwo, rowOne := records[1], records[2]
	if rowTwo[4] != "qa.two" || rowOne[4] != "qa.one" {
		t.Fatalf("order = [%s, %s], want [qa.two, qa.one]", rowTwo[4], rowOne[4])
	}
	if rowTwo[2] != "operator" || rowTwo[3] != "admin-1" {
		t.Fatalf("qa.two actor = %v, want operator/admin-1", rowTwo[2:4])
	}
	if rowTwo[5] != "provider" || rowTwo[6] != providerID {
		t.Fatalf("qa.two target = %v, want provider/%s", rowTwo[5:7], providerID)
	}
	if rowTwo[7] != "req-123" {
		t.Fatalf("qa.two request_id = %q, want req-123", rowTwo[7])
	}
	if !strings.Contains(rowTwo[8], `"reason"`) || !strings.Contains(rowTwo[8], `"qa"`) {
		t.Fatalf("qa.two metadata = %q, want embedded jsonb", rowTwo[8])
	}
	if rowOne[7] != "" || rowOne[8] != "{}" {
		t.Fatalf("qa.one optional cells = %v, want empty request_id and {} metadata", rowOne[7:9])
	}
	// created_at is an RFC3339 timestamp.
	if _, err := time.Parse(time.RFC3339Nano, rowTwo[1]); err != nil {
		t.Fatalf("qa.two created_at %q not RFC3339: %v", rowTwo[1], err)
	}
}

// TestAuditExportJSON verifies the JSON stream is a parseable bare array whose
// objects carry the same fields the list endpoint exposes.
func TestAuditExportJSON(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "aex-json-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	pid := uuid.MustParse(providerID)
	base := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	injectAuditEventDetailed(t, pid, base, "credential", "svc-a", "qa.json",
		map[string]any{"note": "x"}, "req-json")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(time.Hour).Format(time.RFC3339)
	status, header, raw := apiRawReq(t, "GET", "/v1/audit-events/export?from="+from+"&to="+to+"&format=json", apiKey)
	if status != http.StatusOK {
		t.Fatalf("export: status %d, body %q", status, raw)
	}
	if ct := header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if cd := header.Get("Content-Disposition"); !strings.HasSuffix(cd, `.json"`) {
		t.Fatalf("Content-Disposition = %q, want .json attachment", cd)
	}
	var events []map[string]any
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("json export not a bare array: %v (body %q)", err, raw)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %v", len(events), events)
	}
	ev := events[0]
	if ev["action"] != "qa.json" || ev["actor_type"] != "credential" || ev["actor_id"] != "svc-a" {
		t.Fatalf("event fields = %v", ev)
	}
	if ev["provider_id"] != providerID {
		t.Fatalf("provider_id = %v, want %s", ev["provider_id"], providerID)
	}
	if ev["request_id"] != "req-json" {
		t.Fatalf("request_id = %v, want req-json", ev["request_id"])
	}
	if md, ok := ev["metadata"].(map[string]any); !ok || md["note"] != "x" {
		t.Fatalf("metadata = %v, want {note: x}", ev["metadata"])
	}
	if _, ok := ev["id"].(float64); !ok {
		t.Fatalf("id missing or not numeric: %v", ev["id"])
	}
	if _, ok := ev["created_at"].(string); !ok {
		t.Fatalf("created_at missing: %v", ev)
	}
}

// TestAuditExportParameterValidation verifies the export-specific guards:
// bounded required window and a format enum.
func TestAuditExportParameterValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "aex-prm-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	now := time.Now().UTC()
	from := now.Format(time.RFC3339)
	to := now.Add(time.Hour).Format(time.RFC3339)
	cases := []struct{ path, code string }{
		{"/v1/audit-events/export?to=" + to, "missing_from"},
		{"/v1/audit-events/export?from=" + from, "missing_to"},
		{"/v1/audit-events/export?from=" + to + "&to=" + from, "invalid_range"},
		{"/v1/audit-events/export?from=" + from + "&to=" + now.Add(400*24*time.Hour).Format(time.RFC3339), "range_too_wide"},
		{"/v1/audit-events/export?from=" + from + "&to=" + to + "&format=xml", "invalid_format"},
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

// TestAuditExportTenantIsolation verifies an export only ever carries the
// caller's own events, while the operator view spans every environment.
func TestAuditExportTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "aex-iso-a-"+uuid.NewString()[:8])
	keyA = createAuditKey(t, keyA)
	idB, keyB := createProviderAPI(t, "aex-iso-b-"+uuid.NewString()[:8])
	keyB = createAuditKey(t, keyB)
	pidB := uuid.MustParse(idB)
	base := time.Date(2026, 6, 3, 8, 0, 0, 0, time.UTC)
	injectAuditEventAt(t, pidB, base, "credential", "svc-b", "qa.secret.of.b")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(time.Hour).Format(time.RFC3339)

	// A's export must not contain B's event.
	status, _, raw := apiRawReq(t, "GET", "/v1/audit-events/export?from="+from+"&to="+to, keyA)
	if status != http.StatusOK {
		t.Fatalf("isolation export: status %d, body %q", status, raw)
	}
	if strings.Contains(string(raw), "qa.secret.of.b") {
		t.Fatalf("provider A export leaked B's event: %q", raw)
	}

	// B's export contains exactly its own event.
	status, header, raw := apiRawReq(t, "GET", "/v1/audit-events/export?from="+from+"&to="+to, keyB)
	if status != http.StatusOK {
		t.Fatalf("owner export: status %d, body %q", status, raw)
	}
	records := csvRecords(t, raw, header)
	if len(records) != 2 || records[1][4] != "qa.secret.of.b" {
		t.Fatalf("provider B export = %v, want its own event", records)
	}

	// The operator sees B's event via the provider-scoped export.
	status, header, raw = apiRawReq(t, "GET", "/v1/operator/providers/"+idB+"/audit/export?from="+from+"&to="+to, operatorToken)
	if status != http.StatusOK {
		t.Fatalf("operator export: status %d, body %q", status, raw)
	}
	records = csvRecords(t, raw, header)
	if len(records) != 2 || records[1][4] != "qa.secret.of.b" {
		t.Fatalf("operator export = %v, want B's event", records)
	}
}

// TestAuditExportRequiresScope verifies the export endpoint honors audit:read
// like the list and stats endpoints.
func TestAuditExportRequiresScope(t *testing.T) {
	_, baseKey := createProviderAPI(t, "aex-scope-"+uuid.NewString()[:8])
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
	status, body = apiReq(t, "GET", "/v1/audit-events/export?from="+from+"&to="+to, readOnlyKey, nil)
	if status != http.StatusForbidden {
		t.Fatalf("export without audit:read: status %d, want 403 (body %v)", status, body)
	}
}

// TestAuditExportOperatorParameterValidation verifies the operator route's
// id parsing and unknown-provider behavior.
func TestAuditExportOperatorParameterValidation(t *testing.T) {
	now := time.Now().UTC()
	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Format(time.RFC3339)

	// Non-uuid id -> 400 invalid_id.
	status, body := apiReq(t, "GET", "/v1/operator/providers/not-a-uuid/audit/export?from="+from+"&to="+to, operatorToken, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid id: status %d, want 400 (body %v)", status, body)
	}
	if errObj := body["error"].(map[string]any); errObj["code"] != "invalid_id" {
		t.Fatalf("invalid id error = %v, want invalid_id", errObj["code"])
	}

	// Unknown provider -> 404, no row enumeration.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+uuid.NewString()+"/audit/export?from="+from+"&to="+to, operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown provider: status %d, want 404 (body %v)", status, body)
	}
}

// TestAuditExportMatchesList cross-checks the CSV export against the list
// endpoint over the same window: same actions, same newest-first order.
func TestAuditExportMatchesList(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "aex-cns-"+uuid.NewString()[:8])
	apiKey = createAuditKey(t, apiKey)
	pid := uuid.MustParse(providerID)
	base := time.Date(2026, 6, 4, 6, 0, 0, 0, time.UTC)
	injectAuditEventAt(t, pid, base, "credential", "svc-a", "qa.one")
	injectAuditEventAt(t, pid, base.Add(time.Hour), "credential", "svc-a", "qa.two")
	injectAuditEventAt(t, pid, base.Add(2*time.Hour), "operator", "admin-1", "qa.three")

	from := base.Add(-time.Minute).Format(time.RFC3339)
	to := base.Add(3 * time.Hour).Format(time.RFC3339)

	status, body := apiReq(t, "GET", "/v1/audit-events?limit=100&from="+from+"&to="+to, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	var want []string
	for _, e := range body["audit_events"].([]any) {
		want = append(want, e.(map[string]any)["action"].(string))
	}
	if len(want) != 3 {
		t.Fatalf("list returned %d events, want 3: %v", len(want), want)
	}

	status, header, raw := apiRawReq(t, "GET", "/v1/audit-events/export?from="+from+"&to="+to, apiKey)
	if status != http.StatusOK {
		t.Fatalf("export: status %d, body %q", status, raw)
	}
	records := csvRecords(t, raw, header)
	if len(records) != 4 {
		t.Fatalf("export has %d records, want header + 3", len(records))
	}
	var got []string
	for _, rec := range records[1:] {
		got = append(got, rec[4])
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("export actions %v != list actions %v", got, want)
	}
}
