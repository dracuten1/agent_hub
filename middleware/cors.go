package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS returns a middleware that handles Cross-Origin Resource Sharing.
// Supports:
//   - Single origin: CORS_ALLOWED_ORIGINS=https://example.com
//   - Multiple origins: CORS_ALLOWED_ORIGINS=https://a.com,https://b.com
//   - Wildcard (default): CORS_ALLOWED_ORIGINS="" or unset → Access-Control-Allow-Origin: *
func CORS() gin.HandlerFunc {
	origins := os.Getenv("CORS_ALLOWED_ORIGINS")

	allowedMethods := "GET,POST,PUT,PATCH,DELETE,OPTIONS"
	allowedHeaders := "Authorization,Content-Type"
	maxAge := "86400"

	// Parse allowed origins
	var originSet map[string]bool
	var isWildcard bool
	if origins == "" || origins == "*" {
		isWildcard = true
	} else {
		originSet = make(map[string]bool)
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				originSet[o] = true
			}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if isWildcard {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if origin != "" && originSet[origin] {
			// Only echo back the origin if it's in our allowlist
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		// If origin not in allowlist, don't set the header — browser blocks

		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Max-Age", maxAge)

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
