package crypto

import (
	"encoding/hex"
	"strings"
	"testing"
)

// mustKey returns a deterministic 64-char hex key derived from fill byte b.
func mustKey(b byte) string {
	return hex.EncodeToString(bytesRepeat(b, 32))
}

func TestEncryptorRotationLegacyCiphertextDecrypts(t *testing.T) {
	oldKey, newKey := mustKey(0x42), mustKey(0x99)

	legacy, err := NewEncryptor(oldKey)
	if err != nil {
		t.Fatalf("NewEncryptor(old): %v", err)
	}
	ct, err := legacy.Encrypt("pre-rotation-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	rotated, err := NewEncryptorWithPrevious(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}
	pt, err := rotated.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt legacy ciphertext after rotation: %v", err)
	}
	if pt != "pre-rotation-secret" {
		t.Fatalf("decrypted = %q, want %q", pt, "pre-rotation-secret")
	}
}

func TestEncryptorRotationNewCiphertextUsesActiveKey(t *testing.T) {
	oldKey, newKey := mustKey(0x42), mustKey(0x99)

	rotated, err := NewEncryptorWithPrevious(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}
	ct, err := rotated.Encrypt("fresh-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// An encryptor that only knows the new active key must be able to read
	// ciphertext produced after rotation — proving the active key, not a
	// previous one, sealed it.
	onlyNew, err := NewEncryptor(newKey)
	if err != nil {
		t.Fatalf("NewEncryptor(new): %v", err)
	}
	pt, err := onlyNew.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt with active key only: %v", err)
	}
	if pt != "fresh-secret" {
		t.Fatalf("decrypted = %q, want %q", pt, "fresh-secret")
	}
}

func TestEncryptorRotationFallbackObserver(t *testing.T) {
	oldKey, newKey := mustKey(0x42), mustKey(0x99)

	legacy, _ := NewEncryptor(oldKey)
	legacyCT, _ := legacy.Encrypt("legacy")

	rotated, err := NewEncryptorWithPrevious(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}
	fallbacks := 0
	rotated.SetFallbackObserver(func() { fallbacks++ })

	// Reading ciphertext written before rotation uses the previous key and
	// must trip the fallback observer.
	if _, err := rotated.Decrypt(legacyCT); err != nil {
		t.Fatalf("Decrypt legacy: %v", err)
	}
	if fallbacks != 1 {
		t.Fatalf("fallbacks after legacy read = %d, want 1", fallbacks)
	}

	// Reading ciphertext written after rotation uses the active key and must
	// NOT trip the fallback observer.
	freshCT, _ := rotated.Encrypt("fresh")
	if _, err := rotated.Decrypt(freshCT); err != nil {
		t.Fatalf("Decrypt fresh: %v", err)
	}
	if fallbacks != 1 {
		t.Fatalf("fallbacks after fresh read = %d, want 1", fallbacks)
	}
}

func TestEncryptorRotationMultiplePreviousKeys(t *testing.T) {
	k1, k2, k3 := mustKey(0x11), mustKey(0x22), mustKey(0x33)

	e1, _ := NewEncryptor(k1)
	ct1, _ := e1.Encrypt("under-k1")

	e2, err := NewEncryptorWithPrevious(k2, []string{k1})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious(k2): %v", err)
	}
	ct2, _ := e2.Encrypt("under-k2")

	// Two rotations in: active is k3, previous are [k2, k1].
	rotated, err := NewEncryptorWithPrevious(k3, []string{k2, k1})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious(k3): %v", err)
	}
	for name, ct := range map[string]string{"k1": ct1, "k2": ct2} {
		if _, err := rotated.Decrypt(ct); err != nil {
			t.Fatalf("Decrypt ciphertext %s: %v", name, err)
		}
	}
	ct3, _ := rotated.Encrypt("under-k3")
	if _, err := rotated.Decrypt(ct3); err != nil {
		t.Fatalf("Decrypt fresh ciphertext under k3: %v", err)
	}
}

func TestEncryptorRotationAllKeysFail(t *testing.T) {
	oldKey, wrongKey := mustKey(0x42), mustKey(0x55)

	legacy, _ := NewEncryptor(oldKey)
	ct, _ := legacy.Encrypt("secret")

	// An encryptor whose active and previous keys all differ from the key
	// that sealed the ciphertext must fail.
	other, err := NewEncryptorWithPrevious(wrongKey, []string{mustKey(0x11)})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}
	if _, err := other.Decrypt(ct); err == nil {
		t.Fatal("Decrypt must fail when no key matches")
	} else if !strings.Contains(err.Error(), "previous key") {
		t.Fatalf("error should mention previous keys, got: %v", err)
	}
}

