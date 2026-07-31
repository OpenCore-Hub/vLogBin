package integration

import (
	"net/http"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/google/uuid"
)

func TestPSPCredentialLifecycle(t *testing.T) {
	_, apiKey := createProviderAPI(t, "psp-"+uuid.NewString()[:8])

	// Create a PSP credential.
	status, body := apiReq(t, "POST", "/v1/psp-credentials", apiKey, map[string]any{
		"psp_type":       "stripe",
		"label":          "main stripe key",
		"api_key":        "sk_test_12345",
		"webhook_secret": "whsec_abcde",
	})
	if status != http.StatusCreated {
		t.Fatalf("create psp: status %d, body %v", status, body)
	}
	cred := body["credential"].(map[string]any)
	credID := cred["id"].(string)
	if cred["psp_type"] != "stripe" {
		t.Fatalf("psp_type = %v, want stripe", cred["psp_type"])
	}
	if body["plaintext_api_key"] != "sk_test_12345" {
		t.Fatalf("plaintext_api_key = %v, want sk_test_12345", body["plaintext_api_key"])
	}

	// Verify the encrypted_api_key in the database is NOT the plaintext.
	var encryptedKey string
	if err := superPool.QueryRow(testCtx,
		"SELECT encrypted_api_key FROM psp_credentials WHERE id = $1", credID).Scan(&encryptedKey); err != nil {
		t.Fatalf("query encrypted key: %v", err)
	}
	if encryptedKey == "sk_test_12345" {
		t.Fatal("encrypted_api_key must not be plaintext")
	}
	if encryptedKey == "" {
		t.Fatal("encrypted_api_key must not be empty")
	}

	// List credentials — metadata only, no plaintext.
	status, body = apiReq(t, "GET", "/v1/psp-credentials", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list psp: status %d", status)
	}
	creds, ok := body["credentials"].([]any)
	if !ok || len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %v", body["credentials"])
	}
	listCred := creds[0].(map[string]any)
	if listCred["encrypted_api_key"] == "sk_test_12345" {
		t.Fatal("list must not return plaintext api key")
	}
	if listCred["active"] != true {
		t.Fatalf("active = %v, want true", listCred["active"])
	}

	// Rotate the credential.
	status, body = apiReq(t, "POST", "/v1/psp-credentials/"+credID+"/rotate", apiKey, map[string]any{
		"new_api_key": "sk_test_67890",
	})
	if status != http.StatusOK {
		t.Fatalf("rotate psp: status %d, body %v", status, body)
	}
	if body["plaintext_api_key"] != "sk_test_67890" {
		t.Fatalf("rotated plaintext = %v, want sk_test_67890", body["plaintext_api_key"])
	}

	// List again — old credential revoked, new credential active.
	status, body = apiReq(t, "GET", "/v1/psp-credentials", apiKey, nil)
	creds, ok = body["credentials"].([]any)
	if !ok || len(creds) != 2 {
		t.Fatalf("expected 2 credentials (old revoked + new active), got %v", body["credentials"])
	}
	var activeCount, revokedCount int
	for _, c := range creds {
		cm := c.(map[string]any)
		if cm["active"] == true {
			activeCount++
			if cm["key_version"].(float64) != 2 {
				t.Fatalf("new credential key_version = %v, want 2", cm["key_version"])
			}
		} else {
			revokedCount++
		}
	}
	if activeCount != 1 || revokedCount != 1 {
		t.Fatalf("expected 1 active + 1 revoked, got %d active + %d revoked", activeCount, revokedCount)
	}

	// Revoke the active credential.
	// Find the active credential ID.
	var activeID string
	for _, c := range creds {
		cm := c.(map[string]any)
		if cm["active"] == true {
			activeID = cm["id"].(string)
			break
		}
	}
	if activeID == "" {
		t.Fatal("no active credential found")
	}
	status, _ = apiReq(t, "DELETE", "/v1/psp-credentials/"+activeID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("revoke psp: status %d, want 204", status)
	}

	// List — both credentials now revoked.
	status, body = apiReq(t, "GET", "/v1/psp-credentials", apiKey, nil)
	creds, _ = body["credentials"].([]any)
	for _, c := range creds {
		if c.(map[string]any)["active"] == true {
			t.Fatal("all credentials should be revoked")
		}
	}
}

func TestPSPCredentialEncryption(t *testing.T) {
	// Verify that the crypto.Encryptor correctly encrypts and decrypts.
	// This is a white-box test of the encryption layer.
	enc, err := crypto.NewEncryptor("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("create encryptor: %v", err)
	}

	plaintext := "sk_live_super_secret_key_12345"
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if ciphertext == plaintext {
		t.Fatal("ciphertext must not equal plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}

	// Each encryption produces a different ciphertext (random nonce).
	ct2, _ := enc.Encrypt(plaintext)
	if ct2 == ciphertext {
		t.Fatal("two encryptions of same plaintext must differ (random nonce)")
	}
}
