// Package tenant defines the verified tenant context derived from a
// credential. provider_id and environment_id always come from the
// credential, never from request input.
package tenant

import (
	"context"

	"github.com/google/uuid"
)

// Ctx is the tenant context derived from a verified API-key credential.
type Ctx struct {
	CredentialID    uuid.UUID
	ProviderID      uuid.UUID
	ProviderSlug    string
	LifecycleState  string
	EnvironmentID   uuid.UUID
	EnvironmentKind string
	Issuer          string
	Scopes          []string
}

// HasScope reports whether the credential carries scope s.
func (c Ctx) HasScope(s string) bool {
	for _, sc := range c.Scopes {
		if sc == s {
			return true
		}
	}
	return false
}

type ctxKey struct{}

// WithContext returns a context carrying the tenant context.
func WithContext(ctx context.Context, tc Ctx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tc)
}

// FromContext extracts the tenant context; ok is false when absent.
func FromContext(ctx context.Context) (Ctx, bool) {
	tc, ok := ctx.Value(ctxKey{}).(Ctx)
	return tc, ok
}

type operatorKey struct{}

// WithOperator marks the context as operator-authenticated.
func WithOperator(ctx context.Context) context.Context {
	return context.WithValue(ctx, operatorKey{}, true)
}

// IsOperator reports whether the request was operator-authenticated.
func IsOperator(ctx context.Context) bool {
	v, _ := ctx.Value(operatorKey{}).(bool)
	return v
}
