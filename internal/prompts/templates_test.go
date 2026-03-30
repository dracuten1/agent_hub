package prompts

import (
	"strings"
	"testing"
)

func TestDevTemplateContainsSections(t *testing.T) {
	ctx := "## Recent Commits:\nabc123 fix: admin seed"
	cfg := DefaultConfig()
	constr := Constraints{DoNotModify: []string{"internal/task/handler.go"}}

	prompt := DevTemplate(ctx, "Fix auth bug", "Fix the auth bug in delete", "high", cfg, constr)

	checks := []string{
		"## Context",
		"abc123 fix: admin seed",
		"## Task",
		"Title: Fix auth bug",
		"Description: Fix the auth bug",
		"Priority: high",
		"## Instructions",
		"## Build & Test",
		"go build ./...",
		"go test ./...",
		"## Constraints",
		"internal/task/handler.go",
		"## Expected Output",
		"files_changed",
		"commit_message",
	}
	for _, s := range checks {
		if !strings.Contains(prompt, s) {
			t.Errorf("DevTemplate missing: %q", s)
		}
	}
}

func TestDevTemplateMinimal(t *testing.T) {
	cfg := ProjectConfig{}
	prompt := DevTemplate("", "Title", "Desc", "", cfg, Constraints{})
	if !strings.Contains(prompt, "Title: Title") {
		t.Error("DevTemplate should include title")
	}
	if !strings.Contains(prompt, "Description: Desc") {
		t.Error("DevTemplate should include description")
	}
}

func TestDevTemplateNoConstraints(t *testing.T) {
	cfg := DefaultConfig()
	prompt := DevTemplate(ctx("", ""), "T", "D", "low", cfg, Constraints{})
	if strings.Contains(prompt, "## Constraints") {
		t.Error("DevTemplate should not include Constraints section when empty")
	}
}

func TestDevTemplateDefaults(t *testing.T) {
	cfg := ProjectConfig{}
	prompt := DevTemplate("", "T", "D", "", cfg, Constraints{})
	if !strings.Contains(prompt, "go build ./...") {
		t.Error("DevTemplate should default build cmd")
	}
	if !strings.Contains(prompt, "go test ./...") {
		t.Error("DevTemplate should default test cmd")
	}
}

func TestDevTemplateMultipleConstraints(t *testing.T) {
	cfg := DefaultConfig()
	constr := Constraints{
		DoNotModify: []string{"a.go", "b.go", "c.go"},
	}
	prompt := DevTemplate("", "T", "", "", cfg, constr)
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(prompt, f) {
			t.Errorf("DevTemplate should include constraint file: %s", f)
		}
	}
}

func TestReviewTemplateContainsSections(t *testing.T) {
	ctx := "## Git Diff:\n+ new code"
	cfg := DefaultConfig()
	prompt := ReviewTemplate(ctx, "Review auth handler", "security", cfg)

	checks := []string{
		"## Context",
		"## Task",
		"Review auth handler",
		"security vulnerabilities",
		"## Instructions",
		"## Focus Areas",
		"## Expected Output",
		"verdict",
		"severity",
		"issues",
	}
	for _, s := range checks {
		if !strings.Contains(prompt, s) {
			t.Errorf("ReviewTemplate missing: %q", s)
		}
	}
}

func TestReviewTemplateFocusAreas(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		taskType string
		expect   string
	}{
		{"security", "security vulnerabilities"},
		{"performance", "N+1 queries"},
		{"test", "test coverage"},
		{"", "code quality, bugs"},
	}
	for _, tt := range tests {
		prompt := ReviewTemplate("", "Review", tt.taskType, cfg)
		if !strings.Contains(prompt, tt.expect) {
			t.Errorf("ReviewTemplate(type=%q) should contain %q", tt.taskType, tt.expect)
		}
	}
}

