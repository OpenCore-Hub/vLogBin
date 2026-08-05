package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
)

// auditChainState — GET /v1/operator/audit/chain
// Reports the tamper-evident chain status (migration 0031): total events, tail
// hash/id, and the most recent anchor checkpoint. Operator-only.
func (s *Server) auditChainState(w http.ResponseWriter, r *http.Request) {
	_, _ = tenant.FromContext(r.Context())
	state, err := s.svc.AuditChainState(r.Context())
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	resp := auditChainStateResponse{
		TotalEvents:       state.TotalEvents,
		TailHash:          state.TailHash,
		TailEventID:       state.TailEventID,
		LastAnchorID:      state.LastAnchorID,
		LastAnchorEventID: state.LastAnchorEventID,
		LastAnchorHash:    state.LastAnchorHash,
	}
	if !state.LastAnchorAt.IsZero() {
		resp.LastAnchorAt = &state.LastAnchorAt
	}
	writeJSON(w, http.StatusOK, resp)
}

// auditChainVerify — GET /v1/operator/audit/chain/verify?from=<event_id>&to=<event_id>
// Verifies the hash chain over the event id range (from, to]. Both bounds are
// optional: 0 (or absent) means "first surviving row after the pruned head"
// for from, and "current tail" for to. The response reports the first broken
// event id so callers can bound the damage window. Operator-only.
func (s *Server) auditChainVerify(w http.ResponseWriter, r *http.Request) {
	_, _ = tenant.FromContext(r.Context())
	fromID, err := parseChainBound(r, "from")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_from", err.Error(), reqIDFromRequest(r))
		return
	}
	toID, err := parseChainBound(r, "to")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_to", err.Error(), reqIDFromRequest(r))
		return
	}
	res, err := s.svc.VerifyAuditChain(r.Context(), fromID, toID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, auditChainVerifyResponse{
		OK:            res.OK,
		VerifiedFrom:  res.VerifiedFrom,
		VerifiedTo:    res.VerifiedTo,
		VerifiedCount: res.VerifiedCount,
		BrokenAt:      res.BrokenAt,
		Reason:        res.Reason,
	})
}

// auditChainAnchor — POST /v1/operator/audit/chain/anchor
// Creates an anchor checkpoint at the current chain tail. Anchors bound the
// incremental verification window and are the rows external anchoring (WORM
// object storage) will publish outside the DB. Operator-only.
func (s *Server) auditChainAnchor(w http.ResponseWriter, r *http.Request) {
	_, _ = tenant.FromContext(r.Context())
	anchor, err := s.svc.AnchorAuditChain(r.Context(), operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, auditChainAnchorResponse{
		AnchorID:      anchor.AnchorID,
		TailEventID:   anchor.TailEventID,
		TailHash:      anchor.TailHash,
		EventsCovered: anchor.EventsCovered,
	})
}

// parseChainBound parses an optional non-negative bigint query parameter;
// absent values resolve to 0 ("unbounded" for the chain endpoints).
func parseChainBound(r *http.Request, name string) (int64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("query parameter %q must be a non-negative integer", name)
	}
	return n, nil
}

type auditChainStateResponse struct {
	TotalEvents       int64      `json:"total_events"`
	TailHash          *string    `json:"tail_hash"`
	TailEventID       *int64     `json:"tail_event_id"`
	LastAnchorID      int64      `json:"last_anchor_id"`
	LastAnchorEventID int64      `json:"last_anchor_event_id"`
	LastAnchorHash    string     `json:"last_anchor_hash"`
	LastAnchorAt      *time.Time `json:"last_anchor_at"`
}

type auditChainVerifyResponse struct {
	OK            bool   `json:"ok"`
	VerifiedFrom  int64  `json:"verified_from"`
	VerifiedTo    int64  `json:"verified_to"`
	VerifiedCount int64  `json:"verified_count"`
	BrokenAt      *int64 `json:"broken_at"`
	Reason        string `json:"reason"`
}

type auditChainAnchorResponse struct {
	AnchorID      int64  `json:"anchor_id"`
	TailEventID   int64  `json:"tail_event_id"`
	TailHash      string `json:"tail_hash"`
	EventsCovered int64  `json:"events_covered"`
}
