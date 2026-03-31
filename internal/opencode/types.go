// Package opencode provides a Go client for the OpenCode HTTP API.
package opencode

// Session represents an OpenCode session.
type Session struct {
	ID         string              `json:"id"`
	Slug       string              `json:"slug"`
	ProjectID  string              `json:"projectID"`
	Directory  string              `json:"directory"`
	Title      string              `json:"title"`
	Version    string              `json:"version"`
	Summary    SessionSummary      `json:"summary"`
	Permission []SessionPermission `json:"permission"`
	Time       SessionTime         `json:"time"`
	Info       *MessageInfo        `json:"info,omitempty"`
}

// SessionSummary holds git diff stats for the session.
type SessionSummary struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Files     int `json:"files"`
}

// SessionTime holds session timestamps (milliseconds).
type SessionTime struct {
	Created  int64 `json:"created"`
	Updated  int64 `json:"updated"`
	Archived int64 `json:"archived,omitempty"`
}

// SessionPermission controls what the agent is allowed to do.
type SessionPermission struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"`
}

// MessagePart represents a single part of a message.
type MessagePart struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Content string `json:"content,omitempty"`
}

// MessageInfo holds metadata about a message exchange.
type MessageInfo struct {
	ID         string           `json:"id"`
	SessionID  string           `json:"sessionID"`
	Role       string           `json:"role"`
	ParentID   string           `json:"parentID,omitempty"`
	ModelID    string           `json:"modelID,omitempty"`
	ProviderID string           `json:"providerID,omitempty"`
	Mode       string           `json:"mode,omitempty"`
	Agent      string           `json:"agent,omitempty"`
	Error      *MessageError    `json:"error,omitempty"`
	Path       *MessagePath     `json:"path,omitempty"`
	Cost       float64          `json:"cost,omitempty"`
	Tokens     *MessageTokens   `json:"tokens,omitempty"`
	Time       *MessageTimeInfo `json:"time,omitempty"`
}

// MessageTimeInfo holds timing for a message exchange.
type MessageTimeInfo struct {
	Created   int64 `json:"created"`
	Completed int64 `json:"completed"`
}

// MessageTokens holds token usage for a message.
type MessageTokens struct {
	Input     int           `json:"input"`
	Output    int           `json:"output"`
	Reasoning int           `json:"reasoning"`
	Cache     *MessageCache `json:"cache,omitempty"`
}

// MessageCache holds cache read/write token counts.
type MessageCache struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

// MessagePath holds working directory info for a message.
type MessagePath struct {
	Cwd  string `json:"cwd"`
	Root string `json:"root"`
}

// MessageError holds error info when a message exchange fails.
type MessageError struct {
	Name string            `json:"name"`
	Data *MessageErrorData `json:"data,omitempty"`
}

// MessageErrorData holds the underlying error from the LLM provider.
type MessageErrorData struct {
	Message      string `json:"message"`
	StatusCode   int    `json:"statusCode"`
	IsRetryable  bool   `json:"isRetryable"`
	ResponseBody string `json:"responseBody,omitempty"`
	MetadataURL  string `json:"metadataURL,omitempty"`
}

// MessageResponse is the response from POST /session/:id/message.
type MessageResponse struct {
	Info      *MessageInfo  `json:"info,omitempty"`
	Parts     []MessagePart `json:"parts"`
	ID        string        `json:"id"`
	SessionID string        `json:"sessionID"`
}

// HealthResponse is the response from GET /global/health.
type HealthResponse struct {
	Healthy bool   `json:"healthy"`
	Version string `json:"version"`
}

// APIResponse is a generic wrapper for API responses that may contain errors.
type APIResponse[T any] struct {
	Data    T             `json:"data,omitempty"`
	Error   []ErrorDetail `json:"error,omitempty"`
	Success bool          `json:"success"`
}

// ErrorDetail describes a single validation error.
type ErrorDetail struct {
	Expected string `json:"expected"`
	Code     string `json:"code"`
	Path     []any  `json:"path"`
	Message  string `json:"message"`
}
