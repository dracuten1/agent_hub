package ping

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler handles ping requests.
type Handler struct{}

// NewHandler creates a new ping handler.
func NewHandler() *Handler {
	return &Handler{}
}

// Get handles GET /api/ping — returns pong. No auth required.
func (h *Handler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ping": "pong"})
}
