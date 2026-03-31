package uptime

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler tracks server uptime.
type Handler struct {
	start time.Time
	mu    sync.RWMutex
}

// NewHandler creates a new uptime handler, recording the server start time.
func NewHandler() *Handler {
	return &Handler{start: time.Now()}
}

// Get handles GET /api/uptime — returns milliseconds since server start. No auth.
func (h *Handler) Get(c *gin.Context) {
	h.mu.RLock()
	elapsed := time.Since(h.start).Milliseconds()
	h.mu.RUnlock()
	c.JSON(http.StatusOK, gin.H{"uptime": elapsed})
}
