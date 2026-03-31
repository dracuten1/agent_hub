package health

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler serves health check endpoints.
type Handler struct {
	startTime time.Time
}

// NewHandler creates a new health handler with the given server start time.
func NewHandler(startTime time.Time) *Handler {
	return &Handler{startTime: startTime}
}

// HealthResponse is the JSON response for GET /api/health.
type HealthResponse struct {
	Status        string  `json:"status"`
	Version       string  `json:"version"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// Health returns server health status including uptime.
// GET /api/health — no auth required.
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{
		Status:        "ok",
		Version:       "1.0",
		UptimeSeconds: time.Since(h.startTime).Seconds(),
	})
}