func TestEncryptorRotationInvalidPreviousKey(t *testing.T) {
	newKey := mustKey(0x99)

	if _, err := NewEncryptorWithPrevious(newKey, []string{"not-hex"}); err == nil {
		t.Fatal("non-hex previous key must be rejected")
	}
	if _, err := NewEncryptorWithPrevious(newKey, []string{mustKey(0x11), "short"}); err == nil {
		t.Fatal("invalid previous key among a list must be rejected")
	}
	// 16-byte previous key must be rejected too (AES-128, not AES-256).
	if _, err := NewEncryptorWithPrevious(newKey, []string{hex.EncodeToString(bytesRepeat(0x11, 16))}); err == nil {
		t.Fatal("16-byte previous key must be rejected")
	}
}

func TestEncryptorRotationWithNoPreviousMatchesLegacyBehavior(t *testing.T) {
	key := mustKey(0x42)
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	ct, _ := enc.Encrypt("secret")
	if pt, err := enc.Decrypt(ct); err != nil || pt != "secret" {
		t.Fatalf("round-trip without previous keys: pt=%q err=%v", pt, err)
	}
}

func TestEncryptorNeedsReencryption(t *testing.T) {
	oldKey, newKey := mustKey(0x42), mustKey(0x99)

	legacy, _ := NewEncryptor(oldKey)
	legacyCT, _ := legacy.Encrypt("legacy")

	rotated, err := NewEncryptorWithPrevious(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}

	// Ciphertext written under the active key needs no re-encryption.
	freshCT, _ := rotated.Encrypt("fresh")
	needs, err := rotated.NeedsReencryption(freshCT)
	if err != nil {
		t.Fatalf("NeedsReencryption(fresh): %v", err)
	}
	if needs {
		t.Fatal("active-key ciphertext must not need re-encryption")
	}

	// Ciphertext written under the rotated-out key does.
	needs, err = rotated.NeedsReencryption(legacyCT)
	if err != nil {
		t.Fatalf("NeedsReencryption(legacy): %v", err)
	}
	if !needs {
		t.Fatal("previous-key ciphertext must need re-encryption")
	}

	// Ciphertext no configured key can open is an error, not a "needs"
	// verdict — the worker must count such rows as unrecoverable.
	foreign, _ := NewEncryptor(mustKey(0x77))
	foreignCT, _ := foreign.Encrypt("foreign")
	if _, err := rotated.NeedsReencryption(foreignCT); err == nil {
		t.Fatal("NeedsReencryption must error when no key matches")
	}

	// Detection must not fire the fallback observer (it never decrypts).
	fallbacks := 0
	rotated.SetFallbackObserver(func() { fallbacks++ })
	if _, err := rotated.NeedsReencryption(legacyCT); err != nil {
		t.Fatalf("NeedsReencryption(legacy) again: %v", err)
	}
	if fallbacks != 0 {
		t.Fatalf("fallbacks after detection-only call = %d, want 0", fallbacks)
	}
}

func TestEncryptorDecryptWithoutFallbackSilent(t *testing.T) {
	oldKey, newKey := mustKey(0x42), mustKey(0x99)

	legacy, _ := NewEncryptor(oldKey)
	legacyCT, _ := legacy.Encrypt("legacy")

	rotated, err := NewEncryptorWithPrevious(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("NewEncryptorWithPrevious: %v", err)
	}
	fallbacks := 0
	rotated.SetFallbackObserver(func() { fallbacks++ })

	// The re-encryption worker path must decrypt previous-key ciphertext
	// without moving the request-path fallback counter.
	pt, err := rotated.DecryptWithoutFallback(legacyCT)
	if err != nil {
		t.Fatalf("DecryptWithoutFallback: %v", err)
	}
	if pt != "legacy" {
		t.Fatalf("plaintext = %q, want legacy", pt)
	}
	if fallbacks != 0 {
		t.Fatalf("fallbacks after DecryptWithoutFallback = %d, want 0", fallbacks)
	}

	// Decrypt still notifies, proving the silent variant is the only one.
	if _, err := rotated.Decrypt(legacyCT); err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if fallbacks != 1 {
		t.Fatalf("fallbacks after Decrypt = %d, want 1", fallbacks)
	}
}
