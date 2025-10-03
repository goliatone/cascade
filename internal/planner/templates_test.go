package planner

import "testing"

func TestRenderCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		template string
		target   Target
		expected string
	}{
		{
			name:     "empty template uses default",
			template: "",
			target:   Target{Module: "go-errors", Version: "v1.2.3"},
			expected: "Update go-errors to v1.2.3",
		},
		{
			name:     "template with both placeholders",
			template: "Update {{ module }} to {{ version }}",
			target:   Target{Module: "go-errors", Version: "v1.2.3"},
			expected: "Update go-errors to v1.2.3",
		},
		{
			name:     "template with dot notation placeholders",
			template: "chore(deps): bump {{ .Module }} to {{ .Version }}",
			target:   Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"},
			expected: "chore(deps): bump github.com/goliatone/go-errors to v1.2.3",
		},
		{
			name:     "template with only module placeholder",
			template: "Bump {{ module }} dependency",
			target:   Target{Module: "go-errors", Version: "v1.2.3"},
			expected: "Bump go-errors dependency",
		},
		{
			name:     "template with only version placeholder",
			template: "Update to {{ version }}",
			target:   Target{Module: "go-errors", Version: "v1.2.3"},
			expected: "Update to v1.2.3",
		},
		{
			name:     "template with no placeholders",
			template: "Update dependencies",
			target:   Target{Module: "go-errors", Version: "v1.2.3"},
			expected: "Update dependencies",
		},
		{
			name:     "template with multiple occurrences",
			template: "{{ module }}/{{ version }}: Update {{ module }} to {{ version }}",
			target:   Target{Module: "go-errors", Version: "v1.2.3"},
			expected: "go-errors/v1.2.3: Update go-errors to v1.2.3",
		},
		{
			name:     "template with unusual characters in target",
			template: "Update {{ module }} to {{ version }}",
			target:   Target{Module: "github.com/pkg/errors", Version: "v1.0.0-beta.1"},
			expected: "Update github.com/pkg/errors to v1.0.0-beta.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderCommitMessage(tt.template, tt.target)
			if result != tt.expected {
				t.Errorf("RenderCommitMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name     string
		module   string
		version  string
		expected string
	}{
		{
			name:     "simple module and version",
			module:   "go-errors",
			version:  "v1.2.3",
			expected: "auto/go-errors-v1.2.3",
		},
		{
			name:     "module with path",
			module:   "github.com/pkg/errors",
			version:  "v1.0.0",
			expected: "auto/errors-v1.0.0",
		},
		{
			name:     "uppercase gets lowercased",
			module:   "GO-ERRORS",
			version:  "V1.2.3",
			expected: "auto/go-errors-v1.2.3",
		},
		{
			name:     "spaces replaced with hyphens",
			module:   "my module",
			version:  "v1 2 3",
			expected: "auto/my-module-v1-2-3",
		},
		{
			name:     "problematic characters sanitized",
			module:   "mod@le#name",
			version:  "v1.0.0-beta+build",
			expected: "auto/mod-le-name-v1.0.0-beta-build",
		},
		{
			name:     "multiple slashes normalized",
			module:   "github.com//pkg//errors",
			version:  "v1.0.0",
			expected: "auto/errors-v1.0.0",
		},
		{
			name:     "consecutive hyphens cleaned up",
			module:   "mod--ule",
			version:  "v1--2--3",
			expected: "auto/mod-ule-v1-2-3",
		},
		{
			name:     "leading and trailing hyphens removed",
			module:   "-module-",
			version:  "-v1.0.0-",
			expected: "auto/module-v1.0.0",
		},
		{
			name:     "complex sanitization",
			module:   "GitHub.com/@User/My-Module",
			version:  "v1.0.0-beta.1+build.123",
			expected: "auto/my-module-v1.0.0-beta.1-build.123",
		},
		{
			name:     "all problematic characters",
			module:   "mod@#$%^&*()+=[]{}|\\:;\"'<>,?`~ule",
			version:  "v1.0.0",
			expected: "auto/mod-ule-v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateBranchName(tt.module, tt.version)
			if result != tt.expected {
				t.Errorf("GenerateBranchName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGenerateBranchNameDeterministic(t *testing.T) {
	// Test that the function is deterministic - same inputs produce same outputs
	module := "github.com/pkg/errors"
	version := "v1.0.0"

	result1 := GenerateBranchName(module, version)
	result2 := GenerateBranchName(module, version)

	if result1 != result2 {
		t.Errorf("GenerateBranchName is not deterministic: %q != %q", result1, result2)
	}
}

func TestRenderCommitMessageDeterministic(t *testing.T) {
	// Test that the function is deterministic - same inputs produce same outputs
	template := "Update {{ module }} to {{ version }}"
	target := Target{Module: "go-errors", Version: "v1.2.3"}

	result1 := RenderCommitMessage(template, target)
	result2 := RenderCommitMessage(template, target)

	if result1 != result2 {
		t.Errorf("RenderCommitMessage is not deterministic: %q != %q", result1, result2)
	}
}
