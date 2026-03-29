// review.go implements the Review worker for AgentHub.
// It polls the review queue, runs code review via OpenCode, and reports results.
package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// NewReviewWorker creates a Review worker.
func NewReviewWorker(api *API) *Worker {
	return NewWorker("reviewer", api, "/tasks/poll?type=review", "/tasks/%d/claim", reviewProcessor)
}

func reviewProcessor(payload json.RawMessage) Result {
	var task ReviewPayload
	if err := json.Unmarshal(payload, &task); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("unmarshal payload: %v", err)}
	}

	projectDir := task.PRURL
	if projectDir == "" {
		projectDir = fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", task.Project)
	}

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return Result{Success: false, Error: fmt.Sprintf("project not found: %s", projectDir)}
	}

	// Build review command
	command := buildReviewCommand(task, projectDir)
	log.Printf("[reviewer] Reviewing %s (pr=%s)", task.Project, task.PRURL)

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

	return Result{
		Success: true,
		Output:  summarizeReview(output),
		Data: map[string]any{
			"issues_found": len(issues),
			"issues":       issues,
			"approved":     approved,
			"pr_url":       task.PRURL,
		},
	}
}

func buildReviewCommand(task ReviewPayload, projectDir string) string {
	var parts []string

	if task.PRURL != "" {
		parts = append(parts, fmt.Sprintf(
			"Review the PR at %s for the %s project.", task.PRURL, task.Project,
		))
	} else {
		parts = append(parts, fmt.Sprintf(
			"Review the code changes in %s.", projectDir,
		))
	}

	parts = append(parts, "Check for:")
	parts = append(parts, "- Logic errors or bugs")
	parts = append(parts, "- Security issues")
	parts = append(parts, "- Code quality and style")
	parts = append(parts, "- Test coverage")
	parts = append(parts, "- API design consistency")
	parts = append(parts, "Report findings to PM using sessions_send.")

	return strings.Join(parts, " ")
}

func extractReviewIssues(output string) []map[string]string {
	var issues []map[string]string
	// Look for issue patterns like "- [ ] Issue description"
	re := regexp.MustCompile(`(?m)^[-*]\s*(?:\[.\]|\[[xX]\])?\s*(.+)$`)
	matches := re.FindAllStringSubmatch(output, -1)
	for _, m := range matches {
		text := strings.TrimSpace(m[1])
		if text != "" && len(text) > 5 {
			severity := "medium"
			if strings.Contains(strings.ToLower(text), "security") ||
				strings.Contains(strings.ToLower(text), "critical") {
				severity = "high"
			} else if strings.Contains(strings.ToLower(text), "nit") ||
				strings.Contains(strings.ToLower(text), "minor") {
				severity = "low"
			}
			issues = append(issues, map[string]string{
				"description": text,
				"severity":    severity,
			})
		}
	}
	// Limit to 50 issues
	if len(issues) > 50 {
		issues = issues[:50]
	}
	return issues
}

func isApproved(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "lgtm") ||
		strings.Contains(lower, "looks good") ||
		strings.Contains(lower, "approved") ||
		strings.Contains(lower, "✓") ||
		strings.Contains(lower, "✔")
}

func summarizeReview(output string) string {
	lines := strings.Split(output, "\n")
	// Skip very long lines and limit
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
