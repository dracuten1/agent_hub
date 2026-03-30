package middleware

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// Limiter holds a per-client rate limiter.
type Limiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type store struct {
	mu       sync.RWMutex
	limiters map[string]*Limiter
}

// global limiter store
var (
	globalStore     store
	defaultRPM      int
	authRPM         int
	limitCleanupInt = 10 * time.Minute
)

func init() {
	// Read env defaults; guarded init
	globalStore.limiters = make(map[string]*Limiter)
	defaultRPM = 60
	authRPM = 5

	if v := getEnvInt("RATE_LIMIT_RPM", 0); v > 0 {
		defaultRPM = v
	}
	if v := getEnvInt("RATE_LIMIT_AUTH_RPM", 0); v > 0 {
		authRPM = v
	}

	// Start background cleanup of stale entries
	go func() {
		ticker := time.NewTicker(limitCleanupInt)
		defer ticker.Stop()
		for range ticker.C {
			globalStore.mu.Lock()
			for key, l := range globalStore.limiters {
				if time.Since(l.lastSeen) > 30*time.Minute {
					delete(globalStore.limiters, key)
				}
			}
			globalStore.mu.Unlock()
		}
	}()
}

func getEnvInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// getLimiter returns (or creates) a rate.Limiter for the given client key.
func getLimiter(key string, rpm int) *rate.Limiter {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()

	if l, ok := globalStore.limiters[key]; ok {
		l.lastSeen = time.Now()
		return l.limiter
	}

	lim := rate.NewLimiter(rate.Limit(float64(rpm)/60.0), rpm)
	globalStore.limiters[key] = &Limiter{limiter: lim, lastSeen: time.Now()}
	return lim
}

// clientIP extracts the real client IP from a gin context.
func clientIP(c *gin.Context) string {
	// Check X-Forwarded-For first (for reverse proxy setups)
	if fwd := c.GetHeader("X-Forwarded-For"); fwd != "" {
		if idx := strings.Index(fwd, ","); idx != -1 {
			fwd = fwd[:idx]
		}
		if ip := net.ParseIP(strings.TrimSpace(fwd)); ip != nil {
			return ip.String()
		}
	}
	// X-Real-IP
	if real := c.GetHeader("X-Real-IP"); real != "" {
		if ip := net.ParseIP(strings.TrimSpace(real)); ip != nil {
			return ip.String()
		}
	}
	// Fall back to remote addr
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

// isAuthPath returns true for auth-related endpoints.
func isAuthPath(path string) bool {
	return strings.HasPrefix(path, "/api/auth/")
}

// limiterKey includes the RPM tier so auth and general paths get separate buckets.
func limiterKey(ip string, isAuth bool) string {
	if isAuth {
		return ip + ":auth"
	}
	return ip + ":general"
}

// RateLimit returns a gin middleware that enforces per-IP rate limits.
// Auth paths use the stricter RATE_LIMIT_AUTH_RPM limit.
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := isAuthPath(c.Request.URL.Path)
		key := limiterKey(clientIP(c), auth)
		rpm := defaultRPM
		if auth {
			rpm = authRPM
		}

		lim := getLimiter(key, rpm)

		if !lim.Allow() {
			retryAfter := time.Duration(float64(time.Minute) / float64(rpm))
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			c.Header("X-RateLimit-Limit", strconv.Itoa(rpm))
			c.Header("X-RateLimit-Remaining", "0")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": int(retryAfter.Seconds()),
			})
			return
		}

		c.Next()
	}
}
