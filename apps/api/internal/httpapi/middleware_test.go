package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
)

func testTenantCtx() tenant.Ctx {
	return tenant.Ctx{
		CredentialID:    uuid.New(),
		ProviderID:      uuid.New(),
		EnvironmentID:   uuid.New(),
		EnvironmentKind: "test",
		Scopes:          []string{"read"},
	}
}

func TestTenantConflictQueryParam(t *testing.T) {
	tc := testTenantCtx()

	r := httptest.NewRequest("GET", "/v1/credentials?provider_id="+uuid.NewString(), nil)
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting provider_id query param must be rejected")
	}

	r = httptest.NewRequest("GET", "/v1/credentials?environment_id="+uuid.NewString(), nil)
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting environment_id query param must be rejected")
	}

	// Matching values are tolerated (they carry no override).
	r = httptest.NewRequest("GET", "/v1/credentials?provider_id="+tc.ProviderID.String(), nil)
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("matching provider_id must not be flagged")
	}

	r = httptest.NewRequest("GET", "/v1/credentials", nil)
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("absent tenant fields must not be flagged")
	}
}

func TestTenantConflictJSONBody(t *testing.T) {
	tc := testTenantCtx()

	body := `{"name":"x","provider_id":"` + uuid.NewString() + `"}`
	r := httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting provider_id in JSON body must be rejected")
	}

	body = `{"name":"x","environment_id":"` + uuid.NewString() + `"}`
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting environment_id in JSON body must be rejected")
	}

	// Body without tenant fields passes, and the body must be restored.
	body = `{"name":"x"}`
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("body without tenant fields must pass")
	}
	buf := make([]byte, len(body))
	n, _ := r.Body.Read(buf)
	if string(buf[:n]) != body {
		t.Fatalf("body not restored: got %q", string(buf[:n]))
	}

	// Malformed JSON is not an override attempt (handler will 400 it).
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(`{not json`))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("malformed JSON must not be flagged as override")
	}

	// Non-JSON content type is not inspected.
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader("provider_id="+uuid.NewString()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("non-JSON body must not be inspected")
	}
}

func TestCIDRAllowed(t *testing.T) {
	if !cidrAllowed(nil, "1.2.3.4") {
		t.Fatal("nil allowlist must allow any IP")
	}
	if !cidrAllowed([]string{}, "1.2.3.4") {
		t.Fatal("empty allowlist must allow any IP")
	}
	if !cidrAllowed([]string{"10.0.0.0/8"}, "10.1.2.3") {
		t.Fatal("IP inside allowlisted CIDR must be allowed")
	}
	if cidrAllowed([]string{"10.0.0.0/8"}, "192.168.1.1") {
		t.Fatal("IP outside allowlisted CIDR must be denied")
	}
	if cidrAllowed([]string{"10.0.0.0/8"}, "not-an-ip") {
		t.Fatal("unparseable IP must be denied")
	}
}

func TestRequireScope(t *testing.T) {
	handler := requireScope("audit:read")(http404())
	tc := testTenantCtx() // scopes: read

	r := httptest.NewRequest("GET", "/v1/audit-events", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), tc)))
	if w.Code != 403 {
		t.Fatalf("missing scope must yield 403, got %d", w.Code)
	}

	tc.Scopes = append(tc.Scopes, "audit:read")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), tc)))
	if w.Code != 404 { // passed through to inner handler
		t.Fatalf("sufficient scope must pass through, got %d", w.Code)
	}
}

func http404() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
}
