package workerbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/tuyen/agenthub/internal/worker"
)

// Agent invokes the OpenClaw agent CLI for a given task.
type Agent struct {
	cfg *Config
}

// NewAgent creates a new agent with the given config.
func NewAgent(cfg *Config) *Agent {
	return &Agent{cfg: cfg}
}

// TaskResult is the processed result from an agent run.
type TaskResult struct {
	Success      bool
	Output       string
	FilesChanged []string
	CommitHash   string
	Error        string
	Duration     time.Duration
}

// CLIResult represents the JSON output from `openclaw agent --json`.
type cliResult struct {
	RunID   string `json:"runId"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Result  struct {
		Payloads []struct {
			Text string `json:"text"`
		} `json:"payloads"`
	} `json:"result"`
}

// Run invokes the openclaw agent CLI and returns the parsed result.
func (a *Agent) Run(ctx context.Context, task worker.Task) *TaskResult {
	start := time.Now()

	prompt := a.buildPrompt(task)

	args := []string{
		"agent",
		"--agent", a.cfg.AgentID,
		"--message", prompt,
		"--timeout", fmt.Sprintf("%d", a.cfg.TaskTimeout),
		"--json",
	}
	if a.cfg.SessionID != "" {
		args = append(args, "--session-id", a.cfg.SessionID)
	}

	cmd := exec.CommandContext(ctx, "openclaw", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("[Agent] openclaw agent --agent %s (timeout=%ds)", a.cfg.AgentID, a.cfg.TaskTimeout)
	if task.WorkflowID != "" {
		log.Printf("[Agent] Workflow context: phase=%s (index %d), workflow_id=%s",
			task.WorkflowPhase, task.WorkflowPhaseIndex, task.WorkflowID)
	}

	err := cmd.Run()
	result := &TaskResult{Duration: time.Since(start)}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if stderr.Len() > 0 {
			result.Error += "\n" + stderr.String()
		}
		return result
	}

	// Parse JSON output
	var parsed cliResult
	rawOutput := stdout.String()
	if err := json.Unmarshal([]byte(rawOutput), &parsed); err != nil {
		// Non-JSON output — use as-is
		result.Success = len(rawOutput) > 0
		result.Output = rawOutput
		return result
	}

	result.Success = parsed.Status == "ok"

	// Extract text from payloads
	var sb strings.Builder
	for _, p := range parsed.Result.Payloads {
		if p.Text != "" {
			sb.WriteString(p.Text)
		}
	}
	result.Output = sb.String()

	if result.Output != "" {
		result.FilesChanged = extractFilesChanged(result.Output)
		result.CommitHash = extractCommitHash(result.Output)
	}

	return result
}

// buildPrompt constructs the agent prompt from a task + config.
// Project-specific info comes from config, not hardcoded.
func (a *Agent) buildPrompt(task worker.Task) string {
	var sb strings.Builder

	// Task content
	sb.WriteString(task.Title)
	if task.Description != "" {
		sb.WriteString("\n\n")
		sb.WriteString(task.Description)
	}

	// Project context from config (not hardcoded)
	projectDir := a.cfg.ProjectDir
	if projectDir == "" && task.ProjectDir != "" {
		projectDir = task.ProjectDir
	}

	if projectDir != "" {
		sb.WriteString(fmt.Sprintf("\n\nProject directory: %s", projectDir))
	}
	if a.cfg.BuildCmd != "" {
		sb.WriteString(fmt.Sprintf("\nBuild command: %s", a.cfg.BuildCmd))
	}
	if a.cfg.TestCmd != "" {
		sb.WriteString(fmt.Sprintf("\nTest command: %s", a.cfg.TestCmd))
	}

	sb.WriteString("\n\nImplement the task, run build to verify. Reply done with files changed.")

	// Append workflow context if present
	if task.WorkflowID != "" {
		sb.WriteString(fmt.Sprintf("\n\n[Workflow context: phase=%s (index %d), workflow_id=%s]",
			task.WorkflowPhase, task.WorkflowPhaseIndex, task.WorkflowID))
	}

	return sb.String()
}

// extractFilesChanged extracts file names from agent output text.
func extractFilesChanged(output string) []string {
	var files []string
	seen := make(map[string]bool)

	re := regexp.MustCompile(`(?i)(?:changed|modified|created|added):\s*(.+?)(?:\n|$)`)
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		if len(m) > 1 {
			for _, f := range strings.Split(m[1], ",") {
				f = strings.TrimSpace(f)
				if f != "" && !seen[f] {
					seen[f] = true
					files = append(files, f)
				}
			}
		}
	}
	return files
}

// extractCommitHash extracts a git commit hash from agent output text.
func extractCommitHash(output string) string {
	re := regexp.MustCompile(`(?:commit|hash)[\s:]+([a-f0-9]{7,40})`)
	m := re.FindStringSubmatch(output)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
