package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// ErrTimeout is returned by WaitForResult when polling exceeds the deadline.
var ErrTimeout = errors.New("timeout waiting for result")

const (
	defaultPort    = 4096
	pollInterval   = 2 * time.Second
	defaultTimeout = 10 * time.Minute
	maxRetries     = 3
	retryBaseDelay = 100 * time.Millisecond
)

// Client wraps the OpenCode server HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	port       int

	mu       sync.Mutex
	sessions []string // sessions created by this client for cleanup tracking
}

// NewClient returns a client that talks to an OpenCode server on the given port.
// If port <= 0, defaults to 4096.
func NewClient(port int) *Client {
	if port <= 0 {
		port = defaultPort
	}
	return &Client{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		port: port,
	}
}

// Ping checks that the OpenCode server is reachable and healthy.
func (c *Client) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.doRequest(ctx, http.MethodGet, "/global/health", nil, nil)
}

// CreateSession creates a new OpenCode session in the given work directory.
func (c *Client) CreateSession(workDir string) (*Session, error) {
	return c.CreateSessionWithModel(workDir, "")
}

// CreateSessionWithModel creates a session with an optional model override.
// If model is empty, the server default is used.
func (c *Client) CreateSessionWithModel(workDir, model string) (*Session, error) {
	body, _ := json.Marshal(map[string]string{"directory": workDir, "model": model})
	var session Session
	err := c.doRequest(context.Background(), http.MethodPost, "/session", bytes.NewReader(body), &session)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.sessions = append(c.sessions, session.ID)
	c.mu.Unlock()
	return &session, nil
}

// ListSessions returns all sessions from the OpenCode server.
func (c *Client) ListSessions() ([]Session, error) {
	var sessions []Session
	err := c.doRequest(context.Background(), http.MethodGet, "/session", nil, &sessions)
	return sessions, err
}

// GetSession returns a single session by ID.
func (c *Client) GetSession(sessionID string) (*Session, error) {
	var session Session
	err := c.doRequest(context.Background(), http.MethodGet, "/session/"+sessionID, nil, &session)
	return &session, err
}

// GetMessages returns all messages for a session.
func (c *Client) GetMessages(sessionID string) ([]*MessageResponse, error) {
	var messages []*MessageResponse
	err := c.doRequest(context.Background(), http.MethodGet, "/session/"+sessionID+"/message", nil, &messages)
	return messages, err
}

// SendMessage sends a user message to an existing session and returns the response.
// The response may be partial; use WaitForResult to wait for completion.
// Uses a short timeout since OpenCode may block until LLM generation completes.
func (c *Client) SendMessage(sessionID, message string) (*MessageResponse, error) {
	payload := map[string]any{
		"parts": []map[string]string{
			{"type": "text", "text": message},
		},
		"role": "user",
	}
	body, _ := json.Marshal(payload)
	var resp MessageResponse
	// Use a short timeout — the POST may block until LLM finishes.
	// If it times out, that's OK; WaitForResult will pick up the result.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := c.doRequest(ctx, http.MethodPost, "/session/"+sessionID+"/message", bytes.NewReader(body), &resp)
	if err != nil {
		// Timeout is OK — the message was sent, we just didn't get the response.
		// WaitForResult will poll for it.
		var msgResp MessageResponse
		// Try to get the message we just sent
		msgs, getErr := c.GetMessages(sessionID)
		if getErr == nil && len(msgs) > 0 {
			msgResp = *msgs[len(msgs)-1]
			return &msgResp, nil
		}
		return nil, err
	}
	return &resp, err
}

// WaitForResult polls the session until the last message is complete or timeout.
// The messages slice on the returned Result contains all observed MessageResponses.
func (c *Client) WaitForResult(sessionID string, timeout time.Duration) (*Result, error) {
	return c.WaitForResultWith(sessionID, nil, timeout)
}

