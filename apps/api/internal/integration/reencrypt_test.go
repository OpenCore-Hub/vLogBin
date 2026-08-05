package integration

import (
	"net/http"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/google/uuid"
)

// testMasterKeyHex is the active master key baked into the shared test
// encryptor (main_test.go). The rotation test below treats it as the
// "rotated-out" key.
const testMasterKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// TestReencryptLegacyCiphertextsConverges is the end-to-end master-key
// rotation loop: a credential sealed under the old key is re-sealed with the
// new active key by the re-encryption worker, proving that
//
//	rotate → REENCRYPT_SWEEP_INTERVAL worker → fallback metric → 0 → drop old key
//
// converges in production. The worker sweeps ALL encrypted tables across all
// tenants (operator context), so this test intentionally sits last in the
// package (file name sorts after every other *_test.go) and must keep that
// property: after it runs, every ciphertext in the shared test database is
// sealed under newKeyHex.
func TestReencryptLegacyCiphertextsConverges(t *testing.T) {
	_, apiKey := createProviderAPI(t, "reenc-"+uuid.NewString()[:8])

	// Create a credential through the API; it is sealed under the shared
	// test master key (testMasterKeyHex) by the global svc encryptor.
	status, body := apiReq(t, "POST", "/v1/psp-credentials", apiKey, map[string]any{
		"psp_type":       "stripe",
		"label":          "rotation convergence",
		"api_key":        "sk_rotate_me_123",
		"webhook_secret": "whsec_rotate_me",
	})
	if status != http.StatusCreated {
		t.Fatalf("create psp: status %d body %v", status, body)
	}
	credID := body["credential"].(map[string]any)["id"].(string)

	// Simulate a master-key rotation: new active key, old key demoted to
	// previous (PSP_MASTER_KEY=newKeyHex, PSP_MASTER_KEY_PREVIOUS=old).
	newKeyHex := "9999999999999999999999999999999999999999999999999999999999999999"
	rotated, err := crypto.NewEncryptorWithPrevious(newKeyHex, []string{testMasterKeyHex})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}

	// Run one sweep with the rotated encryptor.
	reencrypted, err := service.New(appStore, baseDomain,
		service.WithBillingAdapter(billing.NewNoop(nil)),
		service.WithCryptoEncryptor(rotated),
	).ReencryptLegacyCiphertexts(testCtx, 0)
	if err != nil {
		t.Fatalf("ReencryptLegacyCiphertexts: %v", err)
	}
	if reencrypted < 1 {
		t.Fatalf("reencrypted = %d, want >= 1 (our credential must be converged)", reencrypted)
	}

	// The credential's ciphertext must now be sealed under the new active
	// key: readable with newKeyHex alone, and no longer with the old key.
	var ciphertext string
	if err := superPool.QueryRow(testCtx,
		"SELECT encrypted_api_key FROM psp_credentials WHERE id = $1", credID).Scan(&ciphertext); err != nil {
		t.Fatalf("query ciphertext: %v", err)
	}
	onlyNew, _ := crypto.NewEncryptor(newKeyHex)
	pt, err := onlyNew.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("ciphertext must be readable with the new active key alone: %v", err)
	}
	if pt != "sk_rotate_me_123" {
		t.Fatalf("plaintext = %q, want sk_rotate_me_123", pt)
	}
	oldKeyOnly, _ := crypto.NewEncryptor(testMasterKeyHex)
	if _, err := oldKeyOnly.Decrypt(ciphertext); err == nil {
		t.Fatal("ciphertext must no longer be readable with the rotated-out key (converged)")
	}

	// Idempotence: a second sweep must converge nothing (all rows already
	// sealed under the active key), proving the worker does not churn.
	again, err := service.New(appStore, baseDomain,
		service.WithBillingAdapter(billing.NewNoop(nil)),
		service.WithCryptoEncryptor(rotated),
	).ReencryptLegacyCiphertexts(testCtx, 0)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if again != 0 {
		t.Fatalf("second sweep reencrypted = %d, want 0 (idempotent)", again)
	}
}
