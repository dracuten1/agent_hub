package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestHealthReturnsCorrectFields(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Second)
	h := NewHandler(startTime)
	r := gin.New()
	r.GET("/api/health", h.Health)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status='ok', got %v", resp["status"])
	}
	if resp["version"] != "1.0" {
		t.Errorf("expected version='1.0', got %v", resp["version"])
	}
	uptime, ok := resp["uptime_seconds"].(float64)
	if !ok {
		t.Fatalf("uptime_seconds should be a number, got %T", resp["uptime_seconds"])
	}
	// Uptime should be >= 5 seconds (we started 5s ago)
	if uptime < 4.9 {
		t.Errorf("uptime_seconds too low: got %.2f, want >= 4.9", uptime)
	}
}

func TestHealthUptimeIncreases(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Second) // start 1s ago so uptime > 0
	h := NewHandler(startTime)
	r := gin.New()
	r.GET("/api/health", h.Health)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp1)
	uptime1 := resp1["uptime_seconds"].(float64)

	// Small delay
	time.Sleep(100 * time.Millisecond)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	uptime2 := resp2["uptime_seconds"].(float64)

	if uptime2 <= uptime1 {
		t.Errorf("uptime should increase: uptime1=%.3f, uptime2=%.3f", uptime1, uptime2)
	}
}

func TestHealthNoAuthRequired(t *testing.T) {
	h := NewHandler(time.Now())
	r := gin.New()
	r.GET("/api/health", h.Health)

	req := httptest.NewRequest("GET", "/api/health", nil)
	// No Authorization header
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without auth, got %d", w.Code)
	}
}

func TestNewHandlerStoresStartTime(t *testing.T) {
	now := time.Now()
	h := NewHandler(now)
	if h.startTime != now {
		t.Errorf("startTime not stored correctly")
	}
}
