package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartupProbeBeforeComplete(t *testing.T) {
	srv := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/startup", nil)

	srv.startup(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 before SetStartupComplete", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "starting" {
		t.Fatalf("status field = %v, want starting", body["status"])
	}
}

func TestStartupProbeAfterComplete(t *testing.T) {
	srv := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	srv.SetStartupComplete()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/startup", nil)
	srv.startup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after SetStartupComplete", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "started" {
		t.Fatalf("status field = %v, want started", body["status"])
	}
}

func TestHealthProbe(t *testing.T) {
	srv := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)

	srv.health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status field = %v, want ok", body["status"])
	}
}
