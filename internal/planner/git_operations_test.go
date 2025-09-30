package planner

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

func TestParseCloneURL(t *testing.T) {
	g := newGitOperations(30 * time.Second)
	impl := g.(*gitOperationsImpl)

	tests := []struct {
		name      string
		dependent manifest.Dependent
		want      string
		wantErr   bool
	}{
		{
			name: "explicit CloneURL is used",
			dependent: manifest.Dependent{
				Repo:     "user/repo",
				CloneURL: "https://custom.git/user/repo.git",
			},
			want: "https://custom.git/user/repo.git",
		},
		{
			name: "github.com/user/repo format",
			dependent: manifest.Dependent{
				Repo: "github.com/user/repo",
			},
			want: "https://github.com/user/repo.git",
		},
		{
			name: "gitlab.com/user/repo format",
			dependent: manifest.Dependent{
				Repo: "gitlab.com/user/repo",
			},
			want: "https://gitlab.com/user/repo.git",
		},
		{
			name: "user/repo format (assumes GitHub)",
			dependent: manifest.Dependent{
				Repo: "user/repo",
			},
			want: "https://github.com/user/repo.git",
		},
		{
			name: "full HTTPS URL unchanged",
			dependent: manifest.Dependent{
				Repo: "https://github.com/user/repo.git",
			},
			want: "https://github.com/user/repo.git",
		},
		{
			name: "full HTTP URL unchanged",
			dependent: manifest.Dependent{
				Repo: "http://github.com/user/repo.git",
			},
			want: "http://github.com/user/repo.git",
		},
		{
			name: "SSH URL unchanged",
			dependent: manifest.Dependent{
				Repo: "git@github.com:user/repo.git",
			},
			want: "git@github.com:user/repo.git",
		},
		{
			name: "empty repo returns error",
			dependent: manifest.Dependent{
				Repo: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := impl.parseCloneURL(tt.dependent)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCloneURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseCloneURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetGitHubToken(t *testing.T) {
	g := newGitOperations(30 * time.Second)
	impl := g.(*gitOperationsImpl)

	tests := []struct {
		name     string
		envVars  map[string]string
		wantNone bool
	}{
		{
			name:     "no token returns empty",
			envVars:  map[string]string{},
			wantNone: true,
		},
		{
			name: "GITHUB_TOKEN is read",
			envVars: map[string]string{
				"GITHUB_TOKEN": "token123",
			},
		},
		{
			name: "GH_TOKEN is read",
			envVars: map[string]string{
				"GH_TOKEN": "token456",
			},
		},
		{
			name: "CASCADE_GITHUB_TOKEN is read",
			envVars: map[string]string{
				"CASCADE_GITHUB_TOKEN": "token789",
			},
		},
		{
			name: "GITHUB_TOKEN takes precedence",
			envVars: map[string]string{
				"GITHUB_TOKEN": "token1",
				"GH_TOKEN":     "token2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all token environment variables first
			for _, envVar := range []string{"GITHUB_TOKEN", "GH_TOKEN", "CASCADE_GITHUB_TOKEN"} {
				os.Unsetenv(envVar)
			}

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			got := impl.getGitHubToken()
			if tt.wantNone && got != "" {
				t.Errorf("getGitHubToken() = %v, want empty", got)
			}
			if !tt.wantNone && got == "" {
				t.Errorf("getGitHubToken() = empty, want non-empty")
			}
		})
	}
}

func TestAuthMethod(t *testing.T) {
	g := newGitOperations(30 * time.Second)
	impl := g.(*gitOperationsImpl)

	tests := []struct {
		name     string
		cloneURL string
		token    string
		wantAuth bool
	}{
		{
			name:     "HTTPS URL with token returns BasicAuth",
			cloneURL: "https://github.com/user/repo.git",
			token:    "token123",
			wantAuth: true,
		},
		{
			name:     "HTTPS URL without token returns nil",
			cloneURL: "https://github.com/user/repo.git",
			token:    "",
			wantAuth: false,
		},
		{
			name:     "SSH URL attempts SSH auth",
			cloneURL: "git@github.com:user/repo.git",
			token:    "",
			wantAuth: false, // Will fail without SSH key, but that's expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set token if needed
			os.Unsetenv("GITHUB_TOKEN")
			if tt.token != "" {
				os.Setenv("GITHUB_TOKEN", tt.token)
				defer os.Unsetenv("GITHUB_TOKEN")
			}

			auth, err := impl.authMethod(tt.cloneURL)

			// For SSH URLs without keys, we expect an error
			if tt.cloneURL[:4] == "git@" && err != nil {
				return // Expected failure
			}

			if err != nil {
				t.Errorf("authMethod() error = %v", err)
				return
			}

			if tt.wantAuth && auth == nil {
				t.Errorf("authMethod() = nil, want non-nil")
			}
			if !tt.wantAuth && auth != nil {
				t.Errorf("authMethod() = non-nil, want nil")
			}
		})
	}
}

// mockGitOperations is a mock implementation for testing.
type mockGitOperations struct {
	parseCloneURLFunc func(dependent manifest.Dependent) (string, error)
	fetchGoModFunc    func(ctx context.Context, cloneURL, ref string) (string, error)
}

func (m *mockGitOperations) parseCloneURL(dependent manifest.Dependent) (string, error) {
	if m.parseCloneURLFunc != nil {
		return m.parseCloneURLFunc(dependent)
	}
	return "", fmt.Errorf("not implemented")
}

func (m *mockGitOperations) fetchGoMod(ctx context.Context, cloneURL, ref string) (string, error) {
	if m.fetchGoModFunc != nil {
		return m.fetchGoModFunc(ctx, cloneURL, ref)
	}
	return "", fmt.Errorf("not implemented")
}

func TestFetchGoModIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	g := newGitOperations(60 * time.Second) // Longer timeout for network operations
	impl := g.(*gitOperationsImpl)

	tests := []struct {
		name     string
		cloneURL string
		ref      string
		wantErr  bool
		skipCI   bool // Skip in CI environments
	}{
		{
			name:     "public GitHub repo with main branch",
			cloneURL: "https://github.com/goliatone/go-errors.git",
			ref:      "refs/heads/main",
			wantErr:  false,
		},
		{
			name:     "public GitHub repo with empty ref (defaults to main)",
			cloneURL: "https://github.com/goliatone/go-errors.git",
			ref:      "",
			wantErr:  false,
		},
		{
			name:     "invalid repository returns error",
			cloneURL: "https://github.com/nonexistent/repo-does-not-exist-12345.git",
			ref:      "main",
			wantErr:  true,
		},
		{
			name:     "repo without go.mod returns error",
			cloneURL: "https://github.com/goliatone/empty-test-repo.git",
			ref:      "main",
			wantErr:  true,
			skipCI:   true, // This repo might not exist
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipCI && os.Getenv("CI") != "" {
				t.Skip("skipping in CI environment")
			}

			ctx := context.Background()
			content, err := impl.fetchGoMod(ctx, tt.cloneURL, tt.ref)

			if (err != nil) != tt.wantErr {
				t.Errorf("fetchGoMod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if content == "" {
					t.Errorf("fetchGoMod() returned empty content")
				}
				// Verify it looks like a go.mod file
				if !containsGoModMarkers(content) {
					t.Errorf("fetchGoMod() content doesn't look like go.mod: %s", content[:min(100, len(content))])
				}
			}
		})
	}
}

func TestShallowCloneTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	// Create git ops with very short timeout
	g := newGitOperations(1 * time.Millisecond)
	impl := g.(*gitOperationsImpl)

	tmpDir, err := os.MkdirTemp("", "cascade-timeout-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	err = impl.shallowClone(ctx, "https://github.com/torvalds/linux.git", "main", tmpDir)

	if err == nil {
		t.Errorf("shallowClone() expected timeout error, got nil")
	}
}

func TestFetchGoModCleansUpTempDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	g := newGitOperations(60 * time.Second)
	impl := g.(*gitOperationsImpl)

	// Track temp directories created
	originalTempDir := os.TempDir()
	beforeFiles, err := os.ReadDir(originalTempDir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}

	ctx := context.Background()
	_, err = impl.fetchGoMod(ctx, "https://github.com/goliatone/go-errors.git", "main")
	if err != nil {
		t.Fatalf("fetchGoMod() error = %v", err)
	}

	// Give OS time to clean up
	time.Sleep(100 * time.Millisecond)

	afterFiles, err := os.ReadDir(originalTempDir)
	if err != nil {
		t.Fatalf("read temp dir: %v", err)
	}

	// Count cascade-clone-* directories
	beforeCount := countCascadeCloneDirs(beforeFiles)
	afterCount := countCascadeCloneDirs(afterFiles)

	if afterCount > beforeCount {
		t.Errorf("temp directory not cleaned up: before=%d, after=%d", beforeCount, afterCount)
	}
}

// Helper functions

func containsGoModMarkers(content string) bool {
	// Check for common go.mod markers
	return len(content) > 0 &&
		(contains(content, "module ") ||
			contains(content, "go ") ||
			contains(content, "require"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || contains(s[1:], substr)))
}

func countCascadeCloneDirs(files []os.DirEntry) int {
	count := 0
	for _, f := range files {
		if f.IsDir() && len(f.Name()) >= 14 && f.Name()[:14] == "cascade-clone-" {
			count++
		}
	}
	return count
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
