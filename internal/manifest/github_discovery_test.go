package manifest

import (
	"context"
	"testing"

	"github.com/google/go-github/v66/github"
)

func TestNewGitHubDiscovery(t *testing.T) {
	client := github.NewClient(nil)
	discovery := NewGitHubDiscovery(client)

	if discovery == nil {
		t.Fatal("expected non-nil discovery instance")
	}

	// Verify that it implements the interface
	var _ GitHubDiscovery = discovery
}

func TestNewGitHubDiscoveryFromToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectError bool
	}{
		{
			name:        "empty token should try to load from env",
			token:       "",
			expectError: false, // Will succeed if GITHUB_TOKEN is set in environment
		},
		{
			name:        "invalid token should create client but fail validation",
			token:       "invalid-token",
			expectError: false, // Client creation succeeds, validation would fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			discovery, err := NewGitHubDiscoveryFromToken(tt.token)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if discovery == nil {
				t.Fatal("expected non-nil discovery instance")
			}
		})
	}
}

func TestGitHubDiscovery_DiscoverDependents(t *testing.T) {
	client := github.NewClient(nil)
	discovery := NewGitHubDiscovery(client)

	ctx := context.Background()

	tests := []struct {
		name        string
		options     GitHubDiscoveryOptions
		expectError bool
	}{
		{
			name: "missing organization",
			options: GitHubDiscoveryOptions{
				TargetModule: "github.com/example/module",
			},
			expectError: true,
		},
		{
			name: "missing target module",
			options: GitHubDiscoveryOptions{
				Organization: "test-org",
			},
			expectError: true,
		},
		{
			name: "valid options",
			options: GitHubDiscoveryOptions{
				Organization: "test-org",
				TargetModule: "github.com/example/module",
			},
			expectError: false, // Should succeed but return empty results for non-existent org
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := discovery.DiscoverDependents(ctx, tt.options)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none, result: %+v", result)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGitHubDiscovery_ResolveVersion(t *testing.T) {
	client := github.NewClient(nil)
	discovery := NewGitHubDiscovery(client)

	ctx := context.Background()

	tests := []struct {
		name        string
		options     GitHubVersionResolutionOptions
		expectError bool
	}{
		{
			name: "missing repository",
			options: GitHubVersionResolutionOptions{
				TargetModule: "github.com/example/module",
				Strategy:     GitHubVersionResolutionTags,
			},
			expectError: true,
		},
		{
			name: "missing target module",
			options: GitHubVersionResolutionOptions{
				Repository: "owner/repo",
				Strategy:   GitHubVersionResolutionTags,
			},
			expectError: true,
		},
		{
			name: "invalid strategy",
			options: GitHubVersionResolutionOptions{
				Repository:   "owner/repo",
				TargetModule: "github.com/example/module",
				Strategy:     "invalid",
			},
			expectError: true,
		},
		{
			name: "valid options with tags strategy",
			options: GitHubVersionResolutionOptions{
				Repository:   "owner/repo",
				TargetModule: "github.com/example/module",
				Strategy:     GitHubVersionResolutionTags,
			},
			expectError: true, // Will fail without authentication in test
		},
		{
			name: "valid options with proxy strategy",
			options: GitHubVersionResolutionOptions{
				Repository:   "owner/repo",
				TargetModule: "github.com/example/module",
				Strategy:     GitHubVersionResolutionProxy,
				UseProxy:     true,
			},
			expectError: true, // Will fail without authentication in test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := discovery.ResolveVersion(ctx, tt.options)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildSearchQuery(t *testing.T) {
	discovery := &gitHubDiscovery{}

	tests := []struct {
		name     string
		options  GitHubDiscoveryOptions
		expected string
	}{
		{
			name: "default query",
			options: GitHubDiscoveryOptions{
				Organization: "test-org",
			},
			expected: "language:go filename:go.mod org:test-org",
		},
		{
			name: "custom search query",
			options: GitHubDiscoveryOptions{
				Organization: "test-org",
				SearchQuery:  "custom query",
			},
			expected: "custom query org:test-org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := discovery.buildSearchQuery(tt.options)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestShouldIncludeRepository(t *testing.T) {
	discovery := &gitHubDiscovery{}

	repo := &github.Repository{
		Name: github.String("test-repo"),
	}

	tests := []struct {
		name     string
		options  GitHubDiscoveryOptions
		expected bool
	}{
		{
			name:     "no patterns - include by default",
			options:  GitHubDiscoveryOptions{},
			expected: true,
		},
		{
			name: "exclude pattern matches",
			options: GitHubDiscoveryOptions{
				ExcludePatterns: []string{"test-*"},
			},
			expected: false,
		},
		{
			name: "include pattern matches",
			options: GitHubDiscoveryOptions{
				IncludePatterns: []string{"test-*"},
			},
			expected: true,
		},
		{
			name: "include pattern doesn't match",
			options: GitHubDiscoveryOptions{
				IncludePatterns: []string{"other-*"},
			},
			expected: false,
		},
		{
			name: "exclude takes precedence over include",
			options: GitHubDiscoveryOptions{
				IncludePatterns: []string{"*"},
				ExcludePatterns: []string{"test-*"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := discovery.shouldIncludeRepository(repo, tt.options)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	discovery := &gitHubDiscovery{}

	tests := []struct {
		name     string
		pattern  string
		text     string
		expected bool
	}{
		{
			name:     "exact match",
			pattern:  "test",
			text:     "test",
			expected: true,
		},
		{
			name:     "no match",
			pattern:  "test",
			text:     "other",
			expected: false,
		},
		{
			name:     "wildcard match all",
			pattern:  "*",
			text:     "anything",
			expected: true,
		},
		{
			name:     "prefix wildcard",
			pattern:  "test-*",
			text:     "test-repo",
			expected: true,
		},
		{
			name:     "suffix wildcard",
			pattern:  "*-repo",
			text:     "test-repo",
			expected: true,
		},
		{
			name:     "prefix wildcard no match",
			pattern:  "test-*",
			text:     "other-repo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := discovery.matchPattern(tt.pattern, tt.text)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestInferModulePath(t *testing.T) {
	discovery := &gitHubDiscovery{}

	tests := []struct {
		name         string
		repoFullName string
		expected     string
	}{
		{
			name:         "github repository",
			repoFullName: "owner/repo",
			expected:     "github.com/owner/repo",
		},
		{
			name:         "github repository with subdirectory",
			repoFullName: "owner/complex-repo-name",
			expected:     "github.com/owner/complex-repo-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := discovery.inferModulePath(tt.repoFullName)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestInferLocalModulePath(t *testing.T) {
	discovery := &gitHubDiscovery{}

	result := discovery.inferLocalModulePath("github.com/owner/repo")
	expected := "."

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestIsNewerVersion(t *testing.T) {
	discovery := &gitHubDiscovery{}

	tests := []struct {
		name     string
		version1 string
		version2 string
		expected bool
	}{
		{
			name:     "v2.0.0 is newer than v1.0.0",
			version1: "v2.0.0",
			version2: "v1.0.0",
			expected: true,
		},
		{
			name:     "v1.0.0 is not newer than v2.0.0",
			version1: "v1.0.0",
			version2: "v2.0.0",
			expected: false,
		},
		{
			name:     "equal versions",
			version1: "v1.0.0",
			version2: "v1.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := discovery.isNewerVersion(tt.version1, tt.version2)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestGitHubDiscovery_ValidationAndRateLimit tests the validation and rate limit methods
func TestGitHubDiscovery_ValidationAndRateLimit(t *testing.T) {
	client := github.NewClient(nil)
	discovery := NewGitHubDiscovery(client)

	ctx := context.Background()

	// Test ValidateAuthentication - should fail without real token
	err := discovery.(*gitHubDiscovery).ValidateAuthentication(ctx)
	if err == nil {
		t.Error("expected authentication to fail with no token")
	}

	// Test CheckRateLimit - might succeed with anonymous access, so just verify it doesn't panic
	err = discovery.(*gitHubDiscovery).CheckRateLimit(ctx)
	// This test now just verifies the method can be called without panicking
	// Rate limit checking can work with anonymous access for GitHub API
	t.Logf("CheckRateLimit result: %v", err)
}
