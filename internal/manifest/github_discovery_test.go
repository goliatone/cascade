package manifest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

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

// Integration tests with mocked GitHub responses

func TestGitHubDiscovery_IntegrationWithMockServer(t *testing.T) {
	// Create mock GitHub server
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Configure routes for GitHub API endpoints
	setupMockGitHubRoutes(mux, t)

	// Create client pointing to mock server
	client, err := createMockGitHubClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	discovery := NewGitHubDiscovery(client)
	ctx := context.Background()

	// Test DiscoverDependents with mocked responses
	t.Run("DiscoverDependents with mock responses", func(t *testing.T) {
		options := GitHubDiscoveryOptions{
			Organization: "test-org",
			TargetModule: "github.com/example/target",
			MaxResults:   10,
		}

		dependents, err := discovery.DiscoverDependents(ctx, options)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(dependents) == 0 {
			t.Error("expected at least one dependent repository")
		}

		// Verify the discovered dependent structure
		for _, dep := range dependents {
			if dep.Repository == "" {
				t.Error("expected non-empty repository name")
			}
			if dep.ModulePath == "" {
				t.Error("expected non-empty module path")
			}
		}
	})

	// Test ResolveVersion with mocked responses
	t.Run("ResolveVersion with mock responses", func(t *testing.T) {
		options := GitHubVersionResolutionOptions{
			Repository:   "test-org/test-repo",
			TargetModule: "github.com/example/target",
			Strategy:     GitHubVersionResolutionTags,
		}

		resolution, err := discovery.ResolveVersion(ctx, options)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if resolution.Version == "" {
			t.Error("expected non-empty version")
		}

		if resolution.Source != VersionSourceNetwork {
			t.Errorf("expected source %s, got %s", VersionSourceNetwork, resolution.Source)
		}
	})
}

func TestGitHubDiscovery_RateLimitHandling(t *testing.T) {
	// Create mock server that simulates rate limit scenarios
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Setup rate limit scenarios
	setupRateLimitScenarios(mux, t)

	client, err := createMockGitHubClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	discovery := NewGitHubDiscovery(client)
	ctx := context.Background()

	t.Run("rate limit critically low", func(t *testing.T) {
		err := discovery.(*gitHubDiscovery).CheckRateLimit(ctx)
		if err == nil {
			t.Error("expected rate limit error when critically low")
		}
		if !strings.Contains(err.Error(), "rate limit critically low") {
			t.Errorf("expected rate limit error message, got: %v", err)
		}
	})

	t.Run("authentication failure", func(t *testing.T) {
		err := discovery.(*gitHubDiscovery).ValidateAuthentication(ctx)
		if err == nil {
			t.Error("expected authentication error")
		}
		if !strings.Contains(err.Error(), "authentication failed") {
			t.Errorf("expected authentication error message, got: %v", err)
		}
	})
}

func TestGitHubDiscovery_ErrorHandling(t *testing.T) {
	// Create mock server that simulates various error scenarios
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	// Setup error scenarios
	setupErrorScenarios(mux, t)

	client, err := createMockGitHubClient(server.URL)
	if err != nil {
		t.Fatalf("failed to create mock client: %v", err)
	}

	discovery := NewGitHubDiscovery(client)
	ctx := context.Background()

	t.Run("repository search fails gracefully", func(t *testing.T) {
		options := GitHubDiscoveryOptions{
			Organization: "error-org",
			TargetModule: "github.com/example/target",
		}

		_, err := discovery.DiscoverDependents(ctx, options)
		if err == nil {
			t.Error("expected error for error-org organization")
		}
		if !strings.Contains(err.Error(), "search failed") {
			t.Errorf("expected search error message, got: %v", err)
		}
	})

	t.Run("version resolution with no tags", func(t *testing.T) {
		options := GitHubVersionResolutionOptions{
			Repository:   "test-org/no-tags-repo",
			TargetModule: "github.com/example/target",
			Strategy:     GitHubVersionResolutionTags,
		}

		_, err := discovery.ResolveVersion(ctx, options)
		if err == nil {
			t.Error("expected error for repository with no tags")
		}
		if !strings.Contains(err.Error(), "no tags found") {
			t.Errorf("expected no tags error message, got: %v", err)
		}
	})
}

func TestGitHubDiscovery_TokenAbsenceHandling(t *testing.T) {
	// Test creating discovery without any token
	t.Run("NewGitHubDiscoveryFromToken with no env token", func(t *testing.T) {
		// Clear environment variables temporarily
		originalVars := make(map[string]string)
		envVars := []string{"GITHUB_TOKEN", "GITHUB_ACCESS_TOKEN", "GH_TOKEN"}

		for _, envVar := range envVars {
			// Store original values
			if val := os.Getenv(envVar); val != "" {
				originalVars[envVar] = val
			}
			// Clear the variable
			os.Unsetenv(envVar)
		}

		// Restore original values after test
		defer func() {
			for envVar, val := range originalVars {
				os.Setenv(envVar, val)
			}
		}()

		_, err := NewGitHubDiscoveryFromToken("")
		if err == nil {
			t.Error("expected error when no GitHub token is available")
		}
		if !strings.Contains(err.Error(), "GitHub token not found") {
			t.Errorf("expected token not found error, got: %v", err)
		}
	})

	t.Run("NewGitHubDiscoveryFromConfig with empty token", func(t *testing.T) {
		config := GitHubAuthConfig{
			Token: "",
		}

		_, err := NewGitHubDiscoveryFromConfig(config)
		if err == nil {
			t.Error("expected error when GitHub token is empty")
		}
		if !strings.Contains(err.Error(), "GitHub token is required") {
			t.Errorf("expected token required error, got: %v", err)
		}
	})
}

