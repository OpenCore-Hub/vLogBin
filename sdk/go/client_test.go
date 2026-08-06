package vlogbin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientDoAuthAndIdempotency(t *testing.T) {
	var gotAuth, gotIdem string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotIdem = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "key-123")
	var out map[string]any
	err := client.Do(context.Background(), "POST", "/usage/ingest", RequestOptions{
		IdempotencyKey: "tx-1",
	}, map[string]any{"count": 1}, &out)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotAuth != "Bearer key-123" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotIdem != "tx-1" {
		t.Fatalf("Idempotency-Key = %q", gotIdem)
	}
}

func TestClientDecodesErrorEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":        "rate_limited",
				"message":     "slow down",
				"request_id":  "req-1",
				"retry_after": "5",
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	var out map[string]any
	err := client.Do(context.Background(), "GET", "/customers", RequestOptions{}, nil, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Code != "rate_limited" || apiErr.RequestID != "req-1" || apiErr.RetryAfter != "5" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestClientIngestUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/usage/ingest" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") != "tx-1" {
			t.Errorf("Idempotency-Key = %q, want tx-1", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "key")
	result, err := client.IngestUsage(context.Background(), IngestUsageInput{
		TransactionID:      "tx-1",
		CustomerExternalID: "cust-1",
		MetricCode:         "api_calls",
		Timestamp:          "2026-01-01T00:00:00Z",
		Properties:         map[string]any{"count": 1},
	}, RequestOptions{IdempotencyKey: "tx-1"})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("status = %q, want accepted", result.Status)
	}
}
