package ping

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestPing(t *testing.T) {
	h := NewHandler()
	r := gin.New()
	r.GET("/api/ping", h.Get)

	req := httptest.NewRequest("GET", "/api/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["ping"] != "pong" {
		t.Errorf("expected ping=pong, got %v", resp)
	}
}
