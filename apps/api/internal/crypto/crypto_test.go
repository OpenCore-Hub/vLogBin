package crypto

import (
	"encoding/hex"
	"testing"
)

func TestEncryptorRoundTrip(t *testing.T) {
	// 32-byte key as hex string (64 chars)
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	plaintext := "super-secret-psp-credential"

	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if ciphertext == plaintext {
		t.Fatal("ciphertext must differ from plaintext")
	}

	decrypted, err := enc.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptorDifferentCiphertexts(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	plaintext := "same-input"

	ct1, _ := enc.Encrypt(plaintext)
	ct2, _ := enc.Encrypt(plaintext)

	if ct1 == ct2 {
		t.Fatal("same plaintext must produce different ciphertexts (random nonce)")
	}

	d1, _ := enc.Decrypt(ct1)
	d2, _ := enc.Decrypt(ct2)
	if d1 != plaintext || d2 != plaintext {
		t.Fatal("both ciphertexts must decrypt to the same plaintext")
	}
}

func TestEncryptorEmptyInput(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	ct, err := enc.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	pt, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}

	if pt != "" {
		t.Fatalf("decrypted empty = %q, want empty", pt)
	}
}

func TestEncryptorWrongKey(t *testing.T) {
	key1 := hex.EncodeToString(bytesRepeat(0x42, 32))
	key2 := hex.EncodeToString(bytesRepeat(0x99, 32))

	enc1, _ := NewEncryptor(key1)
	enc2, _ := NewEncryptor(key2)

	ct, _ := enc1.Encrypt("secret")

	_, err := enc2.Decrypt(ct)
	if err == nil {
		t.Fatal("Decrypt with wrong key must fail")
	}
}

func TestEncryptorInvalidKey(t *testing.T) {
	_, err := NewEncryptor("invalid-hex")
	if err == nil {
		t.Fatal("NewEncryptor with invalid hex must fail")
	}
}

func TestEncryptorShortKey(t *testing.T) {
	// 16 bytes = AES-128, not AES-256
	_, err := NewEncryptor(hex.EncodeToString(bytesRepeat(0x42, 16)))
	if err == nil {
		t.Fatal("NewEncryptor with 16-byte key must fail (need 32 bytes)")
	}
}

// bytesRepeat returns a byte slice of length n filled with b.
func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
