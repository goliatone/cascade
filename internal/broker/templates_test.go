package broker

import (
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

func TestRenderTitle(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		SourceModule:  "github.com/example/dependency",
		SourceVersion: "v1.2.3",
		Repo:          "github.com/example/myapp",
		BranchName:    "update-dependency-v1.2.3",
	}

	result := &executor.Result{
		Status:     executor.StatusCompleted,
		CommitHash: "abc123",
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "default template",
			template: "",
			expected: "Update github.com/example/dependency to v1.2.3",
		},
		{
			name:     "custom template",
			template: "chore: bump {{.Module}} to {{.SourceVersion}}",
			expected: "chore: bump github.com/example/dependency to v1.2.3",
		},
		{
			name:     "template with repo",
			template: "[{{.Repo}}] Update {{.Module}} to {{.SourceVersion}}",
			expected: "[github.com/example/myapp] Update github.com/example/dependency to v1.2.3",
		},
		{
			name:     "template with functions",
			template: "{{.Module | upper}} -> {{.SourceVersion}}",
			expected: "GITHUB.COM/EXAMPLE/DEPENDENCY -> v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderTitle(tt.template, item, result)
			if err != nil {
				t.Fatalf("RenderTitle() error = %v", err)
			}
			if got != tt.expected {
				t.Fatalf("RenderTitle() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRenderTitleWithInvalidTemplate(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		SourceVersion: "v1.2.3",
	}

	// Invalid template syntax should return fallback and error
	title, err := RenderTitle("{{.InvalidField", item, nil)

	if err == nil {
		t.Fatal("Expected error for invalid template, got none")
	}

	var templateErr *TemplateRenderError
	if !isTemplateRenderError(err, &templateErr) {
		t.Fatalf("Expected TemplateRenderError, got %T", err)
	}

	expectedTitle := "Update github.com/example/dependency to v1.2.3"
	if title != expectedTitle {
		t.Fatalf("Expected fallback title %q, got %q", expectedTitle, title)
	}
}

func TestRenderBody(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		SourceModule:  "github.com/example/dependency",
		SourceVersion: "v1.2.3",
		Repo:          "github.com/example/myapp",
		BranchName:    "update-dependency-v1.2.3",
		Labels:        []string{"dependencies", "automated"},
	}

	result := &executor.Result{
		Status:     executor.StatusCompleted,
		Reason:     "Successfully updated dependency",
		CommitHash: "abc123def456",
		TestResults: []executor.CommandResult{
			{
				Command: manifest.Command{Cmd: []string{"go", "test", "./..."}},
				Output:  "PASS\nok  \tgithub.com/example/myapp\t0.123s",
			},
		},
		ExtraResults: []executor.CommandResult{
			{
				Command: manifest.Command{Cmd: []string{"go", "mod", "tidy"}},
				Output:  "go: tidied go.mod",
			},
		},
	}

	tests := []struct {
		name         string
		template     string
		wantContains []string
	}{
		{
			name:     "default template",
			template: "",
			wantContains: []string{
				"## Summary",
				"Updates github.com/example/dependency",
				"v1.2.3",
				"**Repository**: github.com/example/myapp",
				"**Branch**: update-dependency-v1.2.3",
				"**Status**: completed",
				"**Commit**: abc123def456",
				"## Details",
				"Successfully updated dependency",
				"## Test Results",
				"PASS",
				"## Additional Command Results",
				"go: tidied go.mod",
				"Generated at",
			},
		},
		{
			name:     "custom template",
			template: "# {{.Module}} Update\n\nStatus: {{.Status}}\nCommit: {{.CommitHash}}",
			wantContains: []string{
				"# github.com/example/dependency Update",
				"Status: completed",
				"Commit: abc123def456",
			},
		},
		{
			name:     "template with functions",
			template: "Module: {{.Module | upper}}\nLabels: {{join .Labels \", \"}}",
			wantContains: []string{
				"Module: GITHUB.COM/EXAMPLE/DEPENDENCY",
				"Labels: dependencies, automated",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderBody(tt.template, item, result)
			if err != nil {
				t.Fatalf("RenderBody() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Fatalf("RenderBody() missing content %q in output:\n%s", want, got)
				}
			}
		})
	}
}

func TestRenderBodyWithInvalidTemplate(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		SourceVersion: "v1.2.3",
	}

	result := &executor.Result{
		Status: executor.StatusFailed,
	}

	// Invalid template syntax should return fallback and error
	body, err := RenderBody("{{.InvalidField", item, result)

	if err == nil {
		t.Fatal("Expected error for invalid template, got none")
	}

	var templateErr *TemplateRenderError
	if !isTemplateRenderError(err, &templateErr) {
		t.Fatalf("Expected TemplateRenderError, got %T", err)
	}

	// Should contain fallback content - the fallback uses default template which is conditional
	expectedContains := []string{
		"github.com/example/dependency",
		"v1.2.3",
		"**Status**: failed",
	}

	for _, want := range expectedContains {
		if !strings.Contains(body, want) {
			t.Fatalf("Expected fallback body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestRenderBodyWithNilResult(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		SourceVersion: "v1.2.3",
		Repo:          "github.com/example/myapp",
	}

	body, err := RenderBody("", item, nil)
	if err != nil {
		t.Fatalf("RenderBody() error = %v", err)
	}

	// Should render without result data
	wantContains := []string{
		"Updates github.com/example/dependency",
		"v1.2.3",
		"github.com/example/myapp",
	}

	for _, want := range wantContains {
		if !strings.Contains(body, want) {
			t.Fatalf("RenderBody() missing content %q in output:\n%s", want, body)
		}
	}

	// Should not contain result-specific fields (they're conditional now)
	notWantContains := []string{
		"## Test Results",
		"## Details",
	}

	for _, notWant := range notWantContains {
		if strings.Contains(body, notWant) {
			t.Fatalf("RenderBody() should not contain %q when result is nil, got:\n%s", notWant, body)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "exact length",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "needs truncation",
			input:  "this is a very long string that needs truncation",
			maxLen: 20,
			want:   "this is a very lo...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateString(tt.input, tt.maxLen); got != tt.want {
				t.Fatalf("truncateString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "backticks",
			input: "code with `backticks`",
			want:  "code with \\`backticks\\`",
		},
		{
			name:  "asterisks",
			input: "text with *emphasis*",
			want:  "text with \\*emphasis\\*",
		},
		{
			name:  "html characters",
			input: "<script>alert('xss')</script>",
			want:  "&lt;script&gt;alert\\('xss'\\)&lt;/script&gt;",
		},
		{
			name:  "mixed characters",
			input: "# Header with `code` and <tags>",
			want:  "\\# Header with \\`code\\` and &lt;tags&gt;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeMarkdown(tt.input); got != tt.want {
				t.Fatalf("escapeMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandOutputTruncation(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		SourceVersion: "v1.2.3",
	}

	// Create very long output that should be truncated
	longOutput := strings.Repeat("This is a very long line of output that should be truncated. ", 50)

	result := &executor.Result{
		Status: executor.StatusCompleted,
		TestResults: []executor.CommandResult{
			{
				Command: manifest.Command{Cmd: []string{"go", "test"}},
				Output:  longOutput,
			},
		},
	}

	body, err := RenderBody("", item, result)
	if err != nil {
		t.Fatalf("RenderBody() error = %v", err)
	}

	// Verify that the output was truncated (should end with "...")
	if !strings.Contains(body, "...") {
		t.Fatal("Expected truncated output to contain '...', but it didn't")
	}

	// The body shouldn't contain the full long output
	if strings.Contains(body, longOutput) {
		t.Fatal("Expected long output to be truncated, but found full output in body")
	}
}

func TestBuildTemplateData(t *testing.T) {
	item := planner.WorkItem{
		Module:        "github.com/example/dependency",
		ModulePath:    "./go.mod",
		SourceModule:  "github.com/example/dependency",
		SourceVersion: "v1.2.3",
		Repo:          "github.com/example/myapp",
		Branch:        "main",
		BranchName:    "update-dependency-v1.2.3",
		CommitMessage: "Update dependency to v1.2.3",
		Labels:        []string{"dependencies"},
	}

	result := &executor.Result{
		Status:     executor.StatusCompleted,
		Reason:     "Update successful",
		CommitHash: "abc123",
	}

	data := buildTemplateData(item, result)

	// Verify work item data
	if data.Module != item.Module {
		t.Errorf("Expected Module %q, got %q", item.Module, data.Module)
	}
	if data.SourceVersion != item.SourceVersion {
		t.Errorf("Expected SourceVersion %q, got %q", item.SourceVersion, data.SourceVersion)
	}
	if data.Repo != item.Repo {
		t.Errorf("Expected Repo %q, got %q", item.Repo, data.Repo)
	}

	// Verify result data
	if data.Status != string(result.Status) {
		t.Errorf("Expected Status %q, got %q", result.Status, data.Status)
	}
	if data.CommitHash != result.CommitHash {
		t.Errorf("Expected CommitHash %q, got %q", result.CommitHash, data.CommitHash)
	}

	// Verify timestamp is recent
	if time.Since(data.Timestamp) > time.Minute {
		t.Error("Expected Timestamp to be recent")
	}
}

// Helper function to check if an error is a TemplateRenderError
func isTemplateRenderError(err error, target **TemplateRenderError) bool {
	if err == nil {
		return false
	}
	if templateErr, ok := err.(*TemplateRenderError); ok {
		*target = templateErr
		return true
	}
	return false
}
