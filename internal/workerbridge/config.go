package workerbridge

import (
	"os"
	"strconv"
)

// Config holds all configuration for the worker bridge.
type Config struct {
	// AgentHub API
	APIURL     string // AgentHub API base URL
	AgentToken string // Agent auth token (ah_... prefix)
	TaskType   string // Queue filter: "dev", "review", "test"

	// OpenClaw agent CLI
	AgentID   string // OpenClaw agent ID to invoke: dev1, dev2, reviewer, tester
	SessionID string // Optional: reuse existing session for recovery

	// Project (from env or task payload)
	ProjectDir string // Working directory for the project
	BuildCmd   string // Command to verify build
	TestCmd    string // Command to run tests

	// Behavior
	PollInterval  int // seconds between queue polls
	TaskTimeout   int // seconds to wait for agent to complete
	IdleThreshold int // minutes without session activity before nudge
}

// FromEnv reads all config from environment variables.
func FromEnv() *Config {
	return &Config{
		APIURL:        getEnv("AGENTHUB_URL", "http://localhost:8081"),
		AgentToken:   getEnv("AGENT_TOKEN", ""),
		TaskType:      getEnv("WORKER_ROLE", "dev"),
		AgentID:       getEnv("AGENT_ID", ""),
		SessionID:     getEnv("SESSION_ID", ""),
		ProjectDir:    os.Getenv("PROJECT_DIR"),
		BuildCmd:      getEnv("BUILD_CMD", "go build ./..."),
		TestCmd:       getEnv("TEST_CMD", "go test ./..."),
		PollInterval:  getEnvInt("POLL_INTERVAL_SECONDS", 10),
		TaskTimeout:   getEnvInt("TASK_TIMEOUT_MINUTES", 30) * 60,
		IdleThreshold: getEnvInt("IDLE_THRESHOLD_MINUTES", 5),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
