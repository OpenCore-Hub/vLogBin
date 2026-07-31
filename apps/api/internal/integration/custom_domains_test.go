package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestRegisterCustomDomain(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-reg-"+uuid.NewString()[:8])

	status, body := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "auth.example.com",
	})
	if status != http.StatusCreated {
		t.Fatalf("register: status %d, body %v", status, body)
	}
	if body["domain"] != "auth.example.com" {
		t.Fatalf("domain = %v", body["domain"])
	}
	if body["status"] != "pending" {
		t.Fatalf("status = %v, want pending", body["status"])
	}
	token := body["verification_token"].(string)
	if token == "" {
		t.Fatal("verification_token must be generated")
	}
}

func TestVerifyCustomDomain(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-ver-"+uuid.NewString()[:8])

	// Register domain.
	status, body := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "verify.example.com",
	})
	if status != http.StatusCreated {
		t.Fatalf("register: status %d", status)
	}
	domainID := body["id"].(string)
	token := body["verification_token"].(string)

	// Simulate DNS TXT record.
	txtName := "_vlogbin-verify.verify.example.com"
	testDNSMu.Lock()
	testDNSRecords[txtName] = token
	testDNSMu.Unlock()
	defer func() {
		testDNSMu.Lock()
		delete(testDNSRecords, txtName)
		testDNSMu.Unlock()
	}()

	// Verify.
	status, body = apiReq(t, "POST", "/v1/custom-domains/"+domainID+"/verify", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("verify: status %d, body %v", status, body)
	}
	if body["status"] != "verified" {
		t.Fatalf("status = %v, want verified", body["status"])
	}
	if body["verified_at"] == nil {
		t.Fatal("verified_at must be set")
	}
}

func TestVerifyCustomDomainFailure(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-vf-"+uuid.NewString()[:8])

	// Register domain (no DNS TXT record added).
	status, body := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "noverify.example.com",
	})
	domainID := body["id"].(string)

	// Verify should fail (no TXT record).
	status, body = apiReq(t, "POST", "/v1/custom-domains/"+domainID+"/verify", apiKey, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("verify without DNS: status %d, want 400, body %v", status, body)
	}
}

func TestVerifyCustomDomainWrongToken(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-wt-"+uuid.NewString()[:8])

	status, body := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "wrongtoken.example.com",
	})
	domainID := body["id"].(string)

	// Add a TXT record with the wrong token.
	txtName := "_vlogbin-verify.wrongtoken.example.com"
	testDNSMu.Lock()
	testDNSRecords[txtName] = "vlogbin-verify-wrong-token"
	testDNSMu.Unlock()
	defer func() {
		testDNSMu.Lock()
		delete(testDNSRecords, txtName)
		testDNSMu.Unlock()
	}()

	status, _ = apiReq(t, "POST", "/v1/custom-domains/"+domainID+"/verify", apiKey, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("verify with wrong token: status %d, want 400", status)
	}
}

func TestListCustomDomains(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-lst-"+uuid.NewString()[:8])

	// Initially empty.
	status, body := apiReq(t, "GET", "/v1/custom-domains", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	domains := body["custom_domains"].([]any)
	if len(domains) != 0 {
		t.Fatalf("expected 0 domains, got %d", len(domains))
	}

	// Register two domains.
	for _, d := range []string{"a.example.com", "b.example.com"} {
		apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{"domain": d})
	}

	status, body = apiReq(t, "GET", "/v1/custom-domains", apiKey, nil)
	domains = body["custom_domains"].([]any)
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(domains))
	}
}

func TestRevokeCustomDomain(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-rvk-"+uuid.NewString()[:8])

	// Register and verify.
	status, body := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "revoke.example.com",
	})
	domainID := body["id"].(string)
	token := body["verification_token"].(string)

	txtName := "_vlogbin-verify.revoke.example.com"
	testDNSMu.Lock()
	testDNSRecords[txtName] = token
	testDNSMu.Unlock()
	defer func() {
		testDNSMu.Lock()
		delete(testDNSRecords, txtName)
		testDNSMu.Unlock()
	}()

	apiReq(t, "POST", "/v1/custom-domains/"+domainID+"/verify", apiKey, nil)

	// Revoke.
	status, body = apiReq(t, "POST", "/v1/custom-domains/"+domainID+"/revoke", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke: status %d, body %v", status, body)
	}
	if body["status"] != "revoked" {
		t.Fatalf("status = %v, want revoked", body["status"])
	}

	// Cannot revoke again.
	status, _ = apiReq(t, "POST", "/v1/custom-domains/"+domainID+"/revoke", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("double revoke: status %d, want 409", status)
	}
}