func TestReviewTemplateMinimal(t *testing.T) {
	cfg := DefaultConfig()
	prompt := ReviewTemplate("", "Check code", "", cfg)
	if !strings.Contains(prompt, "## Task") {
		t.Error("ReviewTemplate should include Task section")
	}
	if !strings.Contains(prompt, "## Expected Output") {
		t.Error("ReviewTemplate should include Expected Output section")
	}
}

func TestTestTemplateContainsSections(t *testing.T) {
	ctx := "## Build:\ngo build"
	cfg := DefaultConfig()
	prompt := TestTemplate(ctx, "Test auth flow", "unit", cfg)

	checks := []string{
		"## Context",
		"## Task",
		"Test auth flow",
		"## Instructions",
		"## Test Focus",
		"individual function behavior",
		"## Test Command",
		"go test ./...",
		"## Expected Output",
		"verdict",
		"tests_run",
		"failures",
	}
	for _, s := range checks {
		if !strings.Contains(prompt, s) {
			t.Errorf("TestTemplate missing: %q", s)
		}
	}
}

func TestTestTemplateFocusAreas(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		taskType string
		expect   string
	}{
		{"integration", "end-to-end flows"},
		{"unit", "edge cases"},
		{"performance", "response times"},
		{"", "functional correctness"},
	}
	for _, tt := range tests {
		prompt := TestTemplate("", "Test", tt.taskType, cfg)
		if !strings.Contains(prompt, tt.expect) {
			t.Errorf("TestTemplate(type=%q) should contain %q", tt.taskType, tt.expect)
		}
	}
}

func TestTestTemplateMinimal(t *testing.T) {
	cfg := ProjectConfig{}
	prompt := TestTemplate("", "Run tests", "", cfg)
	if !strings.Contains(prompt, "Run tests") {
		t.Error("TestTemplate should include task description")
	}
}

func TestTestTemplateFramework(t *testing.T) {
	cfg := ProjectConfig{Framework: "gotest"}
	prompt := TestTemplate("", "Test", "", cfg)
	if !strings.Contains(prompt, "gotest") {
		t.Error("TestTemplate should include framework")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.BuildCmd != "go build ./..." {
		t.Errorf("Expected build cmd, got %q", cfg.BuildCmd)
	}
	if cfg.TestCmd != "go test ./..." {
		t.Errorf("Expected test cmd, got %q", cfg.TestCmd)
	}
	if cfg.Framework != "testing" {
		t.Errorf("Expected framework, got %q", cfg.Framework)
	}
}

func TestFormatConstraints(t *testing.T) {
	c := Constraints{DoNotModify: []string{"a.go", "b.go"}}
	out := FormatConstraints(c)
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.go") {
		t.Error("FormatConstraints should include files")
	}

	empty := FormatConstraints(Constraints{})
	if empty != "" {
		t.Errorf("FormatConstraints empty should return empty string, got %q", empty)
	}
}

func TestDevTemplateContextInjection(t *testing.T) {
	ctx := "## Custom Context\ncustom content here"
	cfg := DefaultConfig()
	prompt := DevTemplate(ctx, "T", "D", "", cfg, Constraints{})
	if !strings.Contains(prompt, "## Custom Context") {
		t.Error("DevTemplate should inject context")
	}
	if !strings.Contains(prompt, "custom content here") {
		t.Error("DevTemplate should include context content")
	}
}

func TestReviewTemplateContextInjection(t *testing.T) {
	ctx := "## Diff:\n--- a/file.go"
	cfg := DefaultConfig()
	prompt := ReviewTemplate(ctx, "Review", "", cfg)
	if !strings.Contains(prompt, "## Diff:") {
		t.Error("ReviewTemplate should inject context")
	}
}

func TestTestTemplateContextInjection(t *testing.T) {
	ctx := "## Changes:\n+ new feature"
	cfg := DefaultConfig()
	prompt := TestTemplate(ctx, "Test", "", cfg)
	if !strings.Contains(prompt, "## Changes:") {
		t.Error("TestTemplate should inject context")
	}
}

// ctx is a helper to create a minimal context string for tests.
func ctx(k, v string) string { return "## " + k + "\n" + v }
