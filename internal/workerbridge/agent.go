package workerbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/tuyen/agenthub/internal/worker"
)

type Agent struct {
	cfg *Config
}

func NewAgent(cfg *Config) *Agent {
	return &Agent{cfg: cfg}
}

type TaskResult struct {
	Success      bool
	Output       string
	FilesChanged []string
	CommitHash   string
	Error        string
	Duration     time.Duration
}

func (a *Agent) Run(ctx context.Context, task worker.Task) *TaskResult {
	start := time.Now()
	result := &TaskResult{Duration: time.Since(start)}

	prompt := buildPrompt(task)
	sessionID := generateSessionID(task.ID)

	cmd := exec.CommandContext(ctx, "openclaw", "agent",
		"--agent", a.cfg.AgentID,
		"--session-id", sessionID,
		"--message", prompt,
		"--timeout", fmt.Sprintf("%d", a.cfg.TaskTimeout),
		"--json",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if stderr.Len() > 0 {
			result.Error += "\n" + stderr.String()
		}
		return result
	}

	output := stdout.String()
	if output == "" {
		result.Success = false
		result.Error = "empty output from agent"
		return result
	}

	var resp struct {
		Result struct {
			Payloads []struct {
				Text string `json:"text"`
			} `json:"payloads"`
		} `json:"result"`
	}

	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to parse JSON: %v\nraw output: %s", err, output)
		return result
	}

	var sb bytes.Buffer
	for _, p := range resp.Result.Payloads {
		sb.WriteString(p.Text)
	}
	result.Output = sb.String()

	if result.Output == "" {
		result.Success = false
		result.Error = "no text output from agent payloads"
		return result
	}

	result.Success = true
	result.FilesChanged = extractFilesChanged(result.Output)
	result.CommitHash = extractCommitHash(result.Output)

	return result
}

func buildPrompt(task worker.Task) string {
	desc := task.Title
	if task.Description != "" {
		desc += "\n\n" + task.Description
	}
	return desc
}

func generateSessionID(taskID string) string {
	return fmt.Sprintf("bridge-%s-%d", taskID, time.Now().Unix())
}

func extractFilesChanged(output string) []string {
	var files []string
	seen := make(map[string]bool)

	re := regexp.MustCompile(`(?i)(?:changed(?:\s+files)?|modified|created|added|renamed):\s*(.+?)(?:\n|$)`)
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		if len(m) > 1 {
			for _, f := range splitCSV(m[1]) {
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

func extractCommitHash(output string) string {
	re := regexp.MustCompile(`(?:commit|hash)[\s:]+([a-f0-9]{7,40})`)
	m := re.FindStringSubmatch(output)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func splitCSV(s string) []string {
	var result []string
	var current []byte
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, string(current))
			current = nil
		} else {
			current = append(current, s[i])
		}
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	return result
}
