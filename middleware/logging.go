package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		clientIP := c.ClientIP()
		method := c.Request.Method

		c.Next()

		if path == "/health" {
			return
		}

		duration := time.Since(start).Milliseconds()
		status := c.Writer.Status()

		log.Printf(`{"time":"%s","method":"%s","path":"%s","status":%d,"duration_ms":%d,"ip":"%s"}`,
			start.Format(time.RFC3339),
			method,
			path,
			status,
			duration,
			clientIP,
		)
	}
}
