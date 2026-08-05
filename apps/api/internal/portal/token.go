// Package portal issues and verifies customer-scoped portal tokens.
//
// A portal token is a short-lived HMAC-signed JWT carrying the exact data
// domain (provider + environment + customer external id). The web frontend
// stores it in a separate customer cookie and presents it as a Bearer token
// to /v1/portal/* endpoints; the API never derives tenant context from
// unauthenticated input.
package portal

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the customer-scoped portal identity.
type Claims struct {
	ProviderID         string `json:"provider_id"`
	EnvironmentID      string `json:"environment_id"`
	EnvironmentKind    string `json:"environment_kind"`
	CustomerExternalID string `json:"customer_external_id"`
	jwt.RegisteredClaims
}

// Issuer signs and verifies portal tokens with a shared HMAC secret.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

// NewIssuer returns an Issuer. The secret must be at least 32 bytes so a
// deployment cannot accidentally configure a trivially forgeable token.
func NewIssuer(secret string, ttl time.Duration) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, errors.New("portal token secret must be at least 32 bytes")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Issuer{secret: []byte(secret), ttl: ttl}, nil
}

// Issue signs a portal token for one customer in one provider environment.
func (i *Issuer) Issue(
	providerID, environmentID uuid.UUID,
	environmentKind, customerExternalID string,
) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(i.ttl)
	claims := Claims{
		ProviderID:         providerID.String(),
		EnvironmentID:      environmentID.String(),
		EnvironmentKind:    environmentKind,
		CustomerExternalID: customerExternalID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "vlogbin-portal",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign portal token: %w", err)
	}
	return signed, expiresAt, nil
}

// Verify validates a portal token signature and expiration.
func (i *Issuer) Verify(raw string) (*Claims, error) {
	var claims Claims
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
		}
		return i.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("vlogbin-portal"))
	if err != nil {
		return nil, fmt.Errorf("invalid portal token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("invalid portal token")
	}
	if claims.ProviderID == "" || claims.EnvironmentID == "" || claims.CustomerExternalID == "" {
		return nil, errors.New("portal token missing data domain claims")
	}
	return &claims, nil
}
