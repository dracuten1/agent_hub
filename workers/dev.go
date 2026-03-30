// dev.go implements the Dev worker for AgentHub.
package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// NewDevWorker creates a Dev worker that polls the shared task queue.
func NewDevWorker(api *API) *Worker {
	return NewWorker("dev", api, "/api/agent/tasks/queue?task_type=dev", "/api/agent/tasks/%s/claim", devProcessor)
}

func devProcessor(t *Task) Result {
	log.Printf("[dev] Processing: %s", t.Title)
	log.Printf("[dev] Description: %s", t.Description)

	// Determine project dir — try payload, then derive from skills/title
	projectDir := detectProjectDir(t)

	// Verify project exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return Result{Success: false, Error: fmt.Sprintf("project directory not found: %s", projectDir)}
	}

	// Build OpenCode prompt from task title + description
	command := buildDevPrompt(t, projectDir)
	log.Printf("[dev] Running OpenCode in %s...", projectDir)

	stdout, stderr, err := RunOpenCode(command)
	output := stdout
	if stderr != "" {
		output += "\n[STDERR]\n" + stderr
	}

	if err != nil {
		return Result{
			Success: false,
			Error:   fmt.Sprintf("OpenCode failed: %v", err),
			Output:  output,
		}
	}

	filesChanged := extractFilesChanged(output)
	commitHash := extractCommitHash(output)

	return Result{
		Success: true,
		Output:  summarizeOutput(output),
		Data: map[string]any{
			"files_changed": filesChanged,
			"commit_hash":   commitHash,
			"project_dir":   projectDir,
			"task_id":      t.ID,
		},
	}
}

// detectProjectDir figures out the project path from task metadata.
func detectProjectDir(t *Task) string {
	// Try to extract project name from payload
	if t.Payload != nil {
		var payload map[string]any
		if json.Unmarshal(t.Payload, &payload) == nil {
			if path, ok := payload["path"].(string); ok && path != "" {
				return path
			}
			if project, ok := payload["project"].(string); ok && project != "" {
				return fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", project)
			}
		}
	}

	// Try to derive from task title (e.g. "Fix auth bug in my-tasks")
	lower := strings.ToLower(t.Title)
	for _, name := range []string{"my-tasks", "agenthub", "taskmaster", "ydda", "roastmy"} {
		if strings.Contains(lower, name) {
			return fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", name)
		}
	}

	// Default workspace
	return "/root/.openclaw/workspace-pm"
}

// buildDevPrompt builds an OpenCode prompt from task title and description.
func buildDevPrompt(t *Task, projectDir string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Task: %s", t.Title))
	if t.Description != "" {
		parts = append(parts, fmt.Sprintf("Details: %s", t.Description))
	}
	if len(t.Skills) > 0 {
		parts = append(parts, fmt.Sprintf("Required skills: %s", strings.Join(t.Skills, ", ")))
	}
	parts = append(parts, fmt.Sprintf("Project path: %s", projectDir))
	parts = append(parts, "Work in the project directory. When done, report completion.")
	return strings.Join(parts, "\n")
}

func summarizeOutput(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) > 100 {
		return strings.Join(lines[:100], "\n") + "\n... (truncated)"
	}
	return output
}

func extractFilesChanged(output string) []string {
	re := regexp.MustCompile(`(?:changed|modified|created):\s*(.+?)(?:\n|$)`)
	matches := re.FindAllStringSubmatch(output, -1)
	var files []string
	seen := make(map[string]bool)
	for _, m := range matches {
		for _, f := range strings.Split(m[1], ",") {
			f = strings.TrimSpace(f)
			if f != "" && !seen[f] {
				seen[f] = true
				files = append(files, f)
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

// RunDevWorker starts the dev worker.
func RunDevWorker() {
	apiBase := getEnv("AGENTHUB_URL", "http://localhost:8081")
	token := getEnv("AGENTHUB_TOKEN", "")

	api := NewAPI(apiBase, token)
	worker := NewDevWorker(api)

	if err := worker.Run(); err != nil {
		log.Fatalf("Dev worker fatal: %v", err)
	}
}
