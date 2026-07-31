package keys

import (
	"strings"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
)

func TestGeneratePrefixCorrectness(t *testing.T) {
	testKey, _ := Generate(domain.EnvKindTest)
	liveKey, _ := Generate(domain.EnvKindLive)

	if !strings.HasPrefix(testKey, PrefixTest) {
		t.Fatalf("test key should have prefix %q, got %q", PrefixTest, testKey)
	}
	if !strings.HasPrefix(liveKey, PrefixLive) {
		t.Fatalf("live key should have prefix %q, got %q", PrefixLive, liveKey)
	}
}

func TestGenerateKeyContent(t *testing.T) {
	// The random part after the prefix must be valid base64url.
	key, _ := Generate(domain.EnvKindTest)
	randomPart := key[len(PrefixTest):]

	// base64url uses A-Z, a-z, 0-9, -, _
	for _, c := range randomPart {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_') {
			t.Fatalf("invalid base64url character in key: %q", c)
		}
	}
}

func TestGenerate1000Unique(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		k, err := Generate(domain.EnvKindLive)
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		if keys[k] {
			t.Fatalf("duplicate key after %d iterations: %s", i, k)
		}
		keys[k] = true
	}
}

func TestHashDeterministic(t *testing.T) {
	key := "pk_test_abc123"
	h1 := Hash(key)
	h2 := Hash(key)
	h3 := Hash(key)

	if h1 != h2 || h2 != h3 {
		t.Fatal("Hash must be deterministic")
	}
}

func TestHashKnownValue(t *testing.T) {
	// Verify SHA-256 of a known input.
	key := "test"
	got := Hash(key)

	// SHA-256("test") = 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
	want := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if got != want {
		t.Fatalf("Hash(test) = %q, want %q", got, want)
	}
}

func TestHashEmptyString(t *testing.T) {
	h := Hash("")
	// SHA-256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	if h != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("Hash(empty) = %q, want known SHA-256 of empty string", h)
	}
}

func TestHashLength(t *testing.T) {
	h := Hash("any-input")
	if len(h) != 64 {
		t.Fatalf("hash length = %d, want 64 (SHA-256 hex)", len(h))
	}
}

func TestPrefixExactLength(t *testing.T) {
	key := "pk_test_abcdefghijklmnop"
	p := Prefix(key)

	if len(p) != PrefixLen {
		t.Fatalf("prefix length = %d, want %d", len(p), PrefixLen)
	}
}

func TestPrefixEqualsKeyWhenShort(t *testing.T) {
	shortKeys := []string{"", "a", "ab", "pk_test"}
	for _, k := range shortKeys {
		p := Prefix(k)
		if p != k {
			t.Fatalf("Prefix(%q) = %q, want %q (key shorter than PrefixLen)", k, p, k)
		}
	}
}

func TestPrefixBoundary(t *testing.T) {
	// Key exactly PrefixLen chars → should return itself.
	key := strings.Repeat("x", PrefixLen)
	p := Prefix(key)
	if p != key {
		t.Fatalf("Prefix(key of length %d) should return key itself", PrefixLen)
	}

	// Key one char longer → should return first PrefixLen chars.
	keyLong := key + "y"
	p = Prefix(keyLong)
	if p != key {
		t.Fatalf("Prefix(key longer than PrefixLen) should return first %d chars", PrefixLen)
	}
}

func TestGenerateAndVerify(t *testing.T) {
	// End-to-end: generate a key, hash it, and verify.
	key, err := Generate(domain.EnvKindLive)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	h := Hash(key)

	// The hash must be a valid hex string of length 64.
	if len(h) != 64 {
		t.Fatalf("hash length = %d, want 64", len(h))
	}

	// Re-hashing the same key must produce the same hash.
	if Hash(key) != h {
		t.Fatal("hash verification failed")
	}

	// The prefix must be extractable.
	p := Prefix(key)
	if len(p) != PrefixLen {
		t.Fatalf("prefix length = %d, want %d", len(p), PrefixLen)
	}
}

func TestGenerateBothKinds(t *testing.T) {
	testKey, err1 := Generate(domain.EnvKindTest)
	liveKey, err2 := Generate(domain.EnvKindLive)

	if err1 != nil || err2 != nil {
		t.Fatalf("Generate errors: %v, %v", err1, err2)
	}

	// Keys must differ (different prefixes + different random parts).
	if testKey == liveKey {
		t.Fatal("test and live keys must differ")
	}

	// Hashes must differ.
	if Hash(testKey) == Hash(liveKey) {
		t.Fatal("hashes of different keys must differ")
	}
}
