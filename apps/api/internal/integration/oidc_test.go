package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/golang-jwt/jwt/v5"
)

// TestOIDCVerifier verifies that the ZITADEL OIDC verifier correctly
// validates JWT access tokens against a test JWKS endpoint.
func TestOIDCVerifier(t *testing.T) {
	// Generate a test RSA key pair.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey
	kid := "test-key-1"

	// Create JWK from the public key.
	nB64 := base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes())

	// Create JWKS response.
	jwksResponse := map[string]any{
		"keys": []map[string]any{
			{"kid": kid, "kty": "RSA", "n": nB64, "e": eB64},
		},
	}

	// Create discovery response.
	discoveryResponse := map[string]any{
		"issuer":   "",  // will be set below
		"jwks_uri": "",  // will be set below
	}

	// Create test servers for JWKS and discovery.
	var jwksURL, issuerURL string

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResponse)
	}))
	defer jwksServer.Close()

	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		discoveryResponse["issuer"] = issuerURL
		discoveryResponse["jwks_uri"] = jwksURL
		json.NewEncoder(w).Encode(discoveryResponse)
	}))
	defer discoveryServer.Close()

	issuerURL = discoveryServer.URL
	jwksURL = jwksServer.URL

	// Create the verifier.
	verifier, err := zitadel.NewVerifier(context.Background(), issuerURL)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// Helper to create a signed JWT.
	makeToken := func(issuer string, expiresAt time.Time) string {
		claims := &struct {
			Sub   string `json:"sub"`
			Email string `json:"email"`
			jwt.RegisteredClaims
		}{
			Sub:   "user-123",
			Email: "admin@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    issuer,
				ExpiresAt: jwt.NewNumericDate(expiresAt),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = kid
		signed, err := token.SignedString(privKey)
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return signed
	}

	// Test 1: Valid token.
	validToken := makeToken(issuerURL, time.Now().Add(1*time.Hour))
	claims, err := verifier.Verify(context.Background(), validToken)
	if err != nil {
		t.Fatalf("verify valid token: %v", err)
	}
	if claims.Sub != "user-123" {
		t.Fatalf("sub = %s, want user-123", claims.Sub)
	}
	if claims.Email != "admin@example.com" {
		t.Fatalf("email = %s, want admin@example.com", claims.Email)
	}

	// Test 2: Expired token.
	expiredToken := makeToken(issuerURL, time.Now().Add(-1*time.Hour))
	_, err = verifier.Verify(context.Background(), expiredToken)
	if err == nil {
		t.Fatal("expired token should fail verification")
	}

	// Test 3: Wrong issuer.
	wrongIssuerToken := makeToken("http://evil.example.com", time.Now().Add(1*time.Hour))
	_, err = verifier.Verify(context.Background(), wrongIssuerToken)
	if err == nil {
		t.Fatal("wrong issuer token should fail verification")
	}

	// Test 4: Garbage token.
	_, err = verifier.Verify(context.Background(), "not.a.jwt")
	if err == nil {
		t.Fatal("garbage token should fail verification")
	}

	// Test 5: Tampered signature.
	tampered := validToken[:len(validToken)-5] + "XXXXX"
	_, err = verifier.Verify(context.Background(), tampered)
	if err == nil {
		t.Fatal("tampered token should fail verification")
	}
}

// TestOIDCVerifierKeyRotation verifies that the verifier refreshes its
// JWKS cache when a new key is introduced.
func TestOIDCVerifierKeyRotation(t *testing.T) {
	// Generate two RSA key pairs.
	priv1, _ := rsa.GenerateKey(rand.Reader, 2048)
	priv2, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub1 := &priv1.PublicKey
	pub2 := &priv2.PublicKey

	keys := map[string]*rsa.PublicKey{"key-1": pub1}
	var mu sync.Mutex

	var jwksURL, issuerURL string

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var keyList []map[string]any
		for kid, pk := range keys {
			keyList = append(keyList, map[string]any{
				"kid": kid, "kty": "RSA",
				"n": base64.RawURLEncoding.EncodeToString(pk.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pk.E)).Bytes()),
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"keys": keyList})
	}))
	defer jwksServer.Close()

	discoveryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":   issuerURL,
			"jwks_uri": jwksURL,
		})
	}))
	defer discoveryServer.Close()

	issuerURL = discoveryServer.URL
	jwksURL = jwksServer.URL

	verifier, err := zitadel.NewVerifier(context.Background(), issuerURL)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// Token signed with key-1 should verify.
	token1 := signToken(t, priv1, "key-1", discoveryServer.URL)
	if _, err := verifier.Verify(context.Background(), token1); err != nil {
		t.Fatalf("verify with key-1: %v", err)
	}

	// Add key-2 to the JWKS (simulates key rotation).
	mu.Lock()
	keys["key-2"] = pub2
	mu.Unlock()

	// Force key refresh by waiting for the cache to expire.
	// In production, the 5-minute throttle would prevent immediate refresh.
	// For testing, we bypass by calling refreshKeys directly via a new verifier.
	// Instead, test that a token signed with key-2 fails until refresh:
	token2 := signToken(t, priv2, "key-2", discoveryServer.URL)

	// The verifier should auto-refresh when it encounters an unknown kid.
	if _, err := verifier.Verify(context.Background(), token2); err != nil {
		t.Fatalf("verify with key-2 (auto-refresh): %v", err)
	}
}

func signToken(t *testing.T, priv *rsa.PrivateKey, kid, issuer string) string {
	t.Helper()
	claims := jwt.RegisteredClaims{
		Issuer:    issuer,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Subject:   "user-456",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}


