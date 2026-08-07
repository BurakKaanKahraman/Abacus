package handler

import (
	"net/http"
	"time"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
)

// HealthHandler serves GET /api/v1/health. The endpoint is dependency free by
// design: the service has no database or downstream call, so liveness and
// readiness are the same question.
type HealthHandler struct {
	startedAt time.Time
	version   string
	now       func() time.Time
}

// NewHealthHandler records the process start time used to report uptime.
func NewHealthHandler(startedAt time.Time, version string) *HealthHandler {
	return &HealthHandler{startedAt: startedAt, version: version, now: time.Now}
}

// Handle renders the current service status.
func (h *HealthHandler) Handle(w http.ResponseWriter, r *http.Request) {
	uptime := h.now().Sub(h.startedAt).Truncate(time.Second)

	httpx.WriteJSON(w, http.StatusOK, domain.HealthResponse{
		Status:  "UP",
		Uptime:  uptime.String(),
		Version: h.version,
	})
}
