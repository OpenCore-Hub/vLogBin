package crypto

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestNewEncryptorInvalidHex(t *testing.T) {
	_, err := NewEncryptor("not-valid-hex!")
	if err == nil {
		t.Fatal("should fail on invalid hex")
	}
	if !strings.Contains(err.Error(), "decode master key") {
		t.Fatalf("error should mention decode, got: %v", err)
	}
}

func TestNewEncryptorShortKey(t *testing.T) {
	// 16 bytes → AES-128, not AES-256
	_, err := NewEncryptor(hex.EncodeToString(bytesRepeat(0x42, 16)))
	if err == nil {
		t.Fatal("should fail on 16-byte key")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("error should mention 32 bytes, got: %v", err)
	}
}

func TestNewEncryptorLongKey(t *testing.T) {
	// 64 bytes → too long
	_, err := NewEncryptor(hex.EncodeToString(bytesRepeat(0x42, 64)))
	if err == nil {
		t.Fatal("should fail on 64-byte key")
	}
}

func TestNewEncryptorEmptyKey(t *testing.T) {
	_, err := NewEncryptor("")
	if err == nil {
		t.Fatal("should fail on empty key")
	}
}

func TestEncryptorLargeInput(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	large := strings.Repeat("A", 10000)
	ct, err := enc.Encrypt(large)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}

	pt, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}

	if pt != large {
		t.Fatal("large input round-trip failed")
	}
}

func TestEncryptorDecryptTruncatedCiphertext(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	// Encrypt then truncate the ciphertext to simulate corruption.
	ct, _ := enc.Encrypt("secret")
	truncated := ct[:len(ct)/2]

	_, err := enc.Decrypt(truncated)
	if err == nil {
		t.Fatal("Decrypt truncated ciphertext should fail")
	}
}

func TestEncryptorDecryptModifiedCiphertext(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	ct, _ := enc.Encrypt("secret")

	// Flip a character in the ciphertext to simulate tampering.
	modified := ct[:len(ct)-2] + "XX"

	_, err := enc.Decrypt(modified)
	if err == nil {
		t.Fatal("Decrypt modified ciphertext should fail (GCM integrity check)")
	}
}

func TestEncryptorMultipleEncryptions(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	plaintext := "same-input"
	results := make(map[string]bool)

	for i := 0; i < 20; i++ {
		ct, _ := enc.Encrypt(plaintext)
		if results[ct] {
			t.Fatal("duplicate ciphertext (nonce not random)")
		}
		results[ct] = true

		// Each must decrypt correctly.
		pt, err := enc.Decrypt(ct)
		if err != nil || pt != plaintext {
			t.Fatalf("decryption failed on iteration %d", i)
		}
	}
}

func TestEncryptorUnicodeInput(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	tests := []string{
		"你好世界",
		"🔒🔐",
		"mixed 日本語 English",
		"\x00\x01\x02binary",
	}

	for _, pt := range tests {
		ct, err := enc.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt unicode: %v", err)
		}

		decrypted, err := enc.Decrypt(ct)
		if err != nil {
			t.Fatalf("Decrypt unicode: %v", err)
		}

		if decrypted != pt {
			t.Fatalf("round-trip failed for %q: got %q", pt, decrypted)
		}
	}
}

func TestEncryptorSpecialChars(t *testing.T) {
	key := hex.EncodeToString(bytesRepeat(0x42, 32))
	enc, _ := NewEncryptor(key)

	tests := []string{
		"",
		" ",
		"\n\t\r",
		"key=value&secret=password",
		`{"json":"data","num":123}`,
		"https://api.example.com:8080/path?q=test&k=val",
	}

	for _, pt := range tests {
		ct, _ := enc.Encrypt(pt)
		decrypted, _ := enc.Decrypt(ct)
		if decrypted != pt {
			t.Fatalf("round-trip failed for %q: got %q", pt, decrypted)
		}
	}
}
