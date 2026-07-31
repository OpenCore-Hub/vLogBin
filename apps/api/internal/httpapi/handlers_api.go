package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
)

// getAPIVersion — GET /v1/api-version
//
// Returns the current API version, supported versions, and the
// compatibility policy. This satisfies the P0 gate: "公开 API 12 个月
// 兼容政策生效" and spec Section 7.2: "破坏性版本至少并行 12 个月".
func (s *Server) getAPIVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"current_version":      APIVersion,
		"supported_versions":   []string{APIVersion},
		"compatibility_policy": CompatibilityPolicy,
		"webhook_schema_version": domain.WebhookSchemaVersion,
		"deprecated_endpoints": GetDeprecatedEndpoints(),
	})
}
