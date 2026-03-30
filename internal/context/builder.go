package context

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// TaskContext holds all input needed to build a rich context prompt.
type TaskContext struct {
	Title          string
	Description    string
	ProjectDir     string
	Branch         string
	AffectedFiles  []string
	Constraints    []string // files NOT to modify
	BuildCmd       string
	TestCmd        string
}

// BuildContext assembles a rich context prompt for OpenCode from a TaskContext.
func BuildContext(ctx TaskContext) (string, error) {
	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("## Task: %s\n", ctx.Title))
	b.WriteString(fmt.Sprintf("## Project: %s (branch: %s)\n",
		filepath.Base(ctx.ProjectDir), ctx.Branch))
	if len(ctx.AffectedFiles) > 0 {
		b.WriteString(fmt.Sprintf("## Affected Files: %s\n",
			strings.Join(ctx.AffectedFiles, ", ")))
	}
	b.WriteString("\n")

	// Recent commits
	commits, err := GetRecentCommits(ctx.ProjectDir, 5)
	if err == nil && len(commits) > 0 {
		b.WriteString("## Recent Commits:\n")
		for _, c := range commits {
			b.WriteString(c + "\n")
		}
		b.WriteString("\n")
	}

	// Git diff
	diff, err := GetGitDiff(ctx.ProjectDir, ctx.AffectedFiles, 20)
	if err == nil && diff != "" {
		b.WriteString("## Git Diff:\n")
		b.WriteString(diff)
		b.WriteString("\n")
	}

	// Affected file contents
	if len(ctx.AffectedFiles) > 0 {
		b.WriteString("## Affected File Contents:\n\n")
		for _, f := range ctx.AffectedFiles {
			content, err := LoadFileContent(ctx.ProjectDir, f)
			if err != nil {
				b.WriteString(fmt.Sprintf("  [Could not load %s: %v]\n\n", f, err))
				continue
			}
			b.WriteString(fmt.Sprintf("--- %s ---\n", f))
			b.WriteString(content)
			b.WriteString("\n\n")
		}
	}

	// Conventions
	conventions, err := LoadConventions(ctx.ProjectDir)
	if err == nil && conventions != "" {
		b.WriteString("## Conventions:\n")
		b.WriteString(conventions)
		b.WriteString("\n\n")
	}

	// Constraints
	if len(ctx.Constraints) > 0 || ctx.BuildCmd != "" || ctx.TestCmd != "" {
		b.WriteString("## Constraints:\n")
		for _, c := range ctx.Constraints {
			b.WriteString(fmt.Sprintf("- %s\n", c))
		}
		if ctx.BuildCmd != "" {
			b.WriteString(fmt.Sprintf("- Build: %s\n", ctx.BuildCmd))
		}
		if ctx.TestCmd != "" {
			b.WriteString(fmt.Sprintf("- Test: %s\n", ctx.TestCmd))
		}
		b.WriteString("\n")
	}

	// Description
	b.WriteString("## Description:\n")
	b.WriteString(ctx.Description)

	return b.String(), nil
}

// GetGitDiff returns the git diff for the given files in workDir.
// If files is empty, returns the diff of the most recent commit (HEAD vs HEAD~1).
// If files is non-empty, returns the diff for those files from the last commit.
// contextLines controls how many lines of surrounding context to include.
// Returns an empty string (not an error) for non-git directories.
func GetGitDiff(workDir string, files []string, contextLines int) (string, error) {
	if contextLines <= 0 {
		contextLines = 20
	}

	var cmd *exec.Cmd
	if len(files) == 0 {
		// Diff of the last commit: compare HEAD to its parent
		// First check that HEAD~1 exists (i.e., there is a parent commit)
		checkCmd := exec.Command("git", "rev-parse", "--verify", "HEAD~1")
		checkCmd.Dir = workDir
		if err := checkCmd.Run(); err != nil {
			// No parent commit (e.g. initial commit with no history)
			return "", nil
		}
		cmd = exec.Command("git", "diff", fmt.Sprintf("-U%d", contextLines), "HEAD~1", "HEAD")
	} else {
		// Diff for specific files in the last commit
		// Check that HEAD~1 exists first
		checkCmd := exec.Command("git", "rev-parse", "--verify", "HEAD~1")
		checkCmd.Dir = workDir
		if err := checkCmd.Run(); err != nil {
			return "", nil
		}
		args := []string{"diff", fmt.Sprintf("-U%d", contextLines), "HEAD~1", "HEAD", "--"}
		args = append(args, files...)
		cmd = exec.Command("git", args...)
	}
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		if isNonGitError(err) {
			return "", nil
		}
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	return string(out), nil
}

