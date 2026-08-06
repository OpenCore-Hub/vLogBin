package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestAuthVaultLifecycle(t *testing.T) {
	body := map[string]any{
		"userSub":      "sub-test",
		"email":        "vault@example.com",
		"name":         "Vault User",
		"roles":        []string{"provider_admin"},
		"workspaceId":  "workspace-1",
		"env":          "test",
		"accessToken":  "access-token-secret",
		"refreshToken": "refresh-token-secret",
		"tokenExp":     time.Now().Add(time.Hour).Unix(),
		"ttlSeconds":   3600,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/auth/vault", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer test-auth-vault-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vault status = %d, want 201", resp.StatusCode)
	}
	var created struct {
		Vault struct {
			ID string `json:"id"`
		} `json:"vault"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Vault.ID == "" {
		t.Fatal("create response missing vault id")
	}

	req, _ = http.NewRequest(http.MethodGet, httpServer.URL+"/v1/auth/vault/"+created.Vault.ID, nil)
	req.Header.Set("Authorization", "Bearer test-auth-vault-token")
	getResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get vault: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get vault status = %d, want 200", getResp.StatusCode)
	}
	var got struct {
		Vault struct {
			AccessToken string `json:"accessToken"`
		} `json:"vault"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got.Vault.AccessToken != "access-token-secret" {
		t.Fatalf("access token = %q, want original", got.Vault.AccessToken)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, httpServer.URL+"/v1/auth/vault/"+created.Vault.ID, nil)
	delReq.Header.Set("Authorization", "Bearer test-auth-vault-token")
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("delete vault: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete vault status = %d, want 204", delResp.StatusCode)
	}

	missingReq, _ := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/auth/vault/"+created.Vault.ID, nil)
	missingReq.Header.Set("Authorization", "Bearer test-auth-vault-token")
	missingResp, err := http.DefaultClient.Do(missingReq)
	if err != nil {
		t.Fatalf("get deleted vault: %v", err)
	}
	defer missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted vault status = %d, want 404", missingResp.StatusCode)
	}
}
