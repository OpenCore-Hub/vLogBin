package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// idemReq performs a request with an Idempotency-Key header and returns the
// status, response headers and raw body.
func idemReq(t *testing.T, method, path, token, idemKey string, body any) (int, http.Header, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, httpServer.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", idemKey)
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

// countCredentials lists the tenant's credentials and returns how many match
// the given name (there is always the provider's own auto-created key).
func countCredentials(t *testing.T, token, name string) int {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/credentials", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list credentials: status %d, body %v", status, body)
	}
	creds := body["credentials"].([]any)
	n := 0
	for _, c := range creds {
		if c.(map[string]any)["name"] == name {
			n++
		}
	}
	return n
}

// TestIdempotencyTenantReplay verifies that a tenant mutating request with an
// Idempotency-Key executes once and replays the cached response for identical
// retries, without creating duplicate resources.
func TestIdempotencyTenantReplay(t *testing.T) {
	slug := "idem-" + uuid.NewString()[:8]
	_, testKey := createProviderAPI(t, slug)
	const key = "create-credential-key-1"

	credBody := map[string]any{"name": "idem-cred", "scopes": []string{"credentials:manage"}}
	status, headers, body := idemReq(t, "POST", "/v1/credentials", testKey, key, credBody)
	if status != http.StatusCreated {
		t.Fatalf("first create: status %d, body %s", status, body)
	}
	if headers.Get("Idempotency-Replayed") != "" {
		t.Fatalf("first execution must not set Idempotency-Replayed, got %q", headers.Get("Idempotency-Replayed"))
	}
	if got := countCredentials(t, testKey, "idem-cred"); got != 1 {
		t.Fatalf("after first execution: %d credentials named idem-cred, want 1", got)
	}

	// Identical retry: same status and body, tagged replayed, no duplicate.
	status2, headers2, body2 := idemReq(t, "POST", "/v1/credentials", testKey, key, credBody)
	if status2 != status {
		t.Fatalf("replay status = %d, want %d", status2, status)
	}
	if headers2.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay must set Idempotency-Replayed: true")
	}
	if !bytes.Equal(body2, body) {
		t.Fatalf("replay body differs:\n first %s\nreplay %s", body, body2)
	}
	if got := countCredentials(t, testKey, "idem-cred"); got != 1 {
		t.Fatalf("after replay: %d credentials named idem-cred, want 1 (duplicate must not be created)", got)
	}

	// A different key is a fresh execution.
	status3, _, _ := idemReq(t, "POST", "/v1/credentials", testKey, key+"-other", credBody)
	if status3 != http.StatusCreated {
		t.Fatalf("different key: status %d, want 201", status3)
	}
	if got := countCredentials(t, testKey, "idem-cred"); got != 2 {
		t.Fatalf("after different-key request: %d credentials, want 2", got)
	}
}

// TestIdempotencyOperatorReplay verifies the operator path replays too: the
// same key sent twice to provider creation creates exactly one provider.
func TestIdempotencyOperatorReplay(t *testing.T) {
	const key = "create-provider-key-1"
	slug := "idemop-" + uuid.NewString()[:8]
	body := map[string]any{"slug": slug, "name": slug + " name", "home_region_code": regionCode}

	status, headers, first := idemReq(t, "POST", "/v1/operator/providers", operatorToken, key, body)
	if status != http.StatusCreated {
		t.Fatalf("first create: status %d, body %s", status, first)
	}
	if headers.Get("Idempotency-Replayed") != "" {
		t.Fatalf("first execution must not set Idempotency-Replayed")
	}

	status2, headers2, second := idemReq(t, "POST", "/v1/operator/providers", operatorToken, key, body)
	if status2 != http.StatusCreated {
		t.Fatalf("replay: status %d, want 201", status2)
	}
	if headers2.Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay must set Idempotency-Replayed: true")
	}
	if !bytes.Equal(second, first) {
		t.Fatalf("replay body differs:\n first %s\nreplay %s", first, second)
	}

	// Exactly one provider with this slug exists.
	status, list := apiReq(t, "GET", "/v1/operator/providers", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list providers: status %d", status)
	}
	n := 0
	for _, p := range list["providers"].([]any) {
		if p.(map[string]any)["slug"] == slug {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("providers with slug %s: %d, want 1 (replay must not create a duplicate)", slug, n)
	}
}

