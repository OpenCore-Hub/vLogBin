package httpapi

import (
	"net/http"
	"strconv"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
)

// streamEvents — GET /v1/events?cursor=<uuid>&limit=100&type=<event_type>&aggregate_type=<aggregate_type>
//
// Enterprise Event Stream API: cursor-based forward pagination for
// consuming platform events. Complements the webhook push model with
// a pull model that lets providers consume events at their own pace.
//
// The cursor is the next_cursor from the previous response. Omit it to
// start from the beginning. The response includes next_cursor and
// has_more to guide pagination.
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())

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

	result, err := s.svc.StreamEvents(r.Context(), tc, cursor, eventType, aggregateType, limit)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if result.Events == nil {
		result.Events = []storegen.OutboxEvent{}
	}
	writeJSON(w, http.StatusOK, result)
}