// Helper functions for setting up mock server responses

func createMockGitHubClient(baseURL string) (*github.Client, error) {
	client := github.NewClient(nil)

	// Parse the base URL and add trailing slash if missing
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	url, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	client.BaseURL = url
	return client, nil
}

func setupMockGitHubRoutes(mux *http.ServeMux, t *testing.T) {
	// Mock rate limit endpoint - return healthy limits
	mux.HandleFunc("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
		resetTime := time.Now().Add(time.Hour)

		// Create the response structure that matches the GitHub API response
		response := struct {
			Resources struct {
				Core *github.Rate `json:"core"`
			} `json:"resources"`
		}{
			Resources: struct {
				Core *github.Rate `json:"core"`
			}{
				Core: &github.Rate{
					Limit:     5000,
					Remaining: 4500, // Healthy limits
					Reset:     github.Timestamp{Time: resetTime},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock repository search endpoint
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")

		var response github.RepositoriesSearchResult

		if strings.Contains(query, "test-org") {
			response = github.RepositoriesSearchResult{
				Total: github.Int(2),
				Repositories: []*github.Repository{
					{
						Name:     github.String("repo1"),
						FullName: github.String("test-org/repo1"),
						Owner: &github.User{
							Login: github.String("test-org"),
						},
						DefaultBranch: github.String("main"),
						Language:      github.String("Go"),
						Private:       github.Bool(false),
					},
					{
						Name:     github.String("repo2"),
						FullName: github.String("test-org/repo2"),
						Owner: &github.User{
							Login: github.String("test-org"),
						},
						DefaultBranch: github.String("master"),
						Language:      github.String("Go"),
						Private:       github.Bool(true),
					},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock code search endpoint for go.mod files
	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")

		var response github.CodeSearchResult

		if strings.Contains(query, "filename:go.mod") {
			response = github.CodeSearchResult{
				Total: github.Int(1),
				CodeResults: []*github.CodeResult{
					{
						Name: github.String("go.mod"),
						Path: github.String("go.mod"),
						Repository: &github.Repository{
							Name:     github.String("repo1"),
							FullName: github.String("test-org/repo1"),
							Owner: &github.User{
								Login: github.String("test-org"),
							},
						},
					},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock repository content endpoint for go.mod files
	mux.HandleFunc("/repos/test-org/repo1/contents/go.mod", func(w http.ResponseWriter, r *http.Request) {
		content := `module github.com/test-org/repo1

go 1.21

require (
	github.com/example/target v1.2.3
	github.com/other/dependency v2.0.0
)`

		response := github.RepositoryContent{
			Name:    github.String("go.mod"),
			Path:    github.String("go.mod"),
			Content: github.String(content),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock tags endpoint
	mux.HandleFunc("/repos/test-org/test-repo/tags", func(w http.ResponseWriter, r *http.Request) {
		response := []*github.RepositoryTag{
			{
				Name: github.String("v2.1.0"),
				Commit: &github.Commit{
					SHA: github.String("abc123"),
				},
			},
			{
				Name: github.String("v2.0.0"),
				Commit: &github.Commit{
					SHA: github.String("def456"),
				},
			},
			{
				Name: github.String("v1.0.0"),
				Commit: &github.Commit{
					SHA: github.String("ghi789"),
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}

func setupRateLimitScenarios(mux *http.ServeMux, t *testing.T) {
	// Mock rate limit endpoint - return critically low limits
	mux.HandleFunc("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
		resetTime := time.Now().Add(time.Hour)

		// Create the response structure that matches the GitHub API response
		response := struct {
			Resources struct {
				Core *github.Rate `json:"core"`
			} `json:"resources"`
		}{
			Resources: struct {
				Core *github.Rate `json:"core"`
			}{
				Core: &github.Rate{
					Limit:     5000,
					Remaining: 50, // Critically low (1% of limit)
					Reset:     github.Timestamp{Time: resetTime},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock user endpoint for authentication - return 401
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	})
}

func setupErrorScenarios(mux *http.ServeMux, t *testing.T) {
	// Mock rate limit endpoint - return healthy limits for error scenario tests
	mux.HandleFunc("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
		resetTime := time.Now().Add(time.Hour)

		// Create the response structure that matches the GitHub API response
		response := struct {
			Resources struct {
				Core *github.Rate `json:"core"`
			} `json:"resources"`
		}{
			Resources: struct {
				Core *github.Rate `json:"core"`
			}{
				Core: &github.Rate{
					Limit:     5000,
					Remaining: 4500, // Healthy limits
					Reset:     github.Timestamp{Time: resetTime},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock repository search that fails for specific org
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")

		if strings.Contains(query, "error-org") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message": "Repository search failed"}`))
			return
		}

		// Return empty result for other queries
		response := github.RepositoriesSearchResult{
			Total:        github.Int(0),
			Repositories: []*github.Repository{},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Mock tags endpoint that returns empty for no-tags-repo
	mux.HandleFunc("/repos/test-org/no-tags-repo/tags", func(w http.ResponseWriter, r *http.Request) {
		response := []*github.RepositoryTag{}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}
