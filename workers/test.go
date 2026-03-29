// test.go implements the Tester worker for AgentHub.
// It polls the test queue, runs tests, and reports results.
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

// TestPayload is the payload for a test task.
type TestPayload struct {
	Type      string `json:"type"`
	Project   string `json:"project"`
	Path      string `json:"path,omitempty"`
	TestSuite string `json:"test_suite,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// NewTestWorker creates a Test worker.
func NewTestWorker(api *API) *Worker {
	return &Worker{
		Name:      "tester",
		API:       api,
		PollPath:  "/tasks/poll?type=test",
		ClaimPath: "/tasks/%d/claim",
		Process:   testProcessor,
	}
}

func testProcessor(payload json.RawMessage) Result {
	var task TestPayload
	if err := json.Unmarshal(payload, &task); err != nil {
		return Result{Success: false, Error: fmt.Sprintf("unmarshal payload: %v", err)}
	}

	projectDir := task.Path
	if projectDir == "" {
		projectDir = fmt.Sprintf("/root/.openclaw/workspace-pm/projects/%s", task.Project)
	}

	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return Result{Success: false, Error: fmt.Sprintf("project not found: %s", projectDir)}
	}

	log.Printf("[tester] Running tests for %s (suite=%s)", task.Project, task.TestSuite)

	// Detect project type and run appropriate tests
	result := runTests(projectDir, task.TestSuite, task.Env)

	if result.ExitCode != 0 && result.ExitCode != 1 {
		return Result{
			Success: false,
			Error:   fmt.Sprintf("Test runner failed with exit code %d: %s", result.ExitCode, result.Stderr),
			Output:  result.Stdout + "\n" + result.Stderr,
		}
	}

	// Parse test results
	parsed := parseTestOutput(result.Stdout, result.Stderr)

	return Result{
		Success: result.ExitCode == 0,
		Output: formatTestOutput(parsed, result),
		Data: map[string]any{
			"passed":     parsed.Passed,
			"failed":     parsed.Failed,
			"skipped":    parsed.Skipped,
			"total":      parsed.Total,
			"duration_ms": parsed.DurationMs,
			"exit_code":  result.ExitCode,
			"all_passed": result.ExitCode == 0,
		},
	}
}

type testResult struct {
	Passed      int
	Failed      int
	Skipped     int
	Total       int
	DurationMs  int64
	TestCases   []testCase
}

type testCase struct {
	Name    string
	Status  string // "pass", "fail", "skip"
	Duration string
	Error   string
}

type runResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMs int64
}

func runTests(projectDir, suite string, env map[string]string) runResult {
	// Build environment
	envVars := os.Environ()
	for k, v := range env {
		envVars = append(envVars, k+"="+v)
	}

	var cmd *exec.Cmd

	switch detectProjectType(projectDir) {
	case "node", "javascript":
		cmd = exec.Command("npm", "test", "--", "--passWithNoTests")
		if suite != "" {
			cmd.Args = append(cmd.Args, "--testPathPattern="+suite)
		}
	case "rust":
		cmd = exec.Command("cargo", "test")
		if suite != "" {
			cmd.Args = append(cmd.Args, suite)
		}
		cmd.Args = append(cmd.Args, "--")
		cmd.Args = append(cmd.Args, "--test-threads=4")
	case "python":
		cmd = exec.Command("python", "-m", "pytest")
		if suite != "" {
			cmd.Args = append(cmd.Args, "-k", suite)
		}
	default:
		return runResult{
			Stdout:  "No test framework detected",
			Stderr:  "",
			ExitCode: 0,
		}
	}

	cmd.Dir = projectDir
	cmd.Env = envVars
	cmd.Env = append(cmd.Env, "TERM=dumb")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	stderrStr := stderr.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return runResult{
				Stdout:   stdout.String(),
				Stderr:   stderrStr,
				ExitCode: exitErr.ExitCode(),
			}
		}
		// Log the error for debugging (issue #6 fix)
		log.Printf("[tester] Test runner failed: %v", err)
		return runResult{
			Stdout:   stdout.String(),
			Stderr:   fmt.Sprintf("run error: %v", err),
			ExitCode: -1,
		}
	}

	return runResult{
		Stdout:   stdout.String(),
		Stderr:   stderrStr,
		ExitCode: 0,
		DurationMs: duration.Milliseconds(),
	}
}

func detectProjectType(dir string) string {
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		name := f.Name()
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

	// Try cargo test format
	if pass := regexp.MustCompile(`(\d+) passed`).FindStringSubmatch(combined); len(pass) > 1 {
		fmt.Sscanf(pass[1], "%d", &r.Passed)
	}
	if fail := regexp.MustCompile(`(\d+) failed`).FindStringSubmatch(combined); len(fail) > 1 {
		fmt.Sscanf(fail[1], "%d", &r.Failed)
	}
	if skip := regexp.MustCompile(`(\d+) ignored`).FindStringSubmatch(combined); len(skip) > 1 {
		fmt.Sscanf(skip[1], "%d", &r.Skipped)
	}
	if skip2 := regexp.MustCompile(`(\d+) skipped`).FindStringSubmatch(combined); len(skip2) > 1 {
		fmt.Sscanf(skip2[1], "%d", &r.Skipped)
	}
	// Jest/Vitest format
	if pass := regexp.MustCompile(`Tests:\s+(\d+)\s+passed`).FindStringSubmatch(combined); len(pass) > 1 {
		fmt.Sscanf(pass[1], "%d", &r.Passed)
	}
	if fail := regexp.MustCompile(`Tests:\s+(\d+)\s+failed`).FindStringSubmatch(combined); len(fail) > 1 {
		fmt.Sscanf(fail[1], "%d", &r.Failed)
	}
	if skip := regexp.MustCompile(`Tests:\s+\d+\s+passed.*?(\d+)\s+skipped`).FindStringSubmatch(combined); len(skip) > 1 {
		fmt.Sscanf(skip[1], "%d", &r.Skipped)
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
		// Try to extract failed test names
		failedRe := regexp.MustCompile(`(FAIL|PASS)[:\s]+(.+)`)
		for _, m := range failedRe.FindAllStringSubmatch(raw.Stdout, -1) {
			if m[1] == "FAIL" {
				b.WriteString("FAIL: " + m[2] + "\n")
			}
		}
	}

	if raw.ExitCode == 0 {
		b.WriteString("✅ All tests passed!")
	} else {
		b.WriteString("❌ Some tests failed.")
	}

	// Limit output
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
