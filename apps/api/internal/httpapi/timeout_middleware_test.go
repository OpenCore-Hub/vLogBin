package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestTimeoutMiddleware(t *testing.T) {
	t.Run("bounds slow handler", func(t *testing.T) {
		s := &Server{requestTimeout: 20 * time.Millisecond}
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				w.WriteHeader(http.StatusGatewayTimeout)
			case <-time.After(200 * time.Millisecond):
				w.WriteHeader(http.StatusOK)
			}
		})

		w := httptest.NewRecorder()
		start := time.Now()
		s.requestTimeoutMiddleware(next).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusGatewayTimeout {
			t.Fatalf("status = %d, want 504", w.Code)
		}
		if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
			t.Fatalf("handler was not bounded: took %v", elapsed)
		}
	})

	t.Run("passes through fast handler", func(t *testing.T) {
		s := &Server{requestTimeout: time.Second}
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		w := httptest.NewRecorder()
		s.requestTimeoutMiddleware(next).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", w.Code)
		}
	})

	t.Run("disabled when non-positive", func(t *testing.T) {
		s := &Server{requestTimeout: 0}
		hasDeadline := make(chan bool, 1)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, deadline := r.Context().Deadline()
			hasDeadline <- deadline
			w.WriteHeader(http.StatusOK)
		})

		w := httptest.NewRecorder()
		s.requestTimeoutMiddleware(next).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if got := <-hasDeadline; got {
			t.Fatal("no deadline expected when requestTimeout is 0")
		}
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestServiceErrorMapsDeadline(t *testing.T) {
	s := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	s.serviceError(w, r, context.DeadlineExceeded)

	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "upstream_timeout") {
		t.Fatalf("body = %s, want upstream_timeout code", body)
	}
}
