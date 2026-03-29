// test.go implements the Tester worker for AgentHub.
package workers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// NewTestWorker creates a Test worker that polls the task queue.
func NewTestWorker(api *API) *Worker {
	return NewWorker("tester", api, "/api/agent/tasks/queue", "/api/agent/tasks/%s/claim", testProcessor)
}

func testProcessor(t *Task) Result {
	log.Printf("[tester] Running tests for: %s", t.Title)

	// Determine project dir and test suite from payload
	projectDir, suite := detectTestTarget(t)

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return Result{Success: false, Error: fmt.Sprintf("project directory not found: %s", projectDir)}
	}

	log.Printf("[tester] Project: %s, Suite: %s", projectDir, suite)

	result := runTests(projectDir, suite)

	if result.ExitCode < 0 {
		return Result{
			Success: false,
			Error:   fmt.Sprintf("Test runner failed: %s", result.Stderr),
			Output:  result.Stdout + "\n" + result.Stderr,
		}
	}

	parsed := parseTestOutput(result.Stdout, result.Stderr)

	verdict := "pass"
	var issues []string
	if result.ExitCode != 0 || parsed.Failed > 0 {
		verdict = "fail"
		issues = extractFailedTestNames(result.Stdout)
	}

	return Result{
		Success: result.ExitCode == 0,
		Output:  formatTestOutput(parsed, result),
		Data: map[string]any{
			"verdict":     verdict,
			"passed":      parsed.Passed,
			"failed":      parsed.Failed,
			"skipped":     parsed.Skipped,
			"total":       parsed.Total,
			"duration_ms": parsed.DurationMs,
			"exit_code":   result.ExitCode,
			"issues":      issues,
		},
	}
}

// detectTestTarget extracts project dir and test suite from task.
func detectTestTarget(t *Task) (string, string) {
	projectDir := ""
	suite := ""

	if t.Payload != nil {
		var payload map[string]any
		if json.Unmarshal(t.Payload, &payload) == nil {
			if path, ok := payload["path"].(string); ok && path != "" {
				projectDir = path
			}
			if project, ok := payload["project"].(string); ok && project != "" {
				if projectDir == "" {
					projectDir = fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", project)
				}
			}
			if s, ok := payload["test_suite"].(string); ok {
				suite = s
			}
		}
	}

	if projectDir == "" {
		// Try to detect from title/description
		lower := strings.ToLower(t.Title) + strings.ToLower(t.Description)
		for _, name := range []string{"my-tasks", "agenthub", "taskmaster"} {
			if strings.Contains(lower, name) {
				projectDir = fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", name)
				break
			}
		}
		if projectDir == "" {
			projectDir = "/root/.openclaw/workspace-pm/projects/my-tasks"
		}
	}

	return projectDir, suite
}

type testResult struct {
	Passed     int
	Failed     int
	Skipped    int
	Total      int
	DurationMs int64
}

type runResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMs int64
}

func runTests(projectDir, suite string) runResult {
	envVars := os.Environ()

	var cmd *exec.Cmd
	switch detectProjectType(projectDir) {
	case "rust":
		cmd = exec.Command("cargo", "test")
		if suite != "" {
			cmd.Args = append(cmd.Args, suite)
		}
		cmd.Args = append(cmd.Args, "--", "--test-threads=4")
	case "node", "javascript":
		cmd = exec.Command("npm", "test", "--", "--passWithNoTests")
		if suite != "" {
			cmd.Args = append(cmd.Args, "--testPathPattern="+suite)
		}
	case "python":
		cmd = exec.Command("python", "-m", "pytest")
		if suite != "" {
			cmd.Args = append(cmd.Args, "-k", suite)
		}
		cmd.Args = append(cmd.Args, "-v")
	default:
		log.Printf("[tester] No test framework detected in %s", projectDir)
		return runResult{Stdout: "No test framework detected", ExitCode: 0}
	}

	cmd.Dir = projectDir
	cmd.Env = append(envVars, "TERM=dumb")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	stderrStr := stderr.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			log.Printf("[tester] Test runner failed: %v (exit %d)", err, code)
			return runResult{
				Stdout:     stdout.String(),
				Stderr:     stderrStr,
				ExitCode:   code,
				DurationMs: duration.Milliseconds(),
			}
		}
		log.Printf("[tester] Test runner error: %v", err)
		return runResult{
			Stdout:     stdout.String(),
			Stderr:     fmt.Sprintf("run error: %v", err),
			ExitCode:   -1,
			DurationMs: duration.Milliseconds(),
		}
	}

	return runResult{
		Stdout:     stdout.String(),
		Stderr:     stderrStr,
		ExitCode:   0,
		DurationMs: duration.Milliseconds(),
	}
}

