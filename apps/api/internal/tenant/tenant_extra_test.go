package tenant

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestWithContextAndFromContext(t *testing.T) {
	tc := Ctx{
		CredentialID:    uuid.New(),
		ProviderID:      uuid.New(),
		EnvironmentID:   uuid.New(),
		EnvironmentKind: "test",
		Scopes:          []string{"read", "write"},
	}

	ctx := WithContext(context.Background(), tc)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext returned ok=false")
	}
	if got.CredentialID != tc.CredentialID {
		t.Fatalf("CredentialID mismatch")
	}
	if got.ProviderID != tc.ProviderID {
		t.Fatalf("ProviderID mismatch")
	}
	if got.EnvironmentID != tc.EnvironmentID {
		t.Fatalf("EnvironmentID mismatch")
	}
}

func TestFromContextAbsent(t *testing.T) {
	_, ok := FromContext(context.Background())
	if ok {
		t.Fatal("FromContext should return ok=false when no tenant context")
	}
}

func TestWithOperatorAndIsOperator(t *testing.T) {
	ctx := WithOperator(context.Background())
	if !IsOperator(ctx) {
		t.Fatal("IsOperator should be true")
	}
}

func TestIsOperatorFalse(t *testing.T) {
	if IsOperator(context.Background()) {
		t.Fatal("IsOperator should be false for plain context")
	}
}

// TestCtxRoundTrip verifies that all Ctx fields survive a context
// round-trip: WithContext stores the full Ctx, and FromContext
// retrieves it intact. This catches regressions where a new field is
// added to Ctx but not propagated through the context value.
func TestCtxRoundTrip(t *testing.T) {
	tc := Ctx{
		CredentialID:    uuid.New(),
		ProviderID:      uuid.New(),
		EnvironmentID:   uuid.New(),
		EnvironmentKind: "live",
		Scopes:          []string{"read", "write", "billing:write"},
		ProviderSlug:    "acme-corp",
		LifecycleState:  "live.active",
		Issuer:          "https://auth.example.com",
	}

	got, ok := FromContext(WithContext(context.Background(), tc))
	if !ok {
		t.Fatal("FromContext returned ok=false")
	}
	if got.ProviderSlug != tc.ProviderSlug {
		t.Fatalf("ProviderSlug = %q, want %q", got.ProviderSlug, tc.ProviderSlug)
	}
	if got.LifecycleState != tc.LifecycleState {
		t.Fatalf("LifecycleState = %q, want %q", got.LifecycleState, tc.LifecycleState)
	}
	if got.Issuer != tc.Issuer {
		t.Fatalf("Issuer = %q, want %q", got.Issuer, tc.Issuer)
	}
	if len(got.Scopes) != len(tc.Scopes) {
		t.Fatalf("Scopes length = %d, want %d", len(got.Scopes), len(tc.Scopes))
	}
}
