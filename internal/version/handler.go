package version

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler returns version info.
type Handler struct{}

// NewHandler creates a new version handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Get handles GET /api/version — returns version info. No auth required.
func (h *Handler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "1.0.0",
		"build":   "dev",
	})
}
