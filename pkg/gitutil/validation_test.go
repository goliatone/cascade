package gitutil

import (
	"strings"
	"testing"
)

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid simple name",
			input:   "main",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			input:   "feature-branch",
			wantErr: false,
		},
		{
			name:    "valid with slashes",
			input:   "feature/add-something",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			input:   "feature_branch",
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   "",
			wantErr: true,
		},
		{
			name:    "starts with slash",
			input:   "/feature",
			wantErr: true,
		},
		{
			name:    "ends with slash",
			input:   "feature/",
			wantErr: true,
		},
		{
			name:    "contains double dots",
			input:   "feature..branch",
			wantErr: true,
		},
		{
			name:    "contains double slashes",
			input:   "feature//branch",
			wantErr: true,
		},
		{
			name:    "ends with .lock",
			input:   "feature.lock",
			wantErr: true,
		},
		{
			name:    "too long (over 250 chars)",
			input:   strings.Repeat("a", 251),
			wantErr: true,
		},
		{
			name:    "exactly 250 chars",
			input:   strings.Repeat("a", 250),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBranchName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRepoName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid simple name",
			input:   "myrepo",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			input:   "my-repo",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			input:   "my_repo",
			wantErr: false,
		},
		{
			name:    "valid with dots",
			input:   "my.repo",
			wantErr: false,
		},
		{
			name:    "valid mixed",
			input:   "my-repo_v2.0",
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   "",
			wantErr: true,
		},
		{
			name:    "starts with hyphen",
			input:   "-myrepo",
			wantErr: true,
		},
		{
			name:    "ends with hyphen",
			input:   "myrepo-",
			wantErr: true,
		},
		{
			name:    "starts with dot",
			input:   ".myrepo",
			wantErr: true,
		},
		{
			name:    "ends with dot",
			input:   "myrepo.",
			wantErr: true,
		},
		{
			name:    "too long (over 100 chars)",
			input:   strings.Repeat("a", 101),
			wantErr: true,
		},
		{
			name:    "exactly 100 chars",
			input:   strings.Repeat("a", 100),
			wantErr: false,
		},
		{
			name:    "contains spaces",
			input:   "my repo",
			wantErr: true,
		},
		{
			name:    "single character",
			input:   "a",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepoName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateOwnerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid simple name",
			input:   "myorg",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			input:   "my-org",
			wantErr: false,
		},
		{
			name:    "valid GitHub username",
			input:   "goliatone",
			wantErr: false,
		},
		{
			name:    "empty name",
			input:   "",
			wantErr: true,
		},
		{
			name:    "starts with hyphen",
			input:   "-myorg",
			wantErr: true,
		},
		{
			name:    "ends with hyphen",
			input:   "myorg-",
			wantErr: true,
		},
		{
			name:    "too long",
			input:   strings.Repeat("a", 101),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOwnerName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOwnerName() error = %v, wantErr %v", err, tt.wantErr)
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
		{
			name:  "valid simple name",
			input: "myrepo",
			want:  true,
		},
		{
			name:  "valid with hyphens",
			input: "my-repo",
			want:  true,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "starts with hyphen",
			input: "-myrepo",
			want:  false,
		},
		{
			name:  "ends with hyphen",
			input: "myrepo-",
			want:  false,
		},
		{
			name:  "too long",
			input: strings.Repeat("a", 101),
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidGitHubName(tt.input)
			if got != tt.want {
				t.Errorf("IsValidGitHubName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name unchanged",
			input: "feature-branch",
			want:  "feature-branch",
		},
		{
			name:  "slashes replaced with hyphens",
			input: "feature/add-something",
			want:  "feature-add-something",
		},
		{
			name:  "@ replaced with -at-",
			input: "v1.0.0@beta",
			want:  "v1-0-0-at-beta",
		},
		{
			name:  "+ replaced with -plus-",
			input: "version+build",
			want:  "version-plus-build",
		},
		{
			name:  "dots replaced with hyphens",
			input: "v1.0.0",
			want:  "v1-0-0",
		},
		{
			name:  "spaces replaced with hyphens",
			input: "feature branch name",
			want:  "feature-branch-name",
		},
		{
			name:  "consecutive hyphens collapsed",
			input: "feature--branch",
			want:  "feature-branch",
		},
		{
			name:  "leading and trailing hyphens removed",
			input: "-feature-branch-",
			want:  "feature-branch",
		},
		{
			name:  "empty string becomes 'branch'",
			input: "",
			want:  "branch",
		},
		{
			name:  "only invalid chars becomes 'branch'",
			input: "!!!",
			want:  "branch",
		},
		{
			name:  "mixed special characters",
			input: "feat/update-v1.0.0@beta+build",
			want:  "feat-update-v1-0-0-at-beta-plus-build",
		},
		{
			name:  "too long truncated to 250",
			input: strings.Repeat("a", 300),
			want:  strings.Repeat("a", 250),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeBranchName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeBranchName() = %v, want %v", got, tt.want)
			}

			// Verify result is valid
			if err := ValidateBranchName(got); err != nil {
				t.Errorf("SanitizeBranchName() produced invalid branch name: %v", err)
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
		{
			name:  "valid simple label",
			input: "bug",
			want:  true,
		},
		{
			name:  "valid with hyphens",
			input: "good-first-issue",
			want:  true,
		},
		{
			name:  "valid with colons",
			input: "type:bug",
			want:  true,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "too long (over 50 chars)",
			input: strings.Repeat("a", 51),
			want:  false,
		},
		{
			name:  "contains comma",
			input: "bug,feature",
			want:  false,
		},
		{
			name:  "contains semicolon",
			input: "bug;feature",
			want:  false,
		},
		{
			name:  "contains double quote",
			input: "bug\"feature",
			want:  false,
		},
		{
			name:  "contains angle brackets",
			input: "<bug>",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidLabelName(tt.input)
			if got != tt.want {
				t.Errorf("IsValidLabelName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeLabels(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "valid labels unchanged",
			input: []string{"bug", "feature", "good-first-issue"},
			want:  []string{"bug", "feature", "good-first-issue"},
		},
		{
			name:  "empty labels removed",
			input: []string{"bug", "", "feature"},
			want:  []string{"bug", "feature"},
		},
		{
			name:  "whitespace trimmed",
			input: []string{"  bug  ", "feature"},
			want:  []string{"bug", "feature"},
		},
		{
			name:  "duplicates removed (case-insensitive)",
			input: []string{"Bug", "bug", "BUG", "feature"},
			want:  []string{"Bug", "feature"},
		},
		{
			name:  "too long truncated to 50",
			input: []string{strings.Repeat("a", 60)},
			want:  []string{strings.Repeat("a", 50)},
		},
		{
			name:  "invalid characters removed",
			input: []string{"bug,feature", "test;label"},
			want:  []string{"bugfeature", "testlabel"},
		},
		{
			name:  "empty after sanitization removed",
			input: []string{",,,,", "bug"},
			want:  []string{"bug"},
		},
		{
			name:  "more than 100 labels truncated",
			input: make([]string, 150),
			want:  make([]string, 100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the "more than 100 labels" test, populate the slices
			if tt.name == "more than 100 labels truncated" {
				for i := range tt.input {
					// Use "label-N" format to ensure uniqueness
					tt.input[i] = "label-" + string(rune('0'+i%10)) + string(rune('a'+i/10))
				}
				for i := range tt.want {
					tt.want[i] = "label-" + string(rune('0'+i%10)) + string(rune('a'+i/10))
				}
			}

			got := SanitizeLabels(tt.input)

			if len(got) != len(tt.want) {
				t.Errorf("SanitizeLabels() length = %v, want %v", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SanitizeLabels()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}

			// Verify all results are valid
			for _, label := range got {
				if !IsValidLabelName(label) {
					t.Errorf("SanitizeLabels() produced invalid label: %v", label)
				}
			}
		})
	}
}
