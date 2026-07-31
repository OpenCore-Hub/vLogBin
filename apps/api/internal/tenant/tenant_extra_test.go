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

func TestCtxProviderSlug(t *testing.T) {
	tc := Ctx{ProviderSlug: "acme-corp"}
	if tc.ProviderSlug != "acme-corp" {
		t.Fatalf("ProviderSlug = %q", tc.ProviderSlug)
	}
}

func TestCtxLifecycleState(t *testing.T) {
	tc := Ctx{LifecycleState: "live.active"}
	if tc.LifecycleState != "live.active" {
		t.Fatalf("LifecycleState = %q", tc.LifecycleState)
	}
}

func TestCtxIssuer(t *testing.T) {
	tc := Ctx{Issuer: "https://auth.example.com"}
	if tc.Issuer != "https://auth.example.com" {
		t.Fatalf("Issuer = %q", tc.Issuer)
	}
}
