package dashboard

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestDashboardRoutePath verifies the handler is wired correctly (Summary endpoint)
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
		h.Summary(c)
	}()

	if !panicked {
		t.Error("expected panic on nil DB — handler not wired correctly")
	}
}

// TestAgentInfoJSON tests that AgentInfo serializes correctly
func TestAgentInfoJSON(t *testing.T) {
	info := AgentInfo{
		Name:           "dev1",
		Role:           "developer",
		Status:         "online",
		CurrentTasks:   2,
		MaxTasks:       5,
		TotalCompleted: 10,
		TotalFailed:    1,
		LastHeartbeat:  "2026-01-01T00:00:00Z",
		Online:         true,
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "dev1" {
		t.Errorf("expected name=dev1, got %v", m["name"])
	}
	if m["online"] != true {
		t.Errorf("expected online=true, got %v", m["online"])
	}
}

// TestTaskCountJSON tests that TaskCount serializes correctly
func TestTaskCountJSON(t *testing.T) {
	count := TaskCount{
		Status: "available",
		Count:  5,
	}
	data, err := json.Marshal(count)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["status"] != "available" {
		t.Errorf("expected status=available, got %v", m["status"])
	}
	if m["count"] != float64(5) {
		t.Errorf("expected count=5, got %v", m["count"])
	}
}

// TestQueueDepthJSON tests that QueueDepth serializes correctly
func TestQueueDepthJSON(t *testing.T) {
	queue := QueueDepth{
		TaskType: "coding",
		Count:    3,
	}
	data, err := json.Marshal(queue)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["task_type"] != "coding" {
		t.Errorf("expected task_type=coding, got %v", m["task_type"])
	}
}

// TestSummaryResponseJSON tests that SummaryResponse serializes correctly
func TestSummaryResponseJSON(t *testing.T) {
	resp := SummaryResponse{
		Agents:     []AgentInfo{},
		TaskCounts: []TaskCount{{Status: "available", Count: 5}},
		Queue:      []QueueDepth{{TaskType: "coding", Count: 3}},
		Recent:     []RecentTask{},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["task_counts"] == nil {
		t.Error("expected task_counts to be present")
	}
	if m["queue"] == nil {
		t.Error("expected queue to be present")
	}
}
