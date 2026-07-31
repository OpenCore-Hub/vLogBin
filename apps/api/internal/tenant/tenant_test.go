package tenant

import (
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/google/uuid"
)

func TestCtxHasScope(t *testing.T) {
	providerID := uuid.New()
	envID := uuid.New()
	credID := uuid.New()

	tc := Ctx{
		ProviderID:    providerID,
		EnvironmentID: envID,
		CredentialID:  credID,
		Scopes:        []string{domain.ScopeRead, domain.ScopeWrite},
	}

	if !tc.HasScope(domain.ScopeRead) {
		t.Fatal("HasScope(read) should be true")
	}
	if !tc.HasScope(domain.ScopeWrite) {
		t.Fatal("HasScope(write) should be true")
	}
	if tc.HasScope(domain.ScopeCredentialsManage) {
		t.Fatal("HasScope(credentials:manage) should be false")
	}
}

func TestCtxHasScopeEmpty(t *testing.T) {
	tc := Ctx{
		Scopes: []string{},
	}

	if tc.HasScope(domain.ScopeRead) {
		t.Fatal("HasScope with empty scopes should be false")
	}
}

func TestCtxHasScopeNil(t *testing.T) {
	tc := Ctx{}

	if tc.HasScope(domain.ScopeRead) {
		t.Fatal("HasScope with nil scopes should be false")
	}
}

func TestCtxProviderNullUUID(t *testing.T) {
	providerID := uuid.New()
	tc := Ctx{ProviderID: providerID}

	nu := tc.ProviderNullUUID()
	if !nu.Valid || nu.UUID != providerID {
		t.Fatalf("ProviderNullUUID = %v, want %s", nu, providerID)
	}
}

func TestCtxEnvironmentNullUUID(t *testing.T) {
	envID := uuid.New()
	tc := Ctx{EnvironmentID: envID}

	nu := tc.EnvironmentNullUUID()
	if !nu.Valid || nu.UUID != envID {
		t.Fatalf("EnvironmentNullUUID = %v, want %s", nu, envID)
	}
}
