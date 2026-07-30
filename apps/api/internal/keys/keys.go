// Package keys implements the platform API-key credential format.
// Keys look like pk_test_<32 url-safe chars> / pk_live_<32 url-safe chars>.
// Only the SHA-256 hash of the key is stored; the plaintext is returned
// exactly once at creation.
package keys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
)

const (
	PrefixTest = "pk_test_"
	PrefixLive = "pk_live_"

	// randomBytes is 24 bytes → 32 base64url chars.
	randomBytes = 24
	// PrefixLen is the number of leading key characters stored for lookup UX.
	PrefixLen = 12
)

// Generate creates a new API key for the given environment kind
// ("test" or "live") and returns the plaintext key.
func Generate(envKind string) (string, error) {
	var prefix string
	switch envKind {
	case domain.EnvKindTest:
		prefix = PrefixTest
	case domain.EnvKindLive:
		prefix = PrefixLive
	default:
		return "", fmt.Errorf("unknown environment kind %q", envKind)
	}
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Hash returns the SHA-256 hex digest of a plaintext key. This is what is
// stored in credentials.key_hash.
func Hash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Prefix returns the first PrefixLen characters of a key, stored as
// credentials.key_prefix for display/lookup.
func Prefix(key string) string {
	if len(key) <= PrefixLen {
		return key
	}
	return key[:PrefixLen]
}
