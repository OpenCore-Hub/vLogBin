package keys

import (
	"strings"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
)

func TestGenerateTest(t *testing.T) {
	key, err := Generate(domain.EnvKindTest)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(key, PrefixTest) {
		t.Fatalf("key = %q, want prefix %q", key, PrefixTest)
	}
	// pk_test_ + 32 base64url chars = 40 chars total
	if len(key) != len(PrefixTest)+32 {
		t.Fatalf("key length = %d, want %d", len(key), len(PrefixTest)+32)
	}
}

func TestGenerateLive(t *testing.T) {
	key, err := Generate(domain.EnvKindLive)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(key, PrefixLive) {
		t.Fatalf("key = %q, want prefix %q", key, PrefixLive)
	}
	if len(key) != len(PrefixLive)+32 {
		t.Fatalf("key length = %d, want %d", len(key), len(PrefixLive)+32)
	}
}

func TestGenerateUnknownKind(t *testing.T) {
	_, err := Generate("invalid")
	if err == nil {
		t.Fatal("Generate with invalid kind should fail")
	}
}

func TestGenerateUniqueness(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		k, _ := Generate(domain.EnvKindTest)
		if keys[k] {
			t.Fatalf("duplicate key generated: %s", k)
		}
		keys[k] = true
	}
}

func TestHash(t *testing.T) {
	key := "pk_test_abc123"
	h := Hash(key)

	// SHA-256 hex = 64 chars
	if len(h) != 64 {
		t.Fatalf("hash length = %d, want 64", len(h))
	}

	// Same input → same hash
	h2 := Hash(key)
	if h != h2 {
		t.Fatal("same input must produce same hash")
	}

	// Different input → different hash
	h3 := Hash("pk_test_different")
	if h == h3 {
		t.Fatal("different inputs must produce different hashes")
	}
}

func TestPrefix(t *testing.T) {
	key := "pk_test_abcdefghijklmnop"
	p := Prefix(key)

	if len(p) != PrefixLen {
		t.Fatalf("prefix length = %d, want %d", len(p), PrefixLen)
	}
	if p != key[:PrefixLen] {
		t.Fatalf("prefix = %q, want %q", p, key[:PrefixLen])
	}
}

func TestPrefixShortKey(t *testing.T) {
	short := "short"
	p := Prefix(short)

	// Short key should return itself (no truncation).
	if p != short {
		t.Fatalf("Prefix(%q) = %q, want %q", short, p, short)
	}
}

func TestHashVerifyRoundTrip(t *testing.T) {
	key, _ := Generate(domain.EnvKindLive)
	h := Hash(key)

	// Verify: hash of the generated key must match the stored hash.
	if Hash(key) != h {
		t.Fatal("hash verification failed: stored hash doesn't match")
	}
}
