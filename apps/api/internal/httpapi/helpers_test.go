package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["hello"] != "world" {
		t.Fatalf("body = %v", body)
	}
}

func TestWriteJSONNil(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "validation_error", "field required", "req-123")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)

	errObj := body["error"].(map[string]any)
	if errObj["code"] != "validation_error" {
		t.Fatalf("code = %v", errObj["code"])
	}
	if errObj["message"] != "field required" {
		t.Fatalf("message = %v", errObj["message"])
	}
	if errObj["request_id"] != "req-123" {
		t.Fatalf("request_id = %v", errObj["request_id"])
	}
}

func TestWriteErrorEmptyRequestID(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusInternalServerError, "internal", "oops", "")

	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)

	errObj := body["error"].(map[string]any)
	if _, exists := errObj["request_id"]; exists {
		t.Fatal("request_id should be absent when empty")
	}
}

func TestReqIDFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, "test-id-456"))

	if id := reqIDFromRequest(r); id != "test-id-456" {
		t.Fatalf("reqID = %q, want test-id-456", id)
	}
}

func TestReqIDFromRequestAbsent(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)

	if id := reqIDFromRequest(r); id != "" {
		t.Fatalf("reqID = %q, want empty", id)
	}
}

func TestDecodeJSONValid(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"test"}`))
	w := httptest.NewRecorder()

	var v struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &v) {
		t.Fatal("decodeJSON should return true for valid JSON")
	}
	if v.Name != "test" {
		t.Fatalf("Name = %q, want test", v.Name)
	}
}

func TestDecodeJSONInvalid(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{invalid json`))
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, "req-id"))
	w := httptest.NewRecorder()

	var v map[string]any
	if decodeJSON(w, r, &v) {
		t.Fatal("decodeJSON should return false for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestDecodeJSONEmpty(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(``))
	w := httptest.NewRecorder()

	var v map[string]any
	if decodeJSON(w, r, &v) {
		t.Fatal("decodeJSON should return false for empty body")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestRequireScopeAllowed(t *testing.T) {
	tc := tenant.Ctx{
		CredentialID:  uuid.New(),
		ProviderID:    uuid.New(),
		EnvironmentID: uuid.New(),
		Scopes:        []string{domain.ScopeRead, domain.ScopeWrite},
	}
	ctx := tenant.WithContext(context.Background(), tc)

	handler := requireScope(domain.ScopeWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("POST", "/v1/test", nil)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (scope present)", w.Code)
	}
}

func TestRequireScopeDenied(t *testing.T) {
	tc := tenant.Ctx{
		CredentialID:  uuid.New(),
		ProviderID:    uuid.New(),
		EnvironmentID: uuid.New(),
		Scopes:        []string{domain.ScopeRead},
	}
	ctx := tenant.WithContext(context.Background(), tc)

	handler := requireScope(domain.ScopeWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("POST", "/v1/test", nil)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (scope missing)", w.Code)
	}
}

func TestRequireScopeNoTenantContext(t *testing.T) {
	handler := requireScope(domain.ScopeRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("POST", "/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no tenant context)", w.Code)
	}
}

func TestDeprecationMiddlewareNoHeaders(t *testing.T) {
	handler := DeprecationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("GET", "/v1/customers", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Deprecation") != "" {
		t.Fatal("active endpoint should not have Deprecation header")
	}
	if w.Header().Get("Sunset") != "" {
		t.Fatal("active endpoint should not have Sunset header")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestDeprecationMiddlewareHeadersAndMetric(t *testing.T) {
	deprecatedAt := time.Now().AddDate(0, -13, 0)
	sunsetAt := deprecatedAt.AddDate(0, 12, 0)
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_api_deprecated_usage_total",
			Help: "test",
		},
		[]string{"path"},
	)
	handler := deprecationMiddlewareWithRegistry(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		[]DeprecationInfo{
			{
				PathPattern:  "GET /v1/old",
				DeprecatedAt: deprecatedAt,
				SunsetAt:     sunsetAt,
				Replacement:  "GET /v1/new",
			},
		},
		counter,
	)

	r := httptest.NewRequest("GET", "/v1/old", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("Deprecation") != "true" {
		t.Fatal("Deprecation header missing")
	}
	if w.Header().Get("Sunset") == "" {
		t.Fatal("Sunset header missing")
	}
	if !strings.Contains(w.Header().Get("Link"), "/v1/new") {
		t.Fatalf("Link header = %q, want successor version", w.Header().Get("Link"))
	}
	value := testutil.ToFloat64(counter.WithLabelValues("/v1/old"))
	if value != 1 {
		t.Fatalf("deprecated usage metric = %v, want 1", value)
	}
}

func TestValidateDeprecationRegistry(t *testing.T) {
	deprecatedAt := time.Now().AddDate(0, -13, 0)
	if err := validateDeprecationRegistry([]DeprecationInfo{
		{
			PathPattern:  "GET /v1/old",
			DeprecatedAt: deprecatedAt,
			SunsetAt:     deprecatedAt.AddDate(0, 12, 0),
		},
	}); err != nil {
		t.Fatalf("valid registry rejected: %v", err)
	}
	if err := validateDeprecationRegistry([]DeprecationInfo{
		{
			PathPattern:  "GET /v1/old",
			DeprecatedAt: deprecatedAt,
			SunsetAt:     deprecatedAt.AddDate(0, 11, 0),
		},
	}); err == nil {
		t.Fatal("sunset before 12 months must fail")
	}
	if err := validateDeprecationRegistry([]DeprecationInfo{
		{
			DeprecatedAt: deprecatedAt,
			SunsetAt:     deprecatedAt.AddDate(0, 12, 0),
		},
	}); err == nil {
		t.Fatal("missing path pattern must fail")
	}
}

func TestMatchDeprecatedPathExact(t *testing.T) {
	if !matchDeprecatedPath("GET /v1/old", "GET /v1/old") {
		t.Fatal("exact match should return true")
	}
	if matchDeprecatedPath("GET /v1/new", "GET /v1/old") {
		t.Fatal("non-matching path should return false")
	}
}

func TestMatchDeprecatedPathWildcard(t *testing.T) {
	if !matchDeprecatedPath("GET /v1/old/anything", "GET /v1/old/*") {
		t.Fatal("wildcard match should return true")
	}
	if !matchDeprecatedPath("GET /v1/old/deep/nested", "GET /v1/old/*") {
		t.Fatal("deep wildcard match should return true")
	}
	if matchDeprecatedPath("GET /v1/new/thing", "GET /v1/old/*") {
		t.Fatal("non-matching wildcard should return false")
	}
}

func TestGetDeprecatedEndpoints(t *testing.T) {
	endpoints := GetDeprecatedEndpoints()
	// Currently no deprecated endpoints — should return empty array.
	if len(endpoints) != 0 {
		t.Fatalf("expected 0 deprecated endpoints, got %d", len(endpoints))
	}
}

func TestBodyLimitMiddleware(t *testing.T) {
	handler := bodyLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Small body — OK
	r := httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 100)))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("small body: status = %d, want 200", w.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	handler := corsMiddleware(func() []string { return []string{"*"} })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Preflight request
	r := httptest.NewRequest("OPTIONS", "/v1/test", nil)
	r.Header.Set("Origin", "https://example.com")
	r.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	// CORS preflight should return 204 or pass through.
	// Verify the CORS header is set on actual requests.
	r2 := httptest.NewRequest("GET", "/v1/test", nil)
	r2.Header.Set("Origin", "https://example.com")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)

	acao := w2.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" && acao != "https://example.com" {
		t.Fatalf("ACAO = %q, want * or origin", acao)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := reqIDFromRequest(r)
		if id == "" {
			t.Fatal("request ID should be set in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("GET", "/v1/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID header should be set")
	}
}

func TestRequestIDMiddlewarePreservesExisting(t *testing.T) {
	existingID := "client-provided-id"
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := reqIDFromRequest(r)
		if id != existingID {
			t.Fatalf("request ID = %q, want %q (client-provided)", id, existingID)
		}
	}))

	r := httptest.NewRequest("GET", "/v1/test", nil)
	r.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Header().Get("X-Request-ID") != existingID {
		t.Fatalf("X-Request-ID = %q, want %q", w.Header().Get("X-Request-ID"), existingID)
	}
}

func TestRecoverMiddleware(t *testing.T) {
	// Verify that recoverMiddleware catches a panic and returns 500.
	srv := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/whatever", nil)

	srv.recoverMiddleware(panicky).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", body)
	}
	if errObj["code"] != "internal" {
		t.Fatalf("error.code = %v, want internal", errObj["code"])
	}
}
