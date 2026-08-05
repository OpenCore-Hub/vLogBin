package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueVerify(t *testing.T) {
	issuer, err := NewIssuer("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	providerID := uuid.New()
	envID := uuid.New()
	token, expiresAt, err := issuer.Issue(providerID, envID, "test", "cust-1")
	if err != nil {
		t.Fatal(err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("expires_at must be in the future")
	}
	claims, err := issuer.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ProviderID != providerID.String() || claims.EnvironmentID != envID.String() {
		t.Fatalf("claims domain mismatch: %+v", claims)
	}
	if claims.CustomerExternalID != "cust-1" {
		t.Fatalf("customer = %q", claims.CustomerExternalID)
	}
}

func TestVerifyRejectsTamperedToken(t *testing.T) {
	issuer, _ := NewIssuer("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", time.Hour)
	token, _, err := issuer.Issue(uuid.New(), uuid.New(), "test", "cust-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuer.Verify(token + "x"); err == nil {
		t.Fatal("tampered token must be rejected")
	}
}

func TestNewIssuerRejectsShortSecret(t *testing.T) {
	if _, err := NewIssuer("short", time.Hour); err == nil {
		t.Fatal("short secret must be rejected")
	}
}
