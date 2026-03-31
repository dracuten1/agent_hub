package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestGetReturnsCorrectFields(t *testing.T) {
	h := NewHandler()
	r := gin.New()
	r.GET("/api/version", h.Get)

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["version"] != "1.0.0" {
		t.Errorf("expected version='1.0.0', got %v", resp["version"])
	}
	if resp["build"] != "dev" {
		t.Errorf("expected build='dev', got %v", resp["build"])
	}
}

func TestGetNoAuthRequired(t *testing.T) {
	h := NewHandler()
	r := gin.New()
	r.GET("/api/version", h.Get)

	// No Authorization header
	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without auth, got %d", w.Code)
	}
}