// isNonGitError returns true for errors that indicate a non-git directory.
func isNonGitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	nonGit := []string{
		"no repository",
		"not a git",
		"not a repository",
		"not a working tree",
	}
	for _, s := range nonGit {
		if strings.Contains(msg, s) {
			return true
		}
	}
	// Also check exit code 128 which covers "no such ref" / "unknown revision" / "ambiguous"
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		return true
	}
	return false
}

// LoadFileContent reads a file and returns its full content.
// Files larger than 500 lines are truncated with a note.
func LoadFileContent(workDir, filePath string) (string, error) {
	fullPath := filepath.Join(workDir, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", filePath, err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 500 {
		truncated := strings.Join(lines[:500], "\n")
		return truncated + fmt.Sprintf("\n... [%d lines truncated, file too large]", len(lines)-500), nil
	}

	return string(data), nil
}

// LoadConventions reads project conventions from, in order:
//  1. .opencode.md
//  2. AGENTS.md
//  3. README.md (first section, up to H2 or blank line separator)
func LoadConventions(workDir string) (string, error) {
	candidates := []string{".opencode.md", "AGENTS.md", "README.md"}

	for _, name := range candidates {
		path := filepath.Join(workDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		// For README.md, extract just the intro section (before first H2)
		if name == "README.md" {
			content = extractReadmeIntro(content)
		}

		return content, nil
	}

	return "", nil // No conventions file found — not an error
}

// extractReadmeIntro returns the first section of a README, stopping at the first ## H2 heading
// or double-blank-line paragraph break.
func extractReadmeIntro(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	h2Re := regexp.MustCompile(`^##\s`)

	for _, line := range lines {
		if h2Re.MatchString(line) && len(result) > 0 {
			break
		}
		// Stop at double blank lines (section separator)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" && len(result) >= 2 &&
			strings.TrimSpace(result[len(result)-1]) == "" &&
			strings.TrimSpace(result[len(result)-2]) != "" {
			break
		}
		result = append(result, line)
	}

	out := strings.TrimRight(strings.Join(result, "\n"), "\n")
	if out == "" {
		return text
	}
	return out
}

// GetRecentCommits returns the last n commit messages in --oneline format.
func GetRecentCommits(workDir string, n int) ([]string, error) {
	if n <= 0 {
		n = 5
	}
	cmd := exec.Command("git", "log", "--oneline", fmt.Sprintf("-n%d", n))
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(err.Error(), "no repository") || strings.Contains(err.Error(), "not a git") ||
			strings.Contains(err.Error(), "not a repository") {
			return nil, nil
		}
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	var commits []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			commits = append(commits, l)
		}
	}
	return commits, nil
}

// GetCurrentBranch returns the name of the current git branch.
func GetCurrentBranch(workDir string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(err.Error(), "no repository") || strings.Contains(err.Error(), "not a git") ||
			strings.Contains(err.Error(), "not a repository") {
			return "", nil
		}
		return "", fmt.Errorf("git branch failed: %w", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		// Detached HEAD state
		branch = "(detached)"
	}
	return branch, nil
}

// RunBuildCmd executes the project's build command and returns its output.
func RunBuildCmd(workDir, cmdStr string) (string, error) {
	if cmdStr == "" {
		return "", nil
	}
	shell := "/bin/sh"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
	}
	cmd := exec.Command(shell, "-c", cmdStr)
	cmd.Dir = workDir
	var stderr, stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	if err != nil {
		return output, fmt.Errorf("build command failed: %w", err)
	}
	return output, nil
}
