package version

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestGet(t *testing.T) {
	h := NewHandler()
	r := gin.New()
	r.GET("/api/version", h.Get)

	req := httptest.NewRequest("GET", "/api/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["version"] != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %s", resp["version"])
	}
	if resp["build"] != "dev" {
		t.Errorf("expected build=dev, got %s", resp["build"])
	}
}
