// dev.go implements the Dev worker for AgentHub.
// It polls the dev queue, claims tasks, executes them with OpenCode, and reports results.
package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// TaskDPayload is the payload for a dev task (Task D).
type TaskDPayload struct {
	Type      string `json:"type"`
	Project   string `json:"project"`
	TaskID    int    `json:"task_id"`
	Model     string `json:"model,omitempty"`
	Path      string `json:"path,omitempty"`
	Command   string `json:"command,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
	ProjectID int    `json:"project_id,omitempty"`
}

// NewDevWorker creates a Dev worker.
func NewDevWorker(api *API) *Worker {
	return &Worker{
		Name:      "dev",
		API:       api,
		PollPath:  "/tasks/poll?type=dev",
		ClaimPath: "/tasks/%d/claim",
		Process:   devProcessor,
	}
}

func devProcessor(payload json.RawMessage) Result {
	var task TaskDPayload
	if err := json.Unmarshal(payload, &task); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("unmarshal payload: %v", err)}
	}

	projectDir := task.Path
	if projectDir == "" {
		projectDir = fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", task.Project)
	}

	// Verify project exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return Result{Success: false, Error: fmt.Sprintf("project not found: %s", projectDir)}
	}

	// Determine command to run
	command := task.Command
	if command == "" {
		command = buildDevCommand(task)
	}

	if command == "" {
		return Result{Success: false, Error: "no command specified and could not build one"}
	}

	log.Printf("[dev] Running: %s (project=%s, dir=%s)", command, task.Project, projectDir)

	// Run OpenCode
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

	// Extract useful info from output
	filesChanged := extractFilesChanged(output)
	commitHash := extractCommitHash(output)

	return Result{
		Success: true,
		Output:  summarizeOutput(output),
		Data: map[string]any{
			"files_changed": filesChanged,
			"commit_hash":   commitHash,
			"project":      task.Project,
		},
	}
}

func buildDevCommand(task TaskDPayload) string {
	// Build OpenCode command based on task type
	switch task.Type {
	case "dev_task":
		return fmt.Sprintf(
			"Implement Task #%d for project %s at %s. Report back when done using sessions_send.",
			task.TaskID, task.Project, task.Path,
		)
	case "fix":
		return fmt.Sprintf(
			"Fix the issue in project %s at %s. Task ID: %d. Report results via sessions_send.",
			task.Project, task.Path, task.TaskID,
		)
	default:
		return fmt.Sprintf(
			"Work on %s task for project %s at %s. Report completion to PM.",
			task.Type, task.Project, task.Path,
		)
	}
}

func summarizeOutput(output string) string {
	lines := strings.Split(output, "\n")
	// Take first 100 lines as summary
	if len(lines) > 100 {
		return strings.Join(lines[:100], "\n") + "\n... (truncated)"
	}
	return output
}

func extractFilesChanged(output string) []string {
	// Look for git diff output or file patterns
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func init() {
	// Register dev worker as a runnable command
}
