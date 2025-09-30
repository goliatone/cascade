package broker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/goliatone/cascade/internal/planner"
)

func TestParseRepoString_Comprehensive(t *testing.T) {
	tests := []struct {
		name        string
		repo        string
		wantOwner   string
		wantName    string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid owner/name format",
			repo:      "octocat/Hello-World",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantErr:   false,
		},
		{
			name:      "valid with dots and underscores",
			repo:      "my-org/my.repo_name",
			wantOwner: "my-org",
			wantName:  "my.repo_name",
			wantErr:   false,
		},
		{
			name:      "GitHub HTTPS URL",
			repo:      "https://github.com/octocat/Hello-World",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantErr:   false,
		},
		{
			name:      "GitHub HTTPS URL with .git suffix",
			repo:      "https://github.com/octocat/Hello-World.git",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantErr:   false,
		},
		{
			name:      "GitHub Enterprise URL",
			repo:      "https://github.enterprise.com/octocat/Hello-World",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantErr:   false,
		},
		{
			name:        "empty string",
			repo:        "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "missing slash",
			repo:        "octocat",
			wantErr:     true,
			errContains: "must be non-empty",
		},
		{
			name:        "too many slashes",
			repo:        "octocat/Hello-World/extra",
			wantErr:     true,
			errContains: "owner/name",
		},
		{
			name:        "empty owner",
			repo:        "/Hello-World",
			wantErr:     true,
			errContains: "non-empty",
		},
		{
			name:        "empty name",
			repo:        "octocat/",
			wantErr:     true,
			errContains: "invalid repository format",
		},
		{
			name:        "invalid characters in owner",
			repo:        "octo@cat/Hello-World",
			wantErr:     true,
			errContains: "invalid owner",
		},
		{
			name:        "invalid characters in name",
			repo:        "octocat/Hello World!",
			wantErr:     true,
			errContains: "invalid repository",
		},
		{
			name:        "owner starts with hyphen",
			repo:        "-octocat/Hello-World",
			wantErr:     true,
			errContains: "invalid owner",
		},
		{
			name:        "name ends with hyphen",
			repo:        "octocat/Hello-World-",
			wantErr:     true,
			errContains: "invalid repository",
		},
		{
			name:        "invalid URL format",
			repo:        "https://github.com/octocat",
			wantErr:     true,
			errContains: "invalid HTTPS URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, name, err := ParseRepoString(tt.repo)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseRepoString() expected error, got none")
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("ParseRepoString() error = %v, want substring %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRepoString() unexpected error = %v", err)
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("ParseRepoString() owner = %v, want %v", owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("ParseRepoString() name = %v, want %v", name, tt.wantName)
			}
		})
	}
}

func TestGenerateBranchName(t *testing.T) {
	tests := []struct {
		name string
		item planner.WorkItem
		want string
	}{
		{
			name: "explicit branch name provided",
			item: planner.WorkItem{
				Module:        "github.com/example/module",
				SourceVersion: "v1.2.3",
				BranchName:    "custom-branch-name",
			},
			want: "custom-branch-name",
		},
		{
			name: "generate from module and version",
			item: planner.WorkItem{
				Module:        "github.com/example/module",
				SourceVersion: "v1.2.3",
			},
			want: "cascade-update-github-com-example-module-v1-2-3",
		},
		{
			name: "sanitize special characters",
			item: planner.WorkItem{
				Module:        "github.com/@org/module+extras",
				SourceVersion: "v1.2.3+build.1",
			},
			want: "cascade-update-github-com-at-org-module-plus-extras-v1-2-3-plus-build-1",
		},
		{
			name: "handle very long names",
			item: planner.WorkItem{
				Module:        "github.com/very-long-organization-name/very-long-module-name-that-exceeds-normal-limits-and-should-be-truncated-because-git-has-branch-name-limits-and-we-need-to-respect-them-for-proper-git-operations-to-work-correctly",
				SourceVersion: "v1.2.3-very-long-prerelease-version-string",
			},
			want: "cascade-update-67697468",
		},
		{
			name: "handle empty fields gracefully",
			item: planner.WorkItem{
				Module:        "",
				SourceVersion: "",
			},
			want: "cascade-update-branch-branch", // gitutil.SanitizeBranchName("") returns "branch"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateBranchName(tt.item)
			if got != tt.want {
				t.Errorf("GenerateBranchName() = %v, want %v", got, tt.want)
			}

			// Ensure branch name is valid length
			if len(got) > 250 {
				t.Errorf("GenerateBranchName() produced branch name too long: %d characters", len(got))
			}
		})
	}
}

