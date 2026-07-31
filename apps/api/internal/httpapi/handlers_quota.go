package httpapi

import (
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type setQuotaLimitRequest struct {
	QuotaKey   string `json:"quota_key"`
	LimitValue int64  `json:"limit_value"`
	PeriodType string `json:"period_type"`
}

// setQuotaLimit — PUT /v1/subscriptions/{id}/quota-limits/{key}
func (s *Server) setQuotaLimit(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req setQuotaLimitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// quota_key from URL takes precedence; fall back to body.
	key := chi.URLParam(r, "key")
	if key == "" {
		key = req.QuotaKey
	}
	limit, err := s.svc.SetQuotaLimit(r.Context(), tc, service.SetQuotaLimitInput{
		SubscriptionID: subID,
		QuotaKey:       key,
		LimitValue:     req.LimitValue,
		PeriodType:     req.PeriodType,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, limit)
}

// getQuotaLimit — GET /v1/subscriptions/{id}/quota-limits/{key}
func (s *Server) getQuotaLimit(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	key := chi.URLParam(r, "key")
	limit, err := s.svc.GetQuotaLimit(r.Context(), tc, subID, key)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, limit)
}

// listQuotaLimits — GET /v1/subscriptions/{id}/quota-limits
func (s *Server) listQuotaLimits(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	limits, err := s.svc.ListQuotaLimits(r.Context(), tc, subID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if limits == nil {
		limits = []storegen.QuotaLimit{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"quota_limits": limits})
}

// deleteQuotaLimit — DELETE /v1/subscriptions/{id}/quota-limits/{key}
func (s *Server) deleteQuotaLimit(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	key := chi.URLParam(r, "key")
	if err := s.svc.DeleteQuotaLimit(r.Context(), tc, subID, key); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reserveQuotaRequest struct {
	QuotaKey      string `json:"quota_key"`
	Amount        int64  `json:"amount"`
	ReservationID string `json:"reservation_id"`
	ExpiresInSec  int    `json:"expires_in_seconds"`
}

// reserveQuota — POST /v1/subscriptions/{id}/quota/reserve
func (s *Server) reserveQuota(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req reserveQuotaRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var expiresAt *time.Time
	if req.ExpiresInSec > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresInSec) * time.Second)
		expiresAt = &t
	}
	reservation, err := s.svc.ReserveQuota(r.Context(), tc, service.ReserveQuotaInput{
		SubscriptionID: subID,
		QuotaKey:       req.QuotaKey,
		Amount:         req.Amount,
		ReservationID:  req.ReservationID,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, reservation)
}

type quotaActionRequest struct {
	ReservationID string `json:"reservation_id"`
}

// commitQuota — POST /v1/subscriptions/{id}/quota/commit
func (s *Server) commitQuota(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req quotaActionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resID, err := uuid.Parse(req.ReservationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_reservation_id", "reservation_id must be a uuid", reqIDFromRequest(r))
		return
	}
	reservation, err := s.svc.CommitQuota(r.Context(), tc, resID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reservation)
}

// releaseQuota — POST /v1/subscriptions/{id}/quota/release
func (s *Server) releaseQuota(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req quotaActionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	resID, err := uuid.Parse(req.ReservationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_reservation_id", "reservation_id must be a uuid", reqIDFromRequest(r))
		return
	}
	reservation, err := s.svc.ReleaseQuota(r.Context(), tc, resID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reservation)
}

// getQuotaUsage — GET /v1/subscriptions/{id}/quota/usage?key=<key>
func (s *Server) getQuotaUsage(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "query parameter 'key' is required", reqIDFromRequest(r))
		return
	}
	usage, err := s.svc.GetQuotaUsage(r.Context(), tc, subID, key)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// listQuotaReservations — GET /v1/subscriptions/{id}/quota/reservations
func (s *Server) listQuotaReservations(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "subscription id must be a uuid", reqIDFromRequest(r))
		return
	}
	reservations, err := s.svc.ListQuotaReservations(r.Context(), tc, subID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if reservations == nil {
		reservations = []storegen.QuotaReservation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"quota_reservations": reservations})
}