// TestIdempotencyInProgressConflict verifies that a duplicate request while
// the first is still in flight gets 409 concurrent instead of racing the
// handler.
func TestIdempotencyInProgressConflict(t *testing.T) {
	slug := "idem-" + uuid.NewString()[:8]
	providerID, testKey := createProviderAPI(t, slug)

	const key = "in-flight-key-1"
	sum := sha256.Sum256([]byte(key))
	scope := "provider:" + providerID
	_, err := superPool.Exec(testCtx, `
		INSERT INTO idempotency_keys (scope, key_hash, method, path, status, request_id, expires_at)
		VALUES ($1, $2, 'POST', '/v1/credentials', 'in_progress', 'test', now() + interval '1 hour')`,
		scope, sum[:])
	if err != nil {
		t.Fatalf("seed in_progress row: %v", err)
	}

	status, _, body := idemReq(t, "POST", "/v1/credentials", testKey, key, map[string]any{"name": "blocked"})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body %s", status, body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if errObj := decoded["error"].(map[string]any); errObj["code"] != "concurrent" {
		t.Fatalf("error code = %v, want concurrent", errObj["code"])
	}
}

// TestIdempotencyScopeIsolation verifies keys are scoped per authenticated
// identity: the same key used by two different providers does not replay
// across tenants.
func TestIdempotencyScopeIsolation(t *testing.T) {
	slugA := "idema-" + uuid.NewString()[:8]
	_, keyA := createProviderAPI(t, slugA)
	slugB := "idemb-" + uuid.NewString()[:8]
	_, keyB := createProviderAPI(t, slugB)
	const shared = "shared-key-1"

	statusA, _, _ := idemReq(t, "POST", "/v1/credentials", keyA, shared, map[string]any{"name": "iso-a", "scopes": []string{"credentials:manage"}})
	if statusA != http.StatusCreated {
		t.Fatalf("provider A first: status %d, want 201", statusA)
	}
	// Provider B with the same key must execute, not replay A's response.
	statusB, headersB, bodyB := idemReq(t, "POST", "/v1/credentials", keyB, shared, map[string]any{"name": "iso-b", "scopes": []string{"credentials:manage"}})
	if statusB != http.StatusCreated {
		t.Fatalf("provider B: status %d, want 201 (keys must be scoped per identity)", statusB)
	}
	if headersB.Get("Idempotency-Replayed") != "" {
		t.Fatalf("provider B must not see provider A's cached response")
	}
	if bytes.Contains(bodyB, []byte("iso-a")) {
		t.Fatalf("provider B replayed provider A's response: %s", bodyB)
	}
}

// TestPurgeExpiredIdempotencyKeys verifies the sweeper deletes records past
// their TTL.
func TestPurgeExpiredIdempotencyKeys(t *testing.T) {
	// Seed one expired and one live row (operator scope, RLS bypass not needed
	// since superPool is a superuser).
	expired := fmt.Sprintf("expired-%s", uuid.NewString())
	live := fmt.Sprintf("live-%s", uuid.NewString())
	for _, seed := range []struct{ key, exp string }{
		{expired, "now() - interval '1 hour'"},
		{live, "now() + interval '1 hour'"},
	} {
		sum := sha256.Sum256([]byte(seed.key))
		if _, err := superPool.Exec(testCtx, `
			INSERT INTO idempotency_keys (scope, key_hash, method, path, status, request_id, expires_at)
			VALUES ('operator:test', $1, 'POST', '/x', 'completed', 'test', `+seed.exp+`)`,
			sum[:]); err != nil {
			t.Fatalf("seed %s: %v", seed.key, err)
		}
	}

	n, err := svc.PurgeExpiredIdempotencyKeys(testCtx, time.Now().UTC())
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n < 1 {
		t.Fatalf("purged %d rows, want >= 1", n)
	}

	// Live row must survive.
	var count int
	liveHash := sha256.Sum256([]byte(live))
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM idempotency_keys WHERE key_hash = $1`,
		liveHash[:]).Scan(&count); err != nil {
		t.Fatalf("count live: %v", err)
	}
	if count != 1 {
		t.Fatalf("live rows = %d, want 1", count)
	}
}
