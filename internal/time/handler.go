package time

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler handles time requests.
type Handler struct{}

// NewHandler creates a new time handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Get handles GET /api/time — returns current Unix timestamp. No auth required.
func (h *Handler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"time": time.Now().Unix()})
}
