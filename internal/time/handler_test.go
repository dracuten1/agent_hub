package time

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
	r.GET("/api/time", h.Get)

	req := httptest.NewRequest("GET", "/api/time", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]int64
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	ts, ok := resp["time"]
	if !ok {
		t.Fatal("expected 'time' key in response")
	}
	// Should be a recent Unix timestamp (within last 60 seconds)
	now := time.Now().Unix()
	if ts < now-60 || ts > now+1 {
		t.Errorf("timestamp %d out of expected range [%d, %d]", ts, now-60, now+1)
	}
}
