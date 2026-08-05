package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSignPayload(t *testing.T) {
	secret := "test-secret"
	timestamp := "1627742400"
	payload := []byte(`{"event":"test"}`)

	// Compute expected signature manually.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	got := signPayload(secret, timestamp, payload)
	if got != expected {
		t.Fatalf("signPayload = %q, want %q", got, expected)
	}
}

func TestSignPayloadDifferentSecret(t *testing.T) {
	payload := []byte("test")
	sig1 := signPayload("secret1", "123", payload)
	sig2 := signPayload("secret2", "123", payload)

	if sig1 == sig2 {
		t.Fatal("different secrets must produce different signatures")
	}
}

func TestSignPayloadDifferentPayload(t *testing.T) {
	sig1 := signPayload("secret", "123", []byte("payload1"))
	sig2 := signPayload("secret", "123", []byte("payload2"))

	if sig1 == sig2 {
		t.Fatal("different payloads must produce different signatures")
	}
}

func TestSignPayloadDifferentTimestamp(t *testing.T) {
	sig1 := signPayload("secret", "123", []byte("payload"))
	sig2 := signPayload("secret", "456", []byte("payload"))

	if sig1 == sig2 {
		t.Fatal("different timestamps must produce different signatures")
	}
}

func TestSignPayloadEmpty(t *testing.T) {
	sig := signPayload("", "", []byte{})
	if sig == "" {
		t.Fatal("signPayload with empty inputs should still produce a signature")
	}
	// Verify it's a valid hex string.
	if _, err := hex.DecodeString(sig); err != nil {
		t.Fatalf("signPayload output is not valid hex: %v", err)
	}
}

func TestVerifySignature(t *testing.T) {
	secret := "test-secret"
	timestamp := "1627742400"
	payload := []byte(`{"event":"test"}`)
	sig := signPayload(secret, timestamp, payload)

	// Correct signature must verify.
	if !VerifySignature(secret, timestamp, payload, sig) {
		t.Fatal("VerifySignature returned false for a valid signature")
	}
}

func TestVerifySignatureWrongSecret(t *testing.T) {
	payload := []byte("test")
	sig := signPayload("secret1", "123", payload)

	// Different secret must fail verification.
	if VerifySignature("secret2", "123", payload, sig) {
		t.Fatal("VerifySignature returned true for a wrong secret")
	}
}

func TestVerifySignatureWrongPayload(t *testing.T) {
	sig := signPayload("secret", "123", []byte("payload1"))

	// Different payload must fail verification.
	if VerifySignature("secret", "123", []byte("payload2"), sig) {
		t.Fatal("VerifySignature returned true for a wrong payload")
	}
}

func TestVerifySignatureWrongTimestamp(t *testing.T) {
	sig := signPayload("secret", "123", []byte("payload"))

	// Different timestamp must fail verification.
	if VerifySignature("secret", "456", []byte("payload"), sig) {
		t.Fatal("VerifySignature returned true for a wrong timestamp")
	}
}

func TestVerifySignatureTamperedSignature(t *testing.T) {
	sig := signPayload("secret", "123", []byte("payload"))

	// Flip a character in the signature — must fail.
	tampered := sig[:len(sig)-1]
	if sig[len(sig)-1] == 'a' {
		tampered += "b"
	} else {
		tampered += "a"
	}
	if VerifySignature("secret", "123", []byte("payload"), tampered) {
		t.Fatal("VerifySignature returned true for a tampered signature")
	}
}

func TestVerifySignatureInvalidHex(t *testing.T) {
	// Non-hex string must fail (not panic).
	if VerifySignature("secret", "123", []byte("payload"), "not-hex!") {
		t.Fatal("VerifySignature returned true for invalid hex")
	}
}

func TestVerifySignatureEmptySignature(t *testing.T) {
	sig := signPayload("secret", "123", []byte("payload"))
	// Empty string is valid hex (decodes to empty byte slice), so it
	// won't match the non-empty expected signature.
	if sig == "" {
		t.Fatal("signPayload produced empty signature")
	}
	if VerifySignature("secret", "123", []byte("payload"), "") {
		t.Fatal("VerifySignature returned true for an empty signature")
	}
}

func TestEventMatchesFilter(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		filter    []string
		want      bool
	}{
		{"empty filter matches all", "customer.created", []string{}, true},
		{"nil filter matches all", "customer.created", nil, true},
		{"exact match", "customer.created", []string{"customer.created"}, true},
		{"match in list", "customer.created", []string{"subscription.created", "customer.created"}, true},
		{"no match", "customer.created", []string{"subscription.created"}, false},
		{"empty event type with empty filter", "", []string{}, true},
		{"empty event type with non-empty filter", "", []string{"customer.created"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventMatchesFilter(tt.eventType, tt.filter)
			if got != tt.want {
				t.Errorf("eventMatchesFilter(%q, %v) = %v, want %v", tt.eventType, tt.filter, got, tt.want)
			}
		})
	}
}
