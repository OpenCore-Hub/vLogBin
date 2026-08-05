package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
)

type riskReviewRequest struct {
	RiskScore  int              `json:"risk_score"`
	Checks     map[string]bool  `json:"checks"`
	Decision   string           `json:"decision"`
	Reason     string           `json:"reason"`
	ReviewedBy string           `json:"reviewed_by"`
}

// operatorSubmitRiskReview — POST /v1/operator/providers/{id}/risk-review
func (s *Server) operatorSubmitRiskReview(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	var req riskReviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	review, err := s.svc.SubmitRiskReview(r.Context(), providerID, service.RiskReviewInput{
		RiskScore:  req.RiskScore,
		Checks:     req.Checks,
		Decision:   req.Decision,
		Reason:     req.Reason,
		ReviewedBy: req.ReviewedBy,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"review": review})
}

// operatorListRiskReviews — GET /v1/operator/providers/{id}/risk-reviews
func (s *Server) operatorListRiskReviews(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	reviews, err := s.svc.ListRiskReviews(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": reviews})
}
