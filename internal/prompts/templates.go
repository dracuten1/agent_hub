package prompts

// ProjectConfig holds project-specific configuration for prompts.
type ProjectConfig struct {
	BuildCmd   string
	TestCmd    string
	Framework  string
	Language   string
}

// DefaultConfig returns a sensible default config for a Go project.
func DefaultConfig() ProjectConfig {
	return ProjectConfig{
		BuildCmd:  "go build ./...",
		TestCmd:   "go test ./...",
		Framework: "testing",
		Language:  "Go",
	}
}

// Constraints holds file-level constraints for a task.
type Constraints struct {
	DoNotModify []string // files the agent must NOT touch
}

// DevTemplate generates a prompt for a development task.
func DevTemplate(context string, taskTitle string, taskDescription string, priority string, cfg ProjectConfig, constraints Constraints) string {
	buildCmd := cfg.BuildCmd
	if buildCmd == "" {
		buildCmd = "go build ./..."
	}
	testCmd := cfg.TestCmd
	if testCmd == "" {
		testCmd = "go test ./..."
	}

	s := `You are a developer. Implement the task below using the provided context.

## Context
` + context + `

## Task
Title: ` + taskTitle

	if taskDescription != "" {
		s += `
Description: ` + taskDescription
	}

	if priority != "" {
		s += `
Priority: ` + priority
	}

	s += `

## Instructions
1. Read and understand the codebase using the provided context
2. Implement the required changes
3. Run the build command to verify compilation
4. Fix any build errors
5. Run tests if available
6. Report what files you changed and a summary of changes

## Build & Test
` + buildCmd + `
` + testCmd

	if len(constraints.DoNotModify) > 0 {
		s += `

## Constraints
- Do NOT modify the following files (assigned to other agents):`
		for _, f := range constraints.DoNotModify {
			s += `
- ` + f
		}
	}

	s += `
- Follow existing code style and conventions
- Keep changes minimal and focused on the task

## Expected Output
Return a JSON object:
{
  "files_changed": ["file1.go", "file2.go"],
  "summary": "brief description of changes",
  "commit_message": "type: short description"
}`

	return s
}

// ReviewTemplate generates a prompt for a code review task.
func ReviewTemplate(context string, taskDescription string, taskType string, cfg ProjectConfig) string {
	s := `You are a code reviewer. Review the changes described below.

## Context
` + context + `

## Task
` + taskDescription

	focusAreas := "code quality, bugs, error handling, and style consistency"
	switch taskType {
	case "security":
		focusAreas = "security vulnerabilities, injection risks, authentication/authorization flaws, and sensitive data exposure"
	case "performance":
		focusAreas = "N+1 queries, missing indexes, inefficient algorithms, and memory leaks"
	case "test":
		focusAreas = "test coverage, missing edge cases, flaky tests, and test quality"
	}

	s += `

## Instructions
1. Read the affected files using the provided context
2. Review for: ` + focusAreas + `
3. Check code style consistency with the existing codebase
4. Provide a clear verdict and list any issues

## Focus Areas
` + focusAreas

	s += `

## Expected Output
Return a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "severity": "critical" | "major" | "minor" | "none",
  "issues": [
    {"file": "file.go", "line": 42, "severity": "major", "description": "issue description"}
  ]
}`

	return s
}

// TestTemplate generates a prompt for a testing task.
func TestTemplate(context string, taskDescription string, taskType string, cfg ProjectConfig) string {
	s := `You are a QA tester. Test the changes described below.

## Context
` + context + `

## Task
` + taskDescription

	testFocus := "functional correctness and edge cases"
	switch taskType {
	case "integration":
		testFocus = "end-to-end flows, service boundaries, and data integrity"
	case "unit":
		testFocus = "individual function behavior, edge cases, and boundary conditions"
	case "performance":
		testFocus = "load handling, response times, and scalability"
	}

	s += `

## Instructions
1. Read the changes and understand what was modified
2. Run the build to verify compilation
3. Run the test suite
4. If tests fail, describe each failure in detail
5. Check for edge cases and regression risks
6. Report a verdict

## Test Focus
` + testFocus

	if cfg.TestCmd != "" {
		s += `

## Test Command
` + cfg.TestCmd
	}

	if cfg.Framework != "" {
		s += `

## Test Framework
` + cfg.Framework
	}

	s += `

## Expected Output
Return a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "tests_run": N,
  "tests_passed": N,
  "tests_failed": N,
  "failures": [
    {"name": "TestName", "error": "error message"}
  ]
}`

	return s
}

// FormatConstraints converts a Constraints struct to a human-readable string.
func FormatConstraints(c Constraints) string {
	if len(c.DoNotModify) == 0 {
		return ""
	}
	s := "Do NOT modify:"
	for _, f := range c.DoNotModify {
		s += " " + f
	}
	return s
}
