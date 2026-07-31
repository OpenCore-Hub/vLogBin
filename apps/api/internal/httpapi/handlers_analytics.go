package httpapi

import (
	"net/http"
	"strconv"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
)

// getProviderDashboard — GET /v1/analytics/dashboard
func (s *Server) getProviderDashboard(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	dashboard, err := s.svc.GetProviderDashboard(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dashboard)
}

// getRevenueSummary — GET /v1/analytics/revenue?months=12
func (s *Server) getRevenueSummary(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	months := 12
	if m := r.URL.Query().Get("months"); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 && n <= 24 {
			months = n
		}
	}
	revenue, err := s.svc.GetRevenueSummary(r.Context(), tc, months)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if revenue == nil {
		revenue = []service.RevenueSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"revenue": revenue})
}

// getMAUSummary — GET /v1/analytics/mau?months=12
func (s *Server) getMAUSummary(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	months := 12
	if m := r.URL.Query().Get("months"); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 && n <= 24 {
			months = n
		}
	}
	mau, err := s.svc.GetMAUSummary(r.Context(), tc, months)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if mau == nil {
		mau = []service.MAUSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"mau": mau})
}

// getConversionSummary — GET /v1/analytics/conversion?months=12
func (s *Server) getConversionSummary(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	months := 12
	if m := r.URL.Query().Get("months"); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 && n <= 24 {
			months = n
		}
	}
	conversion, err := s.svc.GetConversionSummary(r.Context(), tc, months)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if conversion == nil {
		conversion = []service.ConversionSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversion": conversion})
}

// getChurnSummary — GET /v1/analytics/churn?months=12
func (s *Server) getChurnSummary(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	months := 12
	if m := r.URL.Query().Get("months"); m != "" {
		if n, err := strconv.Atoi(m); err == nil && n > 0 && n <= 24 {
			months = n
		}
	}
	churn, err := s.svc.GetChurnSummary(r.Context(), tc, months)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if churn == nil {
		churn = []service.ChurnSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"churn": churn})
}

// getUsageBreakdown — GET /v1/analytics/usage-breakdown?days=30
func (s *Server) getUsageBreakdown(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	breakdown, err := s.svc.GetUsageBreakdown(r.Context(), tc, days)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if breakdown == nil {
		breakdown = []service.UsageBreakdownSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage_breakdown": breakdown})
}

// getUsageAnomalies — GET /v1/analytics/anomalies
func (s *Server) getUsageAnomalies(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	anomalies, err := s.svc.GetUsageAnomalies(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if anomalies == nil {
		anomalies = []service.AnomalySummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"anomalies": anomalies})
}
