package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"provider_id":      tc.ProviderID,
		"slug":             tc.ProviderSlug,
		"environment_kind": tc.EnvironmentKind,
		"environment_id":   tc.EnvironmentID,
		"issuer":           tc.Issuer,
		"lifecycle_state":  tc.LifecycleState,
		"scopes":           tc.Scopes,
	})
}

// credentialView is the safe external representation of a credential: the
// key hash is never exposed.
type credentialView struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	KeyPrefix    string     `json:"key_prefix"`
	Scopes       []string   `json:"scopes"`
	AllowedCIDRs []string   `json:"allowed_cidrs,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func toCredentialView(c storegen.Credential) credentialView {
	return credentialView{
		ID:           c.ID,
		Name:         c.Name,
		KeyPrefix:    c.KeyPrefix,
		Scopes:       c.Scopes,
		AllowedCIDRs: c.AllowedCidrs,
		ExpiresAt:    c.ExpiresAt,
		RevokedAt:    c.RevokedAt,
		LastUsedAt:   c.LastUsedAt,
		CreatedAt:    c.CreatedAt,
	}
}

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	creds, err := s.svc.ListCredentials(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]credentialView, 0, len(creds))
	for _, c := range creds {
		views = append(views, toCredentialView(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": views})
}

type createCredentialRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createCredentialRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := s.svc.CreateCredential(r.Context(), tc, req.Name, req.Scopes, req.ExpiresAt)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"credential": toCredentialView(created.Credential),
		"api_key":    created.APIKey, // plaintext, returned exactly once
	})
}

func (s *Server) revokeCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "credential id must be a uuid", reqIDFromRequest(r))
		return
	}
	cred, err := s.svc.RevokeCredential(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credential": toCredentialView(*cred)})
}

// auditQueryError reports a client-side parameter problem with an API error
// code the operator console can surface directly.
type auditQueryError struct{ code, msg string }

func (e auditQueryError) Error() string { return e.msg }

// auditFilters holds the six filter dimensions shared by list and stats
// audit endpoints. Empty strings skip a filter; nil From/To leave the bound
// open.
type auditFilters struct {
	Action     string
	ActorType  string
	ActorID    string
	TargetType string
	TargetID   string
	From, To   *time.Time
}

// parseAuditFilters reads the common filter dimensions from query parameters.
func parseAuditFilters(r *http.Request) (auditFilters, error) {
	var f auditFilters
	f.Action = r.URL.Query().Get("action")
	f.ActorType = r.URL.Query().Get("actor_type")
	f.ActorID = r.URL.Query().Get("actor_id")
	f.TargetType = r.URL.Query().Get("target_type")
	f.TargetID = r.URL.Query().Get("target_id")
	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, auditQueryError{code: "invalid_from", msg: "from must be an RFC3339 timestamp"}
		}
		f.From = &t
	}
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, auditQueryError{code: "invalid_to", msg: "to must be an RFC3339 timestamp"}
		}
		f.To = &t
	}
	return f, nil
}

// auditQueryFromRequest parses pagination and filter parameters for audit log
// endpoints. An absent cursor starts from the newest row; empty filters and
// absent time bounds are left open.
func auditQueryFromRequest(r *http.Request, fallback int32) (service.AuditQuery, error) {
	q := service.AuditQuery{Limit: queryLimit(r, fallback)}
	if v := r.URL.Query().Get("cursor"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return q, auditQueryError{code: "invalid_cursor", msg: "cursor must be a positive audit event id"}
		}
		q.Cursor = id
	}
	f, err := parseAuditFilters(r)
	if err != nil {
		return q, err
	}
	q.Action, q.ActorType, q.ActorID, q.TargetType, q.TargetID = f.Action, f.ActorType, f.ActorID, f.TargetType, f.TargetID
	q.From, q.To = f.From, f.To
	return q, nil
}

// maxAuditStatsWindow bounds the dashboard aggregation window. A stats query
// without a bounded range would aggregate the whole trail; one calendar year
// of audit history is the generous ceiling that still prevents that.
const maxAuditStatsWindow = 366 * 24 * time.Hour

// auditStatsQueryFromRequest parses the dashboard query. Unlike the list
// endpoints, from and to are required here: an unbounded aggregate is a
// production footgun, and the dashboard always renders a specific window.
func auditStatsQueryFromRequest(r *http.Request) (service.AuditStatsQuery, error) {
	var q service.AuditStatsQuery
	f, err := parseAuditFilters(r)
	if err != nil {
		return q, err
	}
	q.Action, q.ActorType, q.ActorID, q.TargetType, q.TargetID = f.Action, f.ActorType, f.ActorID, f.TargetType, f.TargetID
	q.From, q.To = f.From, f.To
	if q.From == nil {
		return q, auditQueryError{code: "missing_from", msg: "from is required for audit stats"}
	}
	if q.To == nil {
		return q, auditQueryError{code: "missing_to", msg: "to is required for audit stats"}
	}
	if q.From.After(*q.To) {
		return q, auditQueryError{code: "invalid_range", msg: "from must not be after to"}
	}
	if q.To.Sub(*q.From) > maxAuditStatsWindow {
		return q, auditQueryError{code: "range_too_wide", msg: "audit stats window is limited to 366 days"}
	}
	switch g := r.URL.Query().Get("granularity"); g {
	case "hour", "day", "week":
		q.Granularity = g
	case "":
		q.Granularity = "day"
	default:
		return q, auditQueryError{code: "invalid_granularity", msg: "granularity must be one of hour, day, week"}
	}
	return q, nil
}

func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	q, err := auditQueryFromRequest(r, 100)
	if err != nil {
		writeAuditQueryError(w, r, err)
		return
	}
	events, next, err := s.svc.ListAuditEvents(r.Context(), tc, q)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_events": events, "next_cursor": next})
}

// auditEventStats renders the tenant audit dashboard aggregates. The request
// must carry a bounded from/to window and an optional granularity; a bounded
// window is what keeps the aggregation off a full-table scan.
func (s *Server) auditEventStats(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	q, err := auditStatsQueryFromRequest(r)
	if err != nil {
		writeAuditQueryError(w, r, err)
		return
	}
	stats, err := s.svc.AuditEventStats(r.Context(), tc, q)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// auditExportQueryFromRequest parses the export query. As with the stats
// endpoint, from and to are required: an unbounded export would stream the
// whole trail, which is the same full-table-scan footgun the stats window
// guard prevents. format is one of csv (default) or json.
func auditExportQueryFromRequest(r *http.Request) (service.AuditExportQuery, string, error) {
	var q service.AuditExportQuery
	f, err := parseAuditFilters(r)
	if err != nil {
		return q, "", err
	}
	q.Action, q.ActorType, q.ActorID, q.TargetType, q.TargetID = f.Action, f.ActorType, f.ActorID, f.TargetType, f.TargetID
	q.From, q.To = f.From, f.To
	if q.From == nil {
		return q, "", auditQueryError{code: "missing_from", msg: "from is required for audit export"}
	}
	if q.To == nil {
		return q, "", auditQueryError{code: "missing_to", msg: "to is required for audit export"}
	}
	if q.From.After(*q.To) {
		return q, "", auditQueryError{code: "invalid_range", msg: "from must not be after to"}
	}
	if q.To.Sub(*q.From) > maxAuditStatsWindow {
		return q, "", auditQueryError{code: "range_too_wide", msg: "audit export window is limited to 366 days"}
	}
	format := r.URL.Query().Get("format")
	switch format {
	case "", "csv":
		format = "csv"
	case "json":
		// ok
	default:
		return q, "", auditQueryError{code: "invalid_format", msg: "format must be one of csv, json"}
	}
	return q, format, nil
}

// auditEventExport streams the tenant's audit trail as a downloadable file.
// The response is flushed row by row so a large window never buffers entirely
// in memory; a mid-stream failure (e.g. a dropped client) simply truncates the
// download rather than failing the whole request.
func (s *Server) auditEventExport(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	q, format, err := auditExportQueryFromRequest(r)
	if err != nil {
		writeAuditQueryError(w, r, err)
		return
	}
	s.streamAuditExport(w, r, format, func(emit func(storegen.AuditEvent) error) error {
		return s.svc.ExportAuditEvents(r.Context(), tc, q, emit)
	})
}

// auditExportCSVHeader is the fixed CSV column order. Columns are stable and
// documented so downstream tooling (SIEM ingestion, spreadsheets) can rely on
// the schema.
var auditExportCSVHeader = []string{"id", "created_at", "actor_type", "actor_id", "action", "target_type", "target_id", "request_id", "metadata"}

// auditExportCSVRow renders one audit event as a CSV record. Cells are plain
// text; the metadata JSONB blob is embedded verbatim so nothing is lost.
func auditExportCSVRow(ev storegen.AuditEvent) []string {
	targetType, targetID, requestID := "", "", ""
	if ev.TargetType.Valid {
		targetType = ev.TargetType.String
	}
	if ev.TargetID.Valid {
		targetID = ev.TargetID.String
	}
	if ev.RequestID.Valid {
		requestID = ev.RequestID.String
	}
	return []string{
		strconv.FormatInt(ev.ID, 10),
		ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		ev.ActorType,
		ev.ActorID,
		ev.Action,
		targetType,
		targetID,
		requestID,
		string(ev.Metadata),
	}
}

// streamAuditExport writes audit events in the requested format, flushing row
// by row so a large export streams rather than buffering. load drives the
// service-side pagination loop, invoking emit once per event.
func (s *Server) streamAuditExport(w http.ResponseWriter, r *http.Request, format string, load func(emit func(storegen.AuditEvent) error) error) {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit-export-%s.csv"`, stamp))
		cw := csv.NewWriter(w)
		if err := cw.Write(auditExportCSVHeader); err != nil {
			s.log.Error("audit export: write csv header", "error", err, "request_id", reqIDFromRequest(r))
			return
		}
		cw.Flush()
		if err := load(func(ev storegen.AuditEvent) error {
			if err := cw.Write(auditExportCSVRow(ev)); err != nil {
				return err
			}
			cw.Flush()
			return cw.Error()
		}); err != nil {
			s.log.Error("audit export aborted", "error", err, "request_id", reqIDFromRequest(r))
			return
		}
		cw.Flush()
		flush()
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit-export-%s.json"`, stamp))
		first := true
		if _, err := w.Write([]byte("[")); err != nil {
			s.log.Error("audit export: write json array open", "error", err, "request_id", reqIDFromRequest(r))
			return
		}
		if err := load(func(ev storegen.AuditEvent) error {
			if !first {
				if _, err := w.Write([]byte(",")); err != nil {
					return err
				}
			}
			first = false
			if err := json.NewEncoder(w).Encode(newAuditEventView(ev)); err != nil {
				return err
			}
			flush()
			return nil
		}); err != nil {
			s.log.Error("audit export aborted", "error", err, "request_id", reqIDFromRequest(r))
			return
		}
		if _, err := w.Write([]byte("]")); err != nil {
			s.log.Error("audit export: write json array close", "error", err, "request_id", reqIDFromRequest(r))
			return
		}
		flush()
	default:
		writeError(w, http.StatusBadRequest, "invalid_format", "format must be one of csv, json", reqIDFromRequest(r))
	}
}

func (s *Server) listOutboxEvents(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	events, err := s.svc.ListOutboxEvents(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"outbox_events": events})
}

func queryLimit(r *http.Request, fallback int32) int32 {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			return int32(n)
		}
	}
	return fallback
}