// WaitForResultWith is like WaitForResult but starts from an already-received
// MessageResponse (e.g. from a previous SendMessage call).
func (c *Client) WaitForResultWith(sessionID string, initial *MessageResponse, timeout time.Duration) (*Result, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	if initial != nil {
		// If the initial response already shows completion, return immediately
		if initial.Info != nil && initial.Info.Time != nil && initial.Info.Time.Completed > 0 {
			result := &Result{Messages: []*MessageResponse{initial}}
			extractMessagesResult(initial, result)
			if initial.Info.Error != nil && result.Error == "" {
				result.Error = initial.Info.Error.Name
				if initial.Info.Error.Data != nil {
					result.Error = initial.Info.Error.Data.Message
				}
			}
			return result, nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			return &Result{Messages: nil}, ErrTimeout
		case <-ticker.C:
			// Fetch messages from the session to check completion status
			messages, err := c.GetMessages(sessionID)
			if err != nil {
				continue
			}
			// Find the last assistant message with completion info
			for i := len(messages) - 1; i >= 0; i-- {
				m := messages[i]
				if m.Info != nil && m.Info.Time != nil && m.Info.Time.Completed > 0 {
					result := &Result{Messages: messages}
					extractMessagesResult(m, result)
					if m.Info.Error != nil && result.Error == "" {
						result.Error = m.Info.Error.Name
						if m.Info.Error.Data != nil {
							result.Error = m.Info.Error.Data.Message
						}
					}
					return result, nil
				}
			}
		}
	}
}

// DeleteSession removes a session from the server.
func (c *Client) DeleteSession(sessionID string) error {
	err := c.doRequest(context.Background(), http.MethodDelete, "/session/"+sessionID, nil, nil)
	if err != nil {
		return err
	}
	c.mu.Lock()
	sessions := make([]string, 0, len(c.sessions))
	for _, s := range c.sessions {
		if s != sessionID {
			sessions = append(sessions, s)
		}
	}
	c.sessions = sessions
	c.mu.Unlock()
	return nil
}

// Cleanup deletes all sessions created by this client.
// Safe to call multiple times.
func (c *Client) Cleanup() error {
	c.mu.Lock()
	sessions := make([]string, len(c.sessions))
	copy(sessions, c.sessions)
	c.sessions = nil
	c.mu.Unlock()

	var lastErr error
	for _, id := range sessions {
		if err := c.DeleteSession(id); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// extractMessagesResult populates Content and Error on Result from a MessageResponse.
func extractMessagesResult(resp *MessageResponse, result *Result) {
	if resp == nil {
		return
	}
	var sb bytes.Buffer
	if resp.Info != nil {
		if resp.Info.Error != nil {
			if resp.Info.Error.Data != nil {
				result.Error = resp.Info.Error.Data.Message
			} else {
				result.Error = resp.Info.Error.Name
			}
		}
	}
	for _, p := range resp.Parts {
		if p.Text != "" {
			sb.WriteString(p.Text)
		} else if p.Content != "" {
			sb.WriteString(p.Content)
		}
	}
	result.Content = sb.String()
}

// doRequest performs an HTTP request with retry logic and structured error wrapping.
// For GET/DELETE, body should be nil. For POST/PUT, pass the reader.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, out any) error {
	var lastErr error
	delay := retryBaseDelay

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				delay *= 2
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return fmt.Errorf("bad request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			if !isRetryable(err) {
				return lastErr
			}
			continue
		}

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			if out == nil {
				resp.Body.Close()
				return nil
			}
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				resp.Body.Close()
				return fmt.Errorf("decode error: %w", err)
			}
			resp.Body.Close()
			return nil
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Try to parse as APIResponse to extract structured errors
		var apiResp APIResponse[any]
		if json.Unmarshal(bodyBytes, &apiResp) == nil && len(apiResp.Error) > 0 {
			errStr := apiResp.Error[0].Message
			if apiResp.Error[0].Expected != "" {
				errStr = fmt.Sprintf("%s (expected %s)", errStr, apiResp.Error[0].Expected)
			}
			lastErr = fmt.Errorf("API error %d: %s", resp.StatusCode, errStr)
		} else {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
		}

		// Don't retry 4xx errors
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}

	return lastErr
}

// isRetryable returns true for network errors that warrant retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	retryable := []string{
		"connection refused",
		"connection reset",
		"connection timed out",
		"io: read/write on closed pipe",
	}
	for _, substr := range retryable {
		if bytes.Contains([]byte(msg), []byte(substr)) {
			return true
		}
	}
	return false
}
