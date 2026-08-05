package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	securityHeadersMiddleware(next).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/x", nil))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
		"Cache-Control":          "no-store",
	}
	for hdr, val := range want {
		if got := w.Header().Get(hdr); got != val {
			t.Errorf("%s = %q, want %q", hdr, got, val)
		}
	}
}

func TestRedactQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no sensitive", "limit=10&offset=20", "limit=10&offset=20"},
		{"token", "token=abc&limit=10", "limit=10&token=REDACTED"},
		{"access token", "access_token=abc&limit=10", "access_token=REDACTED&limit=10"},
		{"api key", "api_key=abc", "api_key=REDACTED"},
		{"password", "password=hunter2", "password=REDACTED"},
		{"signature", "sig=xyz&ts=123", "sig=REDACTED&ts=123"},
		{"suffix separated", "x-api-key=abc", "x-api-key=REDACTED"},
		{"case insensitive", "TOKEN=abc", "TOKEN=REDACTED"},
		{"sorted output", "z=1&a=2", "a=2&z=1"},
		{"multi value collapsed", "token=a&token=b", "token=REDACTED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactQuery(tt.in); got != tt.want {
				t.Errorf("redactQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSlowRequestEscalation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s := &Server{log: logger}
	s.SetSlowRequestThreshold(time.Nanosecond)

	r := chi.NewRouter()
	r.Use(s.requestLogMiddleware)
	r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	out := buf.String()
	if !strings.Contains(out, "slow=true") {
		t.Fatalf("expected slow=true marker, log:\n%s", out)
	}
	if !strings.Contains(out, "level=WARN") {
		t.Fatalf("expected WARN level for slow request, log:\n%s", out)
	}
}

func TestFastRequestNotEscalated(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s := &Server{log: logger}
	s.SetSlowRequestThreshold(10 * time.Second)

	r := chi.NewRouter()
	r.Use(s.requestLogMiddleware)
	r.Get("/fast", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fast", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(buf.String(), "slow=true") {
		t.Fatalf("fast request must not be flagged slow, log:\n%s", buf.String())
	}
}

func TestRequestLogRedactsQuery(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	s := &Server{log: logger}

	r := chi.NewRouter()
	r.Use(s.requestLogMiddleware)
	r.Get("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/search?token=supersecret&limit=5", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	out := buf.String()
	if strings.Contains(out, "supersecret") {
		t.Fatalf("log leaked sensitive token value:\n%s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected redacted query in log:\n%s", out)
	}
}
