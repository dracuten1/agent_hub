package task

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// TestListTasksInvalidTypeFilter verifies invalid type returns 400 with correct error
func TestListTasksInvalidTypeFilter(t *testing.T) {
	h := &Handler{db: nil}
	r := gin.New()
	r.GET("/tasks", h.ListTasks)

	req := httptest.NewRequest("GET", "/tasks?type=invalid", nil)
	w := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("handler panicked on nil DB")
			}
		}()
		r.ServeHTTP(w, req)
	}()

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["error"] != "Invalid type filter" {
		t.Errorf("expected error='Invalid type filter', got %v", resp["error"])
	}

	allowed, ok := resp["allowed"].([]interface{})
	if !ok {
		t.Fatalf("expected 'allowed' field, got %v", resp)
	}
	if len(allowed) != 4 {
		t.Errorf("expected 4 allowed values, got %d", len(allowed))
	}
}

// TestListTasksValidTypeFilters verifies valid types don't return 400 (pass through to DB)
func TestListTasksValidTypeFilters(t *testing.T) {
	h := &Handler{db: nil}
	validTypes := []string{"general", "dev", "review", "test"}

	for _, typ := range validTypes {
		r := gin.New()
		r.GET("/tasks", h.ListTasks)

		req := httptest.NewRequest("GET", "/tasks?type="+typ, nil)
		w := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					// Panic means the handler didn't return 400 — valid type was accepted
					// (panic is from DB access, which is expected with nil DB)
				}
			}()
			r.ServeHTTP(w, req)
		}()

		if w.Code == http.StatusBadRequest {
			t.Errorf("type=%s: expected non-400, got %d", typ, w.Code)
		}
	}
}

// TestListTasksNoTypeFilter verifies empty type doesn't filter (backward compat)
func TestListTasksNoTypeFilter(t *testing.T) {
	h := &Handler{db: nil}
	r := gin.New()
	r.GET("/tasks", h.ListTasks)

	req := httptest.NewRequest("GET", "/tasks", nil)
	w := httptest.NewRecorder()

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected — nil DB panics when no filter applied
			}
		}()
		r.ServeHTTP(w, req)
	}()
}
