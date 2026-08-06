package httpapi

import "net/http"

// operatorQueueOverview — GET /v1/operator/queues/overview
func (s *Server) operatorQueueOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.svc.QueueOverview(r.Context(), queryLimit(r, 50))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}
