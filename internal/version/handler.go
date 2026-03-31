package version

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler serves version information.
type Handler struct{}

// NewHandler creates a new version handler.
func NewHandler() *Handler {
	return &Handler{}
}

// VersionResponse is the JSON response for GET /api/version.
type VersionResponse struct {
	Version string `json:"version"`
	Build   string `json:"build"`
}

// Get returns the API version and build information.
// GET /api/version — no auth required.
func (h *Handler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, VersionResponse{
		Version: "1.0.0",
		Build:   "dev",
	})
}
