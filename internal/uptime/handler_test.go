package uptime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGet(t *testing.T) {
	h := NewHandler()
	r := gin.New()
	r.GET("/api/uptime", h.Get)

	req := httptest.NewRequest("GET", "/api/uptime", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]int64
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	uptime, ok := resp["uptime"]
	if !ok {
		t.Fatal("expected 'uptime' key in response")
	}
	// Should be 0 or very small (server just started)
	if uptime < 0 {
		t.Errorf("uptime should be non-negative, got %d", uptime)
	}
}

func TestGetMultiple(t *testing.T) {
	h := NewHandler()

	// First call
	w1 := httptest.NewRecorder()
	c1 := writeContext(w1, "/api/uptime")
	h.Get(c1)
	var resp1 map[string]int64
	json.Unmarshal(w1.Body.Bytes(), &resp1)

	// Wait 100ms then second call
	time.Sleep(100 * time.Millisecond)
	w2 := httptest.NewRecorder()
	c2 := writeContext(w2, "/api/uptime")
	h.Get(c2)
	var resp2 map[string]int64
	json.Unmarshal(w2.Body.Bytes(), &resp2)

	if resp2["uptime"] <= resp1["uptime"] {
		t.Errorf("uptime should increase between calls: first=%d, second=%d", resp1["uptime"], resp2["uptime"])
	}
}

func writeContext(w *httptest.ResponseRecorder, path string) *gin.Context {
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", path, nil)
	return c
}

func TestNewHandler(t *testing.T) {
	h := NewHandler()
	if h == nil {
		t.Fatal("NewHandler should not return nil")
	}
	if h.start.IsZero() {
		t.Error("start time should be set")
	}
}
