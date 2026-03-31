package websocket

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub should not return nil")
	}
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHubBroadcastNoClients(t *testing.T) {
	hub := NewHub()
	// Broadcast to hub with no clients — should not panic
	hub.Broadcast([]byte(`{"task_id":"test"}`))
	hub.Broadcast([]byte(`{"task_id":"test2"}`))
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHubBroadcastWithClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// No way to connect a fake client without a real conn,
	// so just verify broadcast doesn't panic and hub is running
	hub.Broadcast([]byte(`{"task_id":"test"}`))
	time.Sleep(20 * time.Millisecond)
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHubStop(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	hub.Broadcast([]byte(`{"task_id":"test"}`))
	time.Sleep(10 * time.Millisecond)

	// Stop should not panic
	hub.Stop()
	time.Sleep(10 * time.Millisecond)
}

func TestHubStopIdempotent(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	hub.Stop()
	hub.Stop() // second stop should not panic
}

func TestBroadcastTaskEvent(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	// BroadcastTaskEvent should not panic
	hub.BroadcastTaskEvent("task-1", "claimed", "in_progress", "dev1")
	time.Sleep(20 * time.Millisecond)
}

func TestTaskEventBytes(t *testing.T) {
	event := TaskEvent{
		TaskID:     "task-1",
		FromStatus: "done",
		ToStatus:   "review",
		Agent:      "dev1",
		Timestamp:  time.Now(),
	}
	data, err := event.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded TaskEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TaskID != "task-1" {
		t.Errorf("expected task_id=task-1, got %s", decoded.TaskID)
	}
	if decoded.ToStatus != "review" {
		t.Errorf("expected to_status=review, got %s", decoded.ToStatus)
	}
	if decoded.FromStatus != "done" {
		t.Errorf("expected from_status=done, got %s", decoded.FromStatus)
	}
	if decoded.Agent != "dev1" {
		t.Errorf("expected agent=dev1, got %s", decoded.Agent)
	}
}

func TestTaskEventJSON(t *testing.T) {
	event := TaskEvent{
		TaskID:    "abc",
		ToStatus:  "in_progress",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, _ := json.Marshal(event)
	s := string(data)
	if !contains(s, "task_id") || !contains(s, "abc") {
		t.Error("json should contain task_id and abc")
	}
}

func TestParseToken(t *testing.T) {
	tests := []struct {
		token  string
		userID string
		agent  string
	}{
		{"user1:dev", "user1", "dev"},
		{"admin", "admin", ""},
		{"user2:reviewer", "user2", "reviewer"},
		{"", "", ""},
	}
	for _, tt := range tests {
		uid, ag := parseToken(tt.token)
		if uid != tt.userID || ag != tt.agent {
			t.Errorf("parseToken(%q) = (%q, %q), want (%q, %q)",
				tt.token, uid, ag, tt.userID, tt.agent)
		}
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		token   string
		wantOK  bool
	}{
		{"short", false},          // too short
		{"1234567", false},       // 7 chars, need >= 8
		{"12345678", true},       // 8 chars
		{"user:agent:extra", true},
		{"", false},
	}
	for _, tt := range tests {
		got := validateToken(tt.token)
		if got != tt.wantOK {
			t.Errorf("validateToken(%q) = %v, want %v", tt.token, got, tt.wantOK)
		}
	}
}

func TestIsOriginAllowed(t *testing.T) {
	// Save and restore CORS_ALLOWED_ORIGINS
	orig := os.Getenv("CORS_ALLOWED_ORIGINS")
	defer os.Setenv("CORS_ALLOWED_ORIGINS", orig)

	tests := []struct {
		origEnv   string
		origin    string
		wantOK    bool
	}{
		{"", "https://example.com", true},         // empty = allow all
		{"*", "https://anything.com", true},       // wildcard = allow all
		{"https://example.com", "https://example.com", true},
		{"https://example.com", "https://evil.com", false},
		{"https://foo.com,https://bar.com", "https://foo.com", true},
		{"https://foo.com,https://bar.com", "https://bar.com", true},
		{"https://foo.com,https://bar.com", "https://evil.com", false},
		{"", "", true}, // no origin header = not a browser
	}
	for _, tt := range tests {
		os.Setenv("CORS_ALLOWED_ORIGINS", tt.origEnv)
		got := isOriginAllowed(tt.origin)
		if got != tt.wantOK {
			t.Errorf("isOriginAllowed(origin=%q, CORS=%q) = %v, want %v",
				tt.origin, tt.origEnv, got, tt.wantOK)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
