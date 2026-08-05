package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newIdemTestServer returns a Server without a store; only the middleware
// branches that do not touch the database are exercised here (the full
// replay/claim flow lives in internal/integration/idempotency_test.go).
func newIdemTestServer(t *testing.T) *Server {
	t.Helper()
	return NewServer(nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func idemRequest(t *testing.T, s *Server, method, path, key string) *httptest.ResponseRecorder {
	t.Helper()
	rq := httptest.NewRequest(method, path, nil)
	if key != "" {
		rq.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	s.idempotencyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("executed"))
	})).ServeHTTP(w, rq)
	return w
}

func TestIdempotencyMiddlewareNoKey(t *testing.T) {
	s := newIdemTestServer(t)
	w := idemRequest(t, s, http.MethodPost, "/x", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "executed" {
		t.Fatalf("body = %q, want %q (handler must run when no key is sent)", got, "executed")
	}
	if got := w.Header().Get("Idempotency-Replayed"); got != "" {
		t.Fatalf("Idempotency-Replayed = %q, want empty for first execution", got)
	}
}

func TestIdempotencyMiddlewareReadMethodsPassThrough(t *testing.T) {
	s := newIdemTestServer(t)
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		w := idemRequest(t, s, method, "/x", "key-1")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", method, w.Code)
		}
		if got := w.Body.String(); got != "executed" {
			t.Fatalf("%s: body = %q, want %q (read methods must pass through)", method, got, "executed")
		}
	}
}

func TestIdempotencyMiddlewareInvalidKey(t *testing.T) {
	s := newIdemTestServer(t)
	// Control characters and values over 255 bytes are rejected; the handler
	// must not run and the client must get 400 invalid_idempotency_key.
	for _, key := range []string{"a b", "a\nb", strings.Repeat("x", 256)} {
		w := idemRequest(t, s, http.MethodPost, "/x", key)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("key %q: status = %d, want 400", key, w.Code)
		}
		if got := w.Body.String(); got == "executed" {
			t.Fatalf("key %q: handler ran, want rejection", key)
		}
	}
}

func TestValidIdempotencyKey(t *testing.T) {
	ok := []string{"a", strings.Repeat("x", 255), "key-123", "0123456789abcdef"}
	for _, key := range ok {
		if !validIdempotencyKey(key) {
			t.Fatalf("validIdempotencyKey(%q) = false, want true", key)
		}
	}
	bad := []string{"", strings.Repeat("x", 256), "has space", "tab\there", "new\nline", "non-ascii-é"}
	for _, key := range bad {
		if validIdempotencyKey(key) {
			t.Fatalf("validIdempotencyKey(%q) = true, want false", key)
		}
	}
}
