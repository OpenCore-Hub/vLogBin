package keys

import (
	"strings"
	"testing"
)

func TestGenerateFormat(t *testing.T) {
	k, err := Generate("test")
	if err != nil {
		t.Fatalf("Generate(test): %v", err)
	}
	if !strings.HasPrefix(k, PrefixTest) {
		t.Fatalf("expected prefix %q, got %q", PrefixTest, k)
	}
	if got := len(strings.TrimPrefix(k, PrefixTest)); got != 32 {
		t.Fatalf("expected 32 random chars, got %d", got)
	}

	k, err = Generate("live")
	if err != nil {
		t.Fatalf("Generate(live): %v", err)
	}
	if !strings.HasPrefix(k, PrefixLive) {
		t.Fatalf("expected prefix %q, got %q", PrefixLive, k)
	}
}

func TestGenerateUnknownKind(t *testing.T) {
	if _, err := Generate("staging"); err == nil {
		t.Fatal("expected error for unknown environment kind")
	}
}

func TestGenerateUnique(t *testing.T) {
	a, _ := Generate("test")
	b, _ := Generate("test")
	if a == b {
		t.Fatal("two generated keys must differ")
	}
}

func TestHash(t *testing.T) {
	k, _ := Generate("live")
	h := Hash(k)
	if len(h) != 64 {
		t.Fatalf("expected sha256 hex (64 chars), got %d", len(h))
	}
	other, _ := Generate("live")
	if Hash(other) == h {
		t.Fatal("different keys must hash differently")
	}
}

func TestPrefix(t *testing.T) {
	k, _ := Generate("test")
	p := Prefix(k)
	if len(p) != PrefixLen {
		t.Fatalf("expected %d chars, got %d", PrefixLen, len(p))
	}
	if !strings.HasPrefix(k, p) {
		t.Fatal("stored prefix must be a prefix of the key")
	}
	if got := Prefix("short"); got != "short" {
		t.Fatalf("short key prefix = %q", got)
	}
}
