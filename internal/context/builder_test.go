package context

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func hasGit(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	_, err := cmd.Output()
	return err == nil
}

func requireGit(t *testing.T, workDir string) {
	if !hasGit(workDir) {
		t.Skip("git not available in this environment")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	requireGit(t, workDir)
	branch, err := GetCurrentBranch(workDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch == "" {
		t.Error("Expected non-empty branch name")
	}
	t.Logf("Current branch: %s", branch)
}

func TestGetRecentCommits(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	requireGit(t, workDir)
	commits, err := GetRecentCommits(workDir, 3)
	if err != nil {
		t.Fatalf("GetRecentCommits failed: %v", err)
	}
	if len(commits) == 0 {
		t.Error("Expected at least one commit")
	}
	for _, c := range commits {
		t.Logf("Commit: %s", c)
	}
}

func TestGetGitDiff(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	requireGit(t, workDir)
	diff, err := GetGitDiff(workDir, []string{"internal/context/builder.go"}, 20)
	if err != nil {
		t.Fatalf("GetGitDiff failed: %v", err)
	}
	t.Logf("Diff length: %d chars", len(diff))
}

func TestGetGitDiffNoFiles(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	requireGit(t, workDir)
	diff, err := GetGitDiff(workDir, nil, 20)
	if err != nil {
		t.Fatalf("GetGitDiff(nil) failed: %v", err)
	}
	t.Logf("Diff (no files) length: %d chars", len(diff))
}

func TestGetGitDiffNonGitDir(t *testing.T) {
	// Use a guaranteed non-git directory: /tmp itself (not inside a git repo)
	tmpDir := "/tmp/nonexistent_context_test_dir"
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	diff, err := GetGitDiff(tmpDir, nil, 20)
	if err != nil {
		t.Errorf("Expected no error for non-git dir, got: %v", err)
	}
	if diff != "" {
		t.Errorf("Expected empty diff for non-git dir, got %d chars", len(diff))
	}
}

func TestLoadFileContent(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); os.IsNotExist(err) {
		t.Skip("project files not mounted in this environment")
	}
	content, err := LoadFileContent(workDir, "go.mod")
	if err != nil {
		t.Fatalf("LoadFileContent failed: %v", err)
	}
	if !strings.Contains(content, "module github.com/tuyen/agenthub") {
		t.Errorf("Expected go.mod content, got: %s", content)
	}
	if len(content) > 100 {
		t.Logf("go.mod content preview: %s", content[:100])
	} else {
		t.Logf("go.mod content: %s", content)
	}
}

func TestLoadFileContentMissing(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	_, err := LoadFileContent(workDir, "nonexistent/file.go")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}

func TestLoadConventionsOpencode(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	conv, err := LoadConventions(workDir)
	if err != nil {
		t.Fatalf("LoadConventions failed: %v", err)
	}
	t.Logf("Conventions length: %d chars", len(conv))
	if conv != "" {
		if len(conv) > 200 {
			t.Logf("Conventions preview: %s", conv[:200])
		} else {
			t.Logf("Conventions: %s", conv)
		}
	}
}

func TestLoadConventionsNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	conv, err := LoadConventions(tmpDir)
	if err != nil {
		t.Fatalf("LoadConventions should not error on missing files: %v", err)
	}
	if conv != "" {
		t.Errorf("Expected empty conventions, got: %s", conv)
	}
}

func TestBuildContext(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	branch, _ := GetCurrentBranch(workDir)

	ctx := TaskContext{
		Title:         "Fix auth bug",
		Description:   "Fix the auth bug where admin users cannot delete agents",
		ProjectDir:    workDir,
		Branch:        branch,
		AffectedFiles: []string{"internal/auth/handler.go", "cmd/server/main.go"},
		Constraints: []string{
			"Do NOT modify: internal/task/handler.go (assigned to Dev2)",
		},
		BuildCmd: "go build ./...",
		TestCmd:  "go test ./...",
	}

	output, err := BuildContext(ctx)
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}

	// Verify expected sections
	sections := []string{
		"## Task: Fix auth bug",
		"## Project:",
		"## Affected Files:",
		"## Constraints:",
		"## Description:",
		"go build ./...",
		"Fix the auth bug",
	}
	for _, s := range sections {
		if !strings.Contains(output, s) {
			t.Errorf("Expected output to contain %q", s)
		}
	}
	t.Logf("BuildContext output (%d chars):\n%s", len(output), output)
}