func TestValidatePRInput(t *testing.T) {
	validInput := &PRInput{
		Repo:       "octocat/Hello-World",
		BaseBranch: "main",
		HeadBranch: "feature-branch",
		Title:      "Update dependency",
		Body:       "This PR updates a dependency to fix security issues.",
		Labels:     []string{"enhancement", "dependencies"},
	}

	tests := []struct {
		name        string
		input       *PRInput
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid input",
			input:   validInput,
			wantErr: false,
		},
		{
			name:        "nil input",
			input:       nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name: "empty repo",
			input: &PRInput{
				Repo:       "",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      "Title",
			},
			wantErr:     true,
			errContains: "repository is required",
		},
		{
			name: "invalid repo format",
			input: &PRInput{
				Repo:       "invalid-repo-format",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      "Title",
			},
			wantErr:     true,
			errContains: "invalid repository format",
		},
		{
			name: "empty base branch",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "",
				HeadBranch: "feature",
				Title:      "Title",
			},
			wantErr:     true,
			errContains: "base branch is required",
		},
		{
			name: "empty head branch",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main",
				HeadBranch: "",
				Title:      "Title",
			},
			wantErr:     true,
			errContains: "head branch is required",
		},
		{
			name: "empty title",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      "",
			},
			wantErr:     true,
			errContains: "title is required",
		},
		{
			name: "title too long",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      string(make([]byte, 300)), // 300 chars, exceeds 256 limit
			},
			wantErr:     true,
			errContains: "title exceeds",
		},
		{
			name: "body too long",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      "Title",
				Body:       string(make([]byte, 70000)), // Exceeds 65536 limit
			},
			wantErr:     true,
			errContains: "body exceeds",
		},
		{
			name: "too many labels",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      "Title",
				Labels:     make([]string, 101), // Exceeds 100 limit
			},
			wantErr:     true,
			errContains: "too many labels",
		},
		{
			name: "invalid label name",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main",
				HeadBranch: "feature",
				Title:      "Title",
				Labels:     []string{"valid-label", "invalid,label"},
			},
			wantErr:     true,
			errContains: "invalid label name",
		},
		{
			name: "invalid branch name with double dots",
			input: &PRInput{
				Repo:       "octocat/Hello-World",
				BaseBranch: "main..branch",
				HeadBranch: "feature",
				Title:      "Title",
			},
			wantErr:     true,
			errContains: "invalid base branch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePRInput(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePRInput() expected error, got none")
					return
				}
				if tt.errContains != "" && !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("ValidatePRInput() error = %v, want substring %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidatePRInput() unexpected error = %v", err)
			}
		})
	}
}

func TestSanitizeLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   []string
	}{
		{
			name:   "valid labels",
			labels: []string{"bug", "enhancement", "good-first-issue"},
			want:   []string{"bug", "enhancement", "good-first-issue"},
		},
		{
			name:   "remove duplicates case-insensitive",
			labels: []string{"Bug", "bug", "BUG", "enhancement"},
			want:   []string{"Bug", "enhancement"},
		},
		{
			name:   "trim whitespace",
			labels: []string{"  bug  ", " enhancement ", "good-first-issue"},
			want:   []string{"bug", "enhancement", "good-first-issue"},
		},
		{
			name:   "remove empty labels",
			labels: []string{"bug", "", "enhancement", "   ", "good-first-issue"},
			want:   []string{"bug", "enhancement", "good-first-issue"},
		},
		{
			name:   "sanitize invalid characters",
			labels: []string{"bug,fix", "enhancement;test", "good:first:issue", "\"quoted\""},
			want:   []string{"bugfix", "enhancementtest", "good:first:issue", "quoted"},
		},
		{
			name:   "truncate long labels",
			labels: []string{"this-is-a-very-long-label-name-that-exceeds-the-fifty-character-limit-for-github-labels"},
			want:   []string{"this-is-a-very-long-label-name-that-exceeds-the-fi"},
		},
		{
			name:   "remove labels that become empty after sanitization",
			labels: []string{"bug", ";\"'<>&", ":", "enhancement"},
			want:   []string{"bug", ":", "enhancement"},
		},
		{
			name:   "limit to 100 labels",
			labels: generateManyLabels(150),
			want:   generateManyLabels(100),
		},
		{
			name:   "empty input",
			labels: []string{},
			want:   []string{},
		},
		{
			name:   "nil input",
			labels: nil,
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeLabels(tt.labels)
			if !equalStringSlices(got, tt.want) {
				t.Errorf("SanitizeLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsValidGitHubName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid simple name", "octocat", true},
		{"valid with hyphens", "my-org", true},
		{"valid with underscores", "my_org", true},
		{"valid with dots", "my.org", true},
		{"valid mixed", "my-org_2.0", true},
		{"empty string", "", false},
		{"starts with hyphen", "-invalid", false},
		{"ends with hyphen", "invalid-", false},
		{"starts with dot", ".invalid", false},
		{"ends with dot", "invalid.", false},
		{"contains spaces", "my org", false},
		{"contains special chars", "my@org", false},
		{"too long", string(make([]byte, 101)), false},
		{"single character", "a", true},
		{"numbers only", "123", true},
		{"starts and ends with valid chars", "a-b", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidGitHubName(tt.input)
			if got != tt.want {
				t.Errorf("isValidGitHubName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidBranchName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid simple name", "main", true},
		{"valid with slashes", "feature/new-feature", true},
		{"valid with hyphens", "feature-branch", true},
		{"empty string", "", false},
		{"starts with slash", "/invalid", false},
		{"ends with slash", "invalid/", false},
		{"contains double dots", "feature..branch", false},
		{"contains double slashes", "feature//branch", false},
		{"too long", strings.Repeat("x", 251), false},
		{"exactly 250 valid chars", strings.Repeat("a", 250), true}, // Use valid characters, not null bytes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidBranchName(tt.input)
			if got != tt.want {
				t.Errorf("isValidBranchName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidLabelName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid simple label", "bug", true},
		{"valid with hyphens", "good-first-issue", true},
		{"empty string", "", false},
		{"contains comma", "bug,fix", false},
		{"contains semicolon", "bug;fix", false},
		{"contains colon", "type:bug", true},
		{"contains quotes", `"bug"`, false},
		{"contains angle brackets", "<bug>", false},
		{"contains ampersand", "bug&fix", false},
		{"too long", string(make([]byte, 51)), false},
		{"exactly 50 chars", string(make([]byte, 50)), true},
		{"valid with numbers", "bug123", true},
		{"valid with spaces", "bug fix", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidLabelName(tt.input)
			if got != tt.want {
				t.Errorf("isValidLabelName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPRValidationError(t *testing.T) {
	err := &PRValidationError{
		Field:   "title",
		Message: "title is required",
	}
	want := "broker: PR validation failed for field title: title is required"
	if got := err.Error(); got != want {
		t.Errorf("PRValidationError.Error() = %v, want %v", got, want)
	}
}

// Helper functions

func containsSubstring(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				func() bool {
					for i := 1; i < len(s)-len(substr)+1; i++ {
						if s[i:i+len(substr)] == substr {
							return true
						}
					}
					return false
				}()))
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func generateManyLabels(count int) []string {
	labels := make([]string, count)
	for i := 0; i < count; i++ {
		labels[i] = fmt.Sprintf("label-%d", i)
	}
	return labels
}
