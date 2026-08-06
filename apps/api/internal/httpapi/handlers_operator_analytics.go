package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
)

// operatorAnalyticsDashboard — GET /v1/operator/providers/{id}/analytics/dashboard?env=test
// Console 分析控制面：汇总收入 / MAU / 转化 / 流失 / 用量异常，一次请求返回。
func (s *Server) operatorAnalyticsDashboard(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	dashboard, err := s.svc.GetProviderDashboard(r.Context(), service.OperatorAuthContext(providerID, env))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dashboard)
}
