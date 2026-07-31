// Package zitadel provides OIDC token verification for Hosted Auth
// (spec Section 8). The verifier fetches ZITADEL's JWKS from the
// well-known discovery endpoint and verifies JWT access tokens.
package zitadel

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// discoveryDocument represents the OIDC well-known configuration.
type discoveryDocument struct {
	Issuer  string `json:"issuer"`
	JwksURI string `json:"jwks_uri"`
}

// jwksKey represents a single key in a JWKS response.
type jwksKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwksResponse represents the JWKS endpoint response.
type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// Claims contains the standard OIDC claims extracted from the access token.
type Claims struct {
	Sub           string   `json:"sub"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	Roles         []string `json:"roles"`
}

// Verifier verifies OIDC access tokens issued by ZITADEL. It caches
// ZITADEL's public keys and refreshes them periodically.
type Verifier struct {
	issuer     string
	jwksURL    string
	keys       map[string]*rsa.PublicKey
	keysMu     sync.RWMutex
	httpClient *http.Client
	lastRefresh time.Time
}

// NewVerifier discovers the ZITADEL OIDC configuration and fetches
// the initial JWKS. The issuerURL should be the base URL of the
// ZITADEL instance (e.g. http://localhost:8080).
func NewVerifier(ctx context.Context, issuerURL string) (*Verifier, error) {
	v := &Verifier{
		issuer: strings.TrimRight(issuerURL, "/"),
		keys:   make(map[string]*rsa.PublicKey),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Fetch discovery document.
	wellKnown := v.issuer + "/.well-known/openid-configuration"
	doc, err := v.fetchJSON(ctx, wellKnown)
	if err != nil {
		return nil, fmt.Errorf("fetch discovery: %w", err)
	}
	var disc discoveryDocument
	if err := json.Unmarshal(doc, &disc); err != nil {
		return nil, fmt.Errorf("parse discovery: %w", err)
	}
	if disc.JwksURI == "" {
		return nil, fmt.Errorf("discovery: jwks_uri is empty")
	}
	v.jwksURL = disc.JwksURI

	// Fetch initial JWKS.
	if err := v.refreshKeys(ctx); err != nil {
		return nil, fmt.Errorf("initial jwks fetch: %w", err)
	}

	return v, nil
}

// Verify validates an OIDC access token and returns the claims.
// It verifies the signature, issuer, and expiry. If the signing key
// is not in the cache, it refreshes the JWKS once and retries.
func (v *Verifier) Verify(ctx context.Context, tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, &jwtCustomClaims{}, func(token *jwt.Token) (any, error) {
		// Ensure the token uses RSA signing.
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("token missing kid header")
		}

		// Look up the key.
		v.keysMu.RLock()
		key, ok := v.keys[kid]
		v.keysMu.RUnlock()
		if !ok {
			// Key not found — force refresh the JWKS (bypass throttle).
			if refreshErr := v.forceRefreshKeys(ctx); refreshErr != nil {
				return nil, fmt.Errorf("key %s not found and refresh failed: %w", kid, refreshErr)
			}
			v.keysMu.RLock()
			key, ok = v.keys[kid]
			v.keysMu.RUnlock()
			if !ok {
				return nil, fmt.Errorf("key %s not found after refresh", kid)
			}
		}
		return key, nil
	},
		jwt.WithIssuer(v.issuer),
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
	)
	if err != nil {
		return nil, fmt.Errorf("verify token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	// Extract custom claims.
	if custom, ok := token.Claims.(*jwtCustomClaims); ok {
		claims.Sub = custom.Sub
		claims.Email = custom.Email
		claims.EmailVerified = custom.EmailVerified
		claims.Name = custom.Name
		claims.Roles = custom.Roles
	}

	return claims, nil
}

// jwtCustomClaims implements jwt.Claims with standard OIDC fields.
type jwtCustomClaims struct {
	Sub           string   `json:"sub"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Name          string   `json:"name"`
	Roles         []string `json:"urn:zitadel:iam:roles"`
	jwt.RegisteredClaims
}

// refreshKeys fetches the JWKS and updates the key cache, throttled to
// once per 5 minutes. Safe to call concurrently.
func (v *Verifier) refreshKeys(ctx context.Context) error {
	v.keysMu.RLock()
	age := time.Since(v.lastRefresh)
	v.keysMu.RUnlock()
	if age < 5*time.Minute {
		return nil
	}
	return v.forceRefreshKeys(ctx)
}

// forceRefreshKeys fetches the JWKS and updates the key cache, bypassing
// the throttle. Used when a key is not found (key rotation scenario).
func (v *Verifier) forceRefreshKeys(ctx context.Context) error {
	data, err := v.fetchJSON(ctx, v.jwksURL)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}

	var resp jwksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse jwks: %w", err)
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, k := range resp.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := jwkToRSAPublicKey(k.N, k.E)
		if err != nil {
			continue // skip invalid keys
		}
		newKeys[k.Kid] = pub
	}
	if len(newKeys) == 0 {
		return fmt.Errorf("no RSA keys found in JWKS")
	}

	v.keysMu.Lock()
	v.keys = newKeys
	v.lastRefresh = time.Now()
	v.keysMu.Unlock()
	return nil
}

func (v *Verifier) fetchJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// jwkToRSAPublicKey constructs an RSA public key from base64url-encoded
// JWK modulus (n) and exponent (e).
func jwkToRSAPublicKey(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() {
		return nil, fmt.Errorf("exponent too large")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}
