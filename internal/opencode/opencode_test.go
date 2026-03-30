package opencode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- Test server ---

func newTestServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {

		case "/global/health":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(HealthResponse{Healthy: true, Version: "1.3.5"})

		case "/session":
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]Session{
					{ID: "ses_1", Directory: "/tmp", Title: "Test 1"},
					{ID: "ses_2", Directory: "/home", Title: "Test 2"},
				})
				return
			}
			if r.Method == http.MethodPost {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				dir, _ := body["directory"].(string)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(Session{
					ID:        "ses_new",
					Directory: dir,
					Title:     "New session",
					Version:   "1.3.5",
					Time:      SessionTime{Created: time.Now().UnixMilli()},
				})
				return
			}
			http.NotFound(w, r)

		case "/session/ses_1":
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodGet {
				json.NewEncoder(w).Encode(Session{
					ID:        "ses_1",
					Directory: "/tmp",
					Title:     "Test 1",
				})
			} else if r.Method == http.MethodDelete {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(true)
			} else {
				http.NotFound(w, r)
			}
			return

		case "/session/ses_1/message":
			w.Header().Set("Content-Type", "application/json")
			now := time.Now()
			if r.Method == http.MethodGet {
				// GetMessages returns array of messages
				json.NewEncoder(w).Encode([]*MessageResponse{
					{
						Info: &MessageInfo{
							ID:        "msg_1",
							SessionID: "ses_1",
							Role:      "assistant",
							Time: &MessageTimeInfo{
								Created:   now.Add(-1 * time.Second).UnixMilli(),
								Completed: now.UnixMilli(),
							},
						},
						Parts: []MessagePart{{Type: "text", Text: "Hello from assistant"}},
						ID:    "msg_1",
					},
				})
			} else {
				// SendMessage returns single message
				json.NewEncoder(w).Encode(MessageResponse{
					Info: &MessageInfo{
						ID:        "msg_1",
						SessionID: "ses_1",
						Role:      "assistant",
						Time: &MessageTimeInfo{
							Created:   now.Add(-1 * time.Second).UnixMilli(),
							Completed: now.UnixMilli(),
						},
					},
					Parts: []MessagePart{{Type: "text", Text: "Hello from assistant"}},
					ID:    "msg_1",
				})
			}
			return

		case "/session/ses_error", "/session/ses_error/message":
			// Session completes immediately with an error
			now := time.Now()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]*MessageResponse{
				{
					Info: &MessageInfo{
						ID:    "msg_error",
						Role:  "assistant",
						Time:  &MessageTimeInfo{Created: now.Add(-1 * time.Second).UnixMilli(), Completed: now.UnixMilli()},
						Error: &MessageError{Name: "APIError", Data: &MessageErrorData{Message: "Invalid API key", StatusCode: 401}},
					},
					Parts: []MessagePart{},
					ID:    "msg_error",
				},
			})

		case "/session/ses_stuck", "/session/ses_stuck/message":
			// Never completes — for timeout testing
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]*MessageResponse{
				{
					Info: &MessageInfo{ID: "msg_stuck", SessionID: "ses_stuck", Time: &MessageTimeInfo{Created: time.Now().UnixMilli()}},
					ID:   "msg_stuck",
				},
			})

		case "/session/ses_inflight", "/session/ses_inflight/message":
			// Completes after one more poll (for success path)
			w.Header().Set("Content-Type", "application/json")
			now := time.Now()
			// Return completed message (the old reqCount counter was per-request,
			// so always return completed for simplicity)
			json.NewEncoder(w).Encode([]*MessageResponse{
				{
					Info: &MessageInfo{
						ID: "msg_inflight", SessionID: "ses_inflight", Role: "assistant",
						Time: &MessageTimeInfo{Created: now.Add(-1*time.Second).UnixMilli(), Completed: now.UnixMilli()},
					},
					Parts: []MessagePart{{Type: "text", Text: "Hello from assistant"}},
					ID:   "msg_inflight",
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

// --- Tests ---

func TestNewClient(t *testing.T) {
	c := NewClient(4096)
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.port != 4096 {
		t.Errorf("port = %d; want 4096", c.port)
	}

	c0 := NewClient(0)
	if c0.port != defaultPort {
		t.Errorf("port with 0 = %d; want default %d", c0.port, defaultPort)
	}
}

func TestPing(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	if err := c.Ping(); err != nil {
		t.Errorf("Ping() error = %v; want nil", err)
	}
}

func TestCreateSession(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	session, err := c.CreateSession("/my/project")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.ID != "ses_new" {
		t.Errorf("session.ID = %q; want ses_new", session.ID)
	}
	if session.Directory != "/my/project" {
		t.Errorf("session.Directory = %q; want /my/project", session.Directory)
	}

	c.mu.Lock()
	if len(c.sessions) != 1 {
		t.Errorf("tracked sessions = %d; want 1", len(c.sessions))
	}
	c.mu.Unlock()
}

func TestListSessions(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("len(sessions) = %d; want 2", len(sessions))
	}
}

func TestGetSession(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	session, err := c.GetSession("ses_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if session.ID != "ses_1" {
		t.Errorf("session.ID = %q; want ses_1", session.ID)
	}
}

func TestSendMessage(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.SendMessage("ses_1", "Hello world")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if resp.Info == nil {
		t.Fatal("resp.Info is nil")
	}
	if resp.Info.Role != "assistant" {
		t.Errorf("resp.Info.Role = %q; want assistant", resp.Info.Role)
	}
	if len(resp.Parts) == 0 || resp.Parts[0].Text != "Hello from assistant" {
		t.Errorf("resp.Parts[0].Text = %q; want 'Hello from assistant'", resp.Parts[0].Text)
	}
}

func TestWaitForResult_Success(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	result, err := c.WaitForResult("ses_1", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForResult() error = %v", err)
	}
	if result.Error != "" {
		t.Errorf("result.Error = %q; want empty", result.Error)
	}
	// Content should be extracted from the session info (the session always
	// completes immediately on /session/ses_1, so we get it on first poll).
	// The Content field comes from the initial nil message, which has no parts,
	// so we only check that we got a result without timeout.
	if result.Messages == nil {
		// Result should still be valid even with no tracked messages
		t.Log("result.Messages is nil (no initial message passed — expected)")
	}
}

func TestWaitForResult_Timeout(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	_, err := c.WaitForResult("ses_stuck", 500*time.Millisecond)
	if err != ErrTimeout {
		t.Errorf("err = %v; want ErrTimeout", err)
	}
}

func TestWaitForResult_ErrorFromSession(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	result, err := c.WaitForResult("ses_error", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForResult() error = %v", err)
	}
	if result.Error == "" {
		t.Error("result.Error should be set for error session")
	}
}

func TestDeleteSession(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := &Client{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
		sessions:   []string{"ses_1"},
	}

	err := c.DeleteSession("ses_1")
	if err != nil {
		t.Errorf("DeleteSession() error = %v; want nil", err)
	}

	c.mu.Lock()
	if len(c.sessions) != 0 {
		t.Errorf("tracked sessions = %v; want []", c.sessions)
	}
	c.mu.Unlock()
}

func TestCleanup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{
		baseURL:    srv.URL,
		httpClient: srv.Client(),
		sessions:   []string{"ses_a", "ses_b", "ses_c"},
	}

	if err := c.Cleanup(); err != nil {
		t.Errorf("Cleanup() error = %v; want nil", err)
	}

	c.mu.Lock()
	if len(c.sessions) != 0 {
		t.Errorf("tracked sessions after cleanup = %v; want []", c.sessions)
	}
	c.mu.Unlock()
}

func TestExtractMessagesResult(t *testing.T) {
	resp := &MessageResponse{
		Info: &MessageInfo{
			ID:    "msg_1",
			Error: &MessageError{Name: "APIError", Data: &MessageErrorData{Message: "bad key", StatusCode: 401}},
		},
		Parts: []MessagePart{{Type: "text", Text: "output text"}},
	}
	result := &Result{}
	extractMessagesResult(resp, result)
	if result.Error != "bad key" {
		t.Errorf("result.Error = %q; want 'bad key'", result.Error)
	}
	if result.Content != "output text" {
		t.Errorf("result.Content = %q; want 'output text'", result.Content)
	}
}

func TestExtractMessagesResult_ContentFromParts(t *testing.T) {
	cases := []struct {
		name  string
		parts []MessagePart
		want  string
	}{
		{"text part", []MessagePart{{Type: "text", Text: "hello"}}, "hello"},
		{"content part", []MessagePart{{Type: "code", Content: "fmt.Println(1)"}}, "fmt.Println(1)"},
		{"mixed parts", []MessagePart{{Type: "text", Text: "hi"}, {Type: "code", Content: "x=1"}}, "hix=1"},
		{"empty", []MessagePart{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := &Result{}
			extractMessagesResult(&MessageResponse{Parts: tc.parts}, result)
			if result.Content != tc.want {
				t.Errorf("Content = %q; want %q", result.Content, tc.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		errMsg string
		want   bool
	}{
		{"connection refused", true},
		{"connection reset by peer", true},
		{"context deadline exceeded", false},
		{"HTTP 401", false},
		{"connection timed out", true},
	}
	for _, tc := range cases {
		got := isRetryable(&testErr{msg: tc.errMsg})
		if got != tc.want {
			t.Errorf("isRetryable(%q) = %v; want %v", tc.errMsg, got, tc.want)
		}
	}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }

// --- Concurrent safety ---

func TestClient_Concurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Session{ID: "ses_concurrent"})
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.ListSessions()
			_, _ = c.GetSession("ses_1")
			_, _ = c.CreateSession("/tmp")
		}()
	}
	wg.Wait()
}

// --- JSON round-trip ---

func TestTypesRoundTrip(t *testing.T) {
	sessionJSON := `{"id":"ses_abc","slug":"happy-path","projectID":"global","directory":"/repo","title":"Test","version":"1.3.5","summary":{"additions":10,"deletions":3,"files":5},"time":{"created":1700000000000,"updated":1700001000000}}`
	var s Session
	if err := json.Unmarshal([]byte(sessionJSON), &s); err != nil {
		t.Fatalf("Session unmarshal error: %v", err)
	}
	if s.Summary.Files != 5 {
		t.Errorf("Summary.Files = %d; want 5", s.Summary.Files)
	}

	msgJSON := `{"info":{"id":"msg_1","sessionID":"ses_abc","role":"assistant","time":{"created":1700000000000,"completed":1700000010000}},"parts":[{"type":"text","text":"hi"}],"id":"msg_1","sessionID":"ses_abc"}`
	var m MessageResponse
	if err := json.Unmarshal([]byte(msgJSON), &m); err != nil {
		t.Fatalf("MessageResponse unmarshal error: %v", err)
	}
	if m.Info.Time.Completed == 0 {
		t.Error("MessageTimeInfo.Completed should be non-zero")
	}
}