func detectProjectType(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "unknown"
	}
	for _, e := range entries {
		name := e.Name()
		if name == "Cargo.toml" {
			return "rust"
		}
		if name == "package.json" {
			return "node"
		}
		if name == "requirements.txt" || name == "pyproject.toml" {
			return "python"
		}
	}
	return "unknown"
}

func parseTestOutput(stdout, stderr string) testResult {
	combined := stdout + "\n" + stderr
	var r testResult

	// cargo test format: "N passed", "N failed", "N ignored"
	for _, pattern := range []struct {
		re   *regexp.Regexp
		dest *int
	}{
		{regexp.MustCompile(`(\d+) passed`), &r.Passed},
		{regexp.MustCompile(`(\d+) failed`), &r.Failed},
		{regexp.MustCompile(`(\d+) ignored`), &r.Skipped},
	} {
		if m := pattern.re.FindStringSubmatch(combined); len(m) > 1 {
			fmt.Sscanf(m[1], "%d", pattern.dest)
		}
	}

	// Jest/Vitest format: "Tests: N passed, N failed, N skipped"
	if m := regexp.MustCompile(`Tests:\s+(?:(\d+)\s+passed,?\s+)?(?:(\d+)\s+failed,?\s+)?(?:(\d+)\s+skipped)?`).FindStringSubmatch(combined); len(m) > 1 {
		if m[1] != "" {
			fmt.Sscanf(m[1], "%d", &r.Passed)
		}
		if m[2] != "" {
			fmt.Sscanf(m[2], "%d", &r.Failed)
		}
		if len(m) > 3 && m[3] != "" {
			fmt.Sscanf(m[3], "%d", &r.Skipped)
		}
	}

	r.Total = r.Passed + r.Failed + r.Skipped

	// Duration
	if d := regexp.MustCompile(`Time:\s+(\d+\.?\d*)s`).FindStringSubmatch(combined); len(d) > 1 {
		var seconds float64
		fmt.Sscanf(d[1], "%f", &seconds)
		r.DurationMs = int64(seconds * 1000)
	}

	return r
}

func extractFailedTestNames(output string) []string {
	var names []string
	re := regexp.MustCompile(`(?m)^(FAIL|PASS|FAILED|TIMEOUT)[:\s]+(.+)$`)
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		if m[1] == "FAIL" || m[1] == "FAILED" || m[1] == "TIMEOUT" {
			name := strings.TrimSpace(m[2])
			if name != "" {
				names = append(names, name)
			}
		}
	}
	if len(names) > 20 {
		names = names[:20]
	}
	return names
}

func formatTestOutput(parsed testResult, raw runResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tests: %d total, %d passed, %d failed, %d skipped\n",
		parsed.Total, parsed.Passed, parsed.Failed, parsed.Skipped))
	if parsed.DurationMs > 0 {
		b.WriteString(fmt.Sprintf("Duration: %dms\n", parsed.DurationMs))
	}
	b.WriteString("\n")

	if parsed.Failed > 0 {
		b.WriteString("--- FAILED TESTS ---\n")
		re := regexp.MustCompile(`(?m)^(FAIL|FAILED)[:\s]+(.+)$`)
		for _, m := range re.FindAllStringSubmatch(raw.Stdout, -1) {
			b.WriteString("FAIL: " + strings.TrimSpace(m[2]) + "\n")
		}
	}

	if raw.ExitCode == 0 {
		b.WriteString("✅ All tests passed!")
	} else {
		b.WriteString(fmt.Sprintf("❌ Tests failed (exit code %d).", raw.ExitCode))
	}

	out := b.String()
	if len(out) > 3000 {
		out = out[:3000] + "\n... (output truncated)"
	}
	return out
}

// RunTestWorker starts the tester worker.
func RunTestWorker() {
	apiBase := getEnv("AGENTHUB_URL", "http://localhost:8081")
	token := getEnv("AGENTHUB_TOKEN", "")

	api := NewAPI(apiBase, token)
	worker := NewTestWorker(api)

	if err := worker.Run(); err != nil {
		log.Fatalf("Tester worker fatal: %v", err)
	}
}
