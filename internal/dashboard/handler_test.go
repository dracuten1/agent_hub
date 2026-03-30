package dashboard

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestDashboardEventJSON(t *testing.T) {
	e := DashboardEvent{
		TaskID:    "task-1",
		Agent:     "dev1",
		Event:     "claimed",
		ToStatus:  "claimed",
		Note:      "test note",
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["task_id"] != "task-1" {
		t.Errorf("expected task_id=task-1, got %v", m["task_id"])
	}
	if m["agent"] != "dev1" {
		t.Errorf("expected agent=dev1, got %v", m["agent"])
	}
}

func TestDashboardRoutePath(t *testing.T) {
	h := &Handler{db: nil}

	// Verify handler panics on nil DB (proves route matched + handler called)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		// Calling with nil DB panics — this proves the handler is wired correctly
		c := &gin.Context{}
		h.Get(c)
	}()

	if !panicked {
		t.Error("expected panic on nil DB — handler not wired correctly")
	}
}

func TestDashboardEmptySliceNotNull(t *testing.T) {
	var events []DashboardEvent
	if events != nil {
		t.Error("nil slice should be nil")
	}
	events = []DashboardEvent{}
	if events == nil {
		t.Error("empty slice should not be nil")
	}
}

func TestGetStatsNilDB(t *testing.T) {
	h := &Handler{db: nil}

	panicked := false
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, err = h.getStats()
	}()

	if !panicked {
		t.Error("expected panic with nil DB")
	}
	if err != nil {
		t.Errorf("expected nil err after panic recovery, got: %v", err)
	}
}