func TestDeleteCustomDomain(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-del-"+uuid.NewString()[:8])

	// Register a pending domain (not verified).
	status, body := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "delete.example.com",
	})
	domainID := body["id"].(string)

	// Delete pending domain — should succeed.
	status, body = apiReq(t, "DELETE", "/v1/custom-domains/"+domainID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete pending: status %d, want 204, body %v", status, body)
	}

	// Register and verify another domain.
	status, body = apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "delete2.example.com",
	})
	domainID2 := body["id"].(string)
	token := body["verification_token"].(string)
	txtName := "_vlogbin-verify.delete2.example.com"
	testDNSMu.Lock()
	testDNSRecords[txtName] = token
	testDNSMu.Unlock()
	defer func() {
		testDNSMu.Lock()
		delete(testDNSRecords, txtName)
		testDNSMu.Unlock()
	}()

	apiReq(t, "POST", "/v1/custom-domains/"+domainID2+"/verify", apiKey, nil)

	// Cannot delete a verified domain (must revoke first).
	status, _ = apiReq(t, "DELETE", "/v1/custom-domains/"+domainID2, apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete verified: status %d, want 409", status)
	}

	// Revoke then delete.
	apiReq(t, "POST", "/v1/custom-domains/"+domainID2+"/revoke", apiKey, nil)
	status, _ = apiReq(t, "DELETE", "/v1/custom-domains/"+domainID2, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete revoked: status %d, want 204", status)
	}
}

func TestCustomDomainTakeoverProtection(t *testing.T) {
	_, keyA := createProviderAPI(t, "cd-tk-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "cd-tk-b-"+uuid.NewString()[:8])

	// Provider A registers a domain.
	status, _ := apiReq(t, "POST", "/v1/custom-domains", keyA, map[string]any{
		"domain": "takeover.example.com",
	})
	if status != http.StatusCreated {
		t.Fatalf("A register: status %d", status)
	}

	// Provider B cannot register the same domain (takeover protection).
	status, body := apiReq(t, "POST", "/v1/custom-domains", keyB, map[string]any{
		"domain": "takeover.example.com",
	})
	if status != http.StatusConflict {
		t.Fatalf("B register same domain: status %d, want 409, body %v", status, body)
	}
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "domain_taken" {
		t.Fatalf("error code = %v, want domain_taken", errObj["code"])
	}
}

func TestCustomDomainCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "cd-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "cd-iso-b-"+uuid.NewString()[:8])

	// Provider A registers a domain.
	apiReq(t, "POST", "/v1/custom-domains", keyA, map[string]any{
		"domain": fmt.Sprintf("iso-a-%s.example.com", uuid.NewString()[:8]),
	})

	// Provider B cannot see A's domains.
	status, body := apiReq(t, "GET", "/v1/custom-domains", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	domains := body["custom_domains"].([]any)
	if len(domains) != 0 {
		t.Fatalf("B: expected 0 domains, got %d (RLS leak)", len(domains))
	}
}

func TestCustomDomainValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cd-val-"+uuid.NewString()[:8])

	// Missing domain.
	status, _ := apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("missing domain: status %d, want 400", status)
	}

	// IP address as domain.
	status, _ = apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "192.168.1.1",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("IP address: status %d, want 400", status)
	}

	// No dot in domain.
	status, _ = apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "localhost",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("no dot: status %d, want 400", status)
	}

	// Invalid characters.
	status, _ = apiReq(t, "POST", "/v1/custom-domains", apiKey, map[string]any{
		"domain": "bad_domain!.example.com",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid chars: status %d, want 400", status)
	}
}