func TestBuildContextMinimal(t *testing.T) {
	ctx := TaskContext{
		Title:       "Simple task",
		Description: "Just do the thing.",
		ProjectDir:  "/tmp",
		Branch:      "main",
	}

	output, err := BuildContext(ctx)
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}
	if !strings.Contains(output, "## Task: Simple task") {
		t.Error("Expected task title in output")
	}
	if !strings.Contains(output, "## Description:") {
		t.Error("Expected description section")
	}
}

func TestBuildContextNoGit(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := TaskContext{
		Title:       "No git task",
		Description: "Working in a non-git dir.",
		ProjectDir:  tmpDir,
		Branch:      "main",
	}

	output, err := BuildContext(ctx)
	if err != nil {
		t.Fatalf("BuildContext should not fail for non-git dirs: %v", err)
	}
	if !strings.Contains(output, "## Task: No git task") {
		t.Error("Expected task title")
	}
}

func TestRunBuildCmd(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); os.IsNotExist(err) {
		t.Skip("project files not mounted in this environment")
	}
	out, err := RunBuildCmd(workDir, "go version")
	if err != nil {
		t.Errorf("RunBuildCmd failed: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "go version") {
		t.Errorf("Expected go version output, got: %s", out)
	}
}

func TestRunBuildCmdEmpty(t *testing.T) {
	out, err := RunBuildCmd("/tmp", "")
	if err != nil {
		t.Errorf("RunBuildCmd with empty cmd should not error: %v", err)
	}
	if out != "" {
		t.Errorf("Expected empty output for empty cmd, got: %s", out)
	}
}

func TestRunBuildCmdFails(t *testing.T) {
	workDir := "/root/.openclaw/workspace-pm/projects/agenthub"
	_, err := RunBuildCmd(workDir, "exit 1")
	if err == nil {
		t.Error("Expected error for failing command")
	}
}

func TestExtractReadmeIntro(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		contains    string
		notContains string
	}{
		{
			name:     "stops at h2",
			input:    "Intro line\n\n## Section Two\n\nMore content",
			contains: "Intro line",
		},
		{
			name:        "h1 not treated as h2",
			input:       "# Project Title\n\n## Section One\n\nExtra content",
			contains:    "# Project Title",
			notContains: "# Project Title\n\n## Section One",
		},
		{
			name:     "full content if no h2",
			input:    "All content here\nNo h2 headings",
			contains: "No h2 headings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := extractReadmeIntro(tt.input)
			if !strings.Contains(out, tt.contains) {
				t.Errorf("extractReadmeIntro(%q) should contain %q, got %q", tt.input, tt.contains, out)
			}
			if tt.notContains != "" && strings.Contains(out, tt.notContains) {
				t.Errorf("extractReadmeIntro(%q) should NOT contain %q, got %q", tt.input, tt.notContains, out)
			}
		})
	}
}

func TestLoadFileContentLarge(t *testing.T) {
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.go")
	var buf strings.Builder
	buf.WriteString("package main\n\n")
	for i := 0; i < 600; i++ {
		buf.WriteString("func line() {}\n")
	}
	os.WriteFile(largeFile, []byte(buf.String()), 0644)

	content, err := LoadFileContent(tmpDir, "large.go")
	if err != nil {
		t.Fatalf("LoadFileContent failed: %v", err)
	}
	if !strings.Contains(content, "truncated") {
		t.Error("Expected truncation note for large file")
	}
	if strings.Contains(content, "line 501") || strings.Contains(content, "line 599") {
		t.Error("Expected truncated content to not include lines after 500")
	}
}
