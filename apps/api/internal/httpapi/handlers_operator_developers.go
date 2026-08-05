package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type operatorCreateCredentialRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedBy string     `json:"created_by,omitempty"`
}

// operatorCreateCredential — POST /v1/operator/providers/{id}/credentials?env=test
func (s *Server) operatorCreateCredential(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req operatorCreateCredentialRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := s.svc.CreateProviderCredential(r.Context(), providerID, env, req.Name, req.Scopes, req.ExpiresAt, req.CreatedBy)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"credential": newProviderCredentialRowView(created.Credential),
		"api_key":    created.APIKey,
	})
}

// operatorRotateCredential — POST /v1/operator/providers/{id}/credentials/{credentialId}/rotate?env=test
//
// Atomic rotation: the old key is revoked and a replacement with the same
// name/scopes/expiry is issued in one transaction. The new plaintext is
// returned exactly once; the old key stops working immediately.
func (s *Server) operatorRotateCredential(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	credentialID, err := uuid.Parse(chi.URLParam(r, "credentialId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_credential_id", "credential id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req struct {
		CreatedBy string `json:"created_by"`
	}
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}
	created, err := s.svc.RotateProviderCredential(r.Context(), providerID, env, credentialID, req.CreatedBy)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"credential": newProviderCredentialRowView(created.Credential),
		"api_key":    created.APIKey,
	})
}

// operatorEventView is the Console-safe event representation: JSONB payload
// is decoded to raw JSON instead of base64, and environment metadata is
// resolved for the active environment.
type operatorEventView struct {
	ID              uuid.UUID       `json:"id"`
	ProviderID      uuid.UUID       `json:"provider_id"`
	EnvironmentID   uuid.UUID       `json:"environment_id"`
	EnvironmentKind string          `json:"environment_kind"`
	AggregateType   string          `json:"aggregate_type"`
	AggregateID     string          `json:"aggregate_id"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	PayloadHash     string          `json:"payload_hash"`
	TransactionID   string          `json:"transaction_id"`
	Status          string          `json:"status"`
	Attempts        int32           `json:"attempts"`
	CreatedAt       time.Time       `json:"created_at"`
	PublishedAt     *time.Time      `json:"published_at,omitempty"`
	NextAttemptAt   *time.Time      `json:"next_attempt_at,omitempty"`
	LastError       string          `json:"last_error,omitempty"`
}

func newOperatorEventView(e storegen.OutboxEvent, envKind string) operatorEventView {
	v := operatorEventView{
		ID:              e.ID,
		ProviderID:      e.ProviderID,
		EnvironmentID:   e.EnvironmentID,
		EnvironmentKind: envKind,
		AggregateType:   e.AggregateType,
		AggregateID:     e.AggregateID,
		EventType:       e.EventType,
		Payload:         json.RawMessage(e.Payload),
		PayloadHash:     e.PayloadHash,
		TransactionID:   e.TransactionID,
		Status:          e.Status,
		Attempts:        e.Attempts,
		CreatedAt:       e.CreatedAt,
		PublishedAt:     e.PublishedAt,
		NextAttemptAt:   e.NextAttemptAt,
		LastError:       e.LastError.String,
	}
	if !e.LastError.Valid {
		v.LastError = ""
	}
	return v
}

// operatorStreamEvents — GET /v1/operator/providers/{id}/events?env=test
//
// Cursor-based forward pagination mirroring the provider-domain /v1/events
// contract for the Console Events page.
func (s *Server) operatorStreamEvents(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}

	cursor := uuid.Nil
	if c := r.URL.Query().Get("cursor"); c != "" {
		var err error
		cursor, err = uuid.Parse(c)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor must be a uuid", reqIDFromRequest(r))
			return
		}
	}

	eventType := r.URL.Query().Get("type")
	aggregateType := r.URL.Query().Get("aggregate_type")
	limit := int32(100)
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = int32(n)
		}
	}

	result, err := s.svc.StreamEventsByProvider(r.Context(), providerID, env.ID, cursor, eventType, aggregateType, limit)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]operatorEventView, 0, len(result.Events))
	for _, e := range result.Events {
		views = append(views, newOperatorEventView(e, env.Kind))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":      views,
		"next_cursor": result.NextCursor,
		"has_more":    result.HasMore,
	})
}
