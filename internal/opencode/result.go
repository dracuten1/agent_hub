package opencode

// Result holds the outcome of an OpenCode session run.
type Result struct {
	// Content is the concatenated text from the assistant's reply parts.
	Content string
	// Error is set if the run encountered an error.
	Error string
	// Files is the list of files modified during the run (populated from
	// session summary when available).
	Files []string
	// Messages holds the message history for the session.
	Messages []*MessageResponse
}
