package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCORSWildcard(t *testing.T) {
	// No env set → wildcard
	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSSingleOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	// Matching origin
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("expected https://example.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}

	// Non-matching origin
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Origin", "https://evil.com")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected empty origin for non-matching, got %s", w2.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSMultiOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.com, https://b.com")

	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	for _, origin := range []string{"https://a.com", "https://b.com"} {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("expected %s, got %s", origin, w.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}

func TestCORSPreflight(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	r := gin.New()
	r.Use(CORS())
	r.OPTIONS("/test", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestCORSExplicitWildcard(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://anything.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSNoOriginHeader(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")

	r := gin.New()
	r.Use(CORS())
	r.GET("/test", func(c *gin.Context) { c.String(200, "ok") })

	req := httptest.NewRequest("GET", "/test", nil)
	// No Origin header
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should not set origin header when no Origin is provided
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected empty for no Origin header, got %s", w.Header().Get("Access-Control-Allow-Origin"))
	}
}
