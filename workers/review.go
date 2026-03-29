// review.go implements the Review worker for AgentHub.
package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// NewReviewWorker creates a Review worker that polls the task queue.
func NewReviewWorker(api *API) *Worker {
	return NewWorker("reviewer", api, "/api/agent/tasks/queue", "/api/agent/tasks/%s/claim", reviewProcessor)
}

func reviewProcessor(t *Task) Result {
	log.Printf("[reviewer] Reviewing: %s", t.Title)

	// Determine project dir
	projectDir := detectReviewDir(t)

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return Result{Success: false, Error: fmt.Sprintf("project directory not found: %s", projectDir)}
	}

	// Build review command
	command := buildReviewPrompt(t, projectDir)
	log.Printf("[reviewer] Running review in %s...", projectDir)

	stdout, stderr, err := RunOpenCode(command)
	output := stdout
	if stderr != "" {
		output += "\n[STDERR]\n" + stderr
	}

	if err != nil {
		return Result{
			Success: false,
			Error:   fmt.Sprintf("Review failed: %v", err),
			Output:  summarizeReview(output),
		}
	}

	// Parse review findings
	issues := extractReviewIssues(output)
	approved := isApproved(output)

	// Determine verdict and severity
	verdict := "pass"
	severity := "minor"
	if !approved {
		verdict = "fail"
		severity = "major"
	}
	if hasCriticalIssues(issues) {
		verdict = "fail"
		severity = "critical"
	}

	return Result{
		Success: true,
		Output:  summarizeReview(output),
		Data: map[string]any{
			"verdict":     verdict,
			"severity":    severity,
			"issues":      len(issues),
			"approved":    approved,
			"issues_list": issues,
		},
	}
}

// detectReviewDir figures out the project path for a review task.
func detectReviewDir(t *Task) string {
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

	lower := strings.ToLower(t.Title) + strings.ToLower(t.Description)
	for _, name := range []string{"my-tasks", "agenthub", "taskmaster"} {
		if strings.Contains(lower, name) {
			return fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", name)
		}
	}
	return "/root/.openclaw/workspace-pm"
}

// buildReviewPrompt builds an OpenCode prompt for code review.
func buildReviewPrompt(t *Task, projectDir string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Review the task: %s", t.Title))
	if t.Description != "" {
		parts = append(parts, fmt.Sprintf("Task description: %s", t.Description))
	}
	parts = append(parts, fmt.Sprintf("Code location: %s", projectDir))
	parts = append(parts, "Review the code changes. Check for:")
	parts = append(parts, "- Logic errors or bugs")
	parts = append(parts, "- Security issues")
	parts = append(parts, "- Code quality and style")
	parts = append(parts, "- Test coverage")
	parts = append(parts, "- API design consistency")
	parts = append(parts, "Use markdown format. Include verdict (pass/fail) and list specific issues.")
	return strings.Join(parts, "\n")
}

func extractReviewIssues(output string) []map[string]string {
	var issues []map[string]string
	re := regexp.MustCompile(`(?m)^[-*]\s*(?:\[.\]|\[[xX]\])?\s*(.+)$`)
	matches := re.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		text := strings.TrimSpace(m[1])
		if text != "" && len(text) > 5 && !isHeadingOrEmpty(text) {
			severity := "medium"
			lower := strings.ToLower(text)
			if strings.Contains(lower, "security") || strings.Contains(lower, "critical") {
				severity = "high"
			} else if strings.Contains(lower, "nit:") || strings.Contains(lower, "minor") {
				severity = "low"
			}
			issues = append(issues, map[string]string{
				"description": text,
				"severity":    severity,
			})
		}
	}
	if len(issues) > 50 {
		issues = issues[:50]
	}
	return issues
}

func isHeadingOrEmpty(s string) bool {
	// Skip lines that look like markdown headings
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if len(s) < 2 {
		return true
	}
	// Headings start with # or are all caps short strings
	if strings.HasPrefix(s, "#") {
		return true
	}
	return false
}

func hasCriticalIssues(issues []map[string]string) bool {
	for _, issue := range issues {
		if issue["severity"] == "high" {
			return true
		}
	}
	return false
}

func isApproved(output string) bool {
	lower := strings.ToLower(output)
	// Check for pass/approved signals
	passSignals := []string{"lgtm", "looks good", "looks good to me", "approved", "✅", "✔", "✓", "pass", "all good"}
	for _, sig := range passSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	// Negative signals override
	negativeSignals := []string{"fails review", "not approved", "❌", "✗", "fail"}
	for _, sig := range negativeSignals {
		if strings.Contains(lower, sig) {
			return false
		}
	}
	// Default to false (requires explicit approval)
	return false
}

func summarizeReview(output string) string {
	lines := strings.Split(output, "\n")
	var short []string
	for i, line := range lines {
		if i > 80 {
			break
		}
		if len(line) > 200 {
			line = line[:200] + "..."
		}
		short = append(short, line)
	}
	return strings.Join(short, "\n")
}

// RunReviewWorker starts the reviewer worker.
func RunReviewWorker() {
	apiBase := getEnv("AGENTHUB_URL", "http://localhost:8081")
	token := getEnv("AGENTHUB_TOKEN", "")

	api := NewAPI(apiBase, token)
	worker := NewReviewWorker(api)

	if err := worker.Run(); err != nil {
		log.Fatalf("Reviewer worker fatal: %v", err)
	}
}
