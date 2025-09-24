//go:build integration

package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGitOperations_IntegrationWithRealRepo(t *testing.T) {
	// Skip if integration tests are disabled
	if os.Getenv("SKIP_INTEGRATION_TESTS") != "" {
		t.Skip("Integration tests disabled via SKIP_INTEGRATION_TESTS")
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "git-integration-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	git := NewGitOperations()
	ctx := context.Background()

	// Test with a real public repository (this will actually clone)
	// Using a small, stable repository for testing
	testRepo := "https://github.com/octocat/Hello-World.git"

	t.Run("real repository clone", func(t *testing.T) {
		repoPath, err := git.EnsureClone(ctx, testRepo, tempDir)
		if err != nil {
			t.Skipf("Could not clone test repository (network/auth issue): %v", err)
		}

		// Verify the repository was cloned
		if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
			t.Error("Expected .git directory to exist after clone")
		}

		// Test that calling EnsureClone again returns the same path
		repoPath2, err := git.EnsureClone(ctx, testRepo, tempDir)
		if err != nil {
			t.Errorf("Second EnsureClone call failed: %v", err)
		}

		if repoPath != repoPath2 {
			t.Errorf("Expected same repo path, got %s and %s", repoPath, repoPath2)
		}

		t.Run("worktree operations", func(t *testing.T) {
			// Test worktree creation
			worktreePath, err := git.EnsureWorktree(ctx, repoPath, "test-branch", "")
			if err != nil {
				t.Errorf("Failed to create worktree: %v", err)
			}

			// Verify worktree exists
			if _, err := os.Stat(filepath.Join(worktreePath, ".git")); os.IsNotExist(err) {
				t.Error("Expected .git file to exist in worktree")
			}
		})
	})
}
