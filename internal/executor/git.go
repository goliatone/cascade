package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// gitOperations implements GitOperations interface using a command runner.
type gitOperations struct {
	runner GitCommandRunner
}

// NewGitOperations creates a new GitOperations implementation with the default command runner.
func NewGitOperations() GitOperations {
	return &gitOperations{
		runner: NewDefaultGitCommandRunner(),
	}
}

// NewGitOperationsWithRunner creates a new GitOperations implementation with a custom command runner.
func NewGitOperationsWithRunner(runner GitCommandRunner) GitOperations {
	return &gitOperations{
		runner: runner,
	}
}

// EnsureClone ensures a repository is cloned to the workspace and returns the repo path.
// If the repository already exists, it verifies it's the correct repository.
func (g *gitOperations) EnsureClone(ctx context.Context, repo, workspace string) (string, error) {
	// Extract repo name from URL for directory name
	repoName := extractRepoName(repo)
	repoPath := filepath.Join(workspace, repoName)

	cloneURL := buildCloneURL(repo)

	// Check if repository already exists
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		// Verify it's the correct repository
		output, err := g.runner.Run(ctx, repoPath, "config", "--get", "remote.origin.url")
		if err != nil {
			return "", fmt.Errorf("failed to get remote origin URL for %s: %w", repoPath, err)
		}

		output = cleanGitOutput(output)

		if normalizeGitURL(output) != normalizeGitURL(cloneURL) {
			return "", &ErrInvalidRepo{
				Path:     repoPath,
				Expected: cloneURL,
				Actual:   output,
			}
		}

		// Repository exists and is correct, return the path
		return repoPath, nil
	}

	// Create workspace directory if it doesn't exist
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace directory %s: %w", workspace, err)
	}

	// Clone the repository
	_, err := g.runner.Run(ctx, "", "clone", cloneURL, repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to clone repository %s to %s: %w", repo, repoPath, err)
	}

	return repoPath, nil
}

// EnsureWorktree ensures a worktree exists for the given branch and returns the worktree path.
// If the branch doesn't exist, it creates it from the current default branch.
func (g *gitOperations) EnsureWorktree(ctx context.Context, repoPath, branch string, base string) (string, error) {
	// First, fetch the latest changes
	_, err := g.runner.Run(ctx, repoPath, "fetch", "origin")
	if err != nil {
		return "", fmt.Errorf("failed to fetch from origin in %s: %w", repoPath, err)
	}

	// Check if branch exists locally and remotely
	branchExists := g.branchExists(ctx, repoPath, "refs/heads/"+branch)
	remoteBranchExists := g.branchExists(ctx, repoPath, "refs/remotes/origin/"+branch)

	worktreePath := filepath.Join(repoPath, ".worktrees", branch)

	// Check if worktree already exists
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); err == nil {
		// Worktree exists, verify it's on the correct branch
		currentBranch, err := g.runner.Run(ctx, worktreePath, "branch", "--show-current")
		if err != nil {
			return "", fmt.Errorf("failed to check current branch in worktree %s: %w", worktreePath, err)
		}
		currentBranch = cleanGitOutput(currentBranch)

		if currentBranch != branch {
			return "", fmt.Errorf("worktree %s is on branch %s, expected %s", worktreePath, currentBranch, branch)
		}

		return worktreePath, nil
	}

	// Create the worktree directory
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create worktree parent directory %s: %w", filepath.Dir(worktreePath), err)
	}

	// Create worktree based on branch existence
	if branchExists {
		_, err = g.runner.Run(ctx, repoPath, "worktree", "add", worktreePath, branch)
	} else if remoteBranchExists {
		_, err = g.runner.Run(ctx, repoPath, "worktree", "add", "-b", branch, worktreePath, "origin/"+branch)
	} else {
		baseRef := base
		if baseRef == "" {
			var derr error
			baseRef, derr = g.getDefaultBranch(ctx, repoPath)
			if derr != nil {
				return "", fmt.Errorf("failed to determine default branch: %w", derr)
			}
		}
		_, err = g.runner.Run(ctx, repoPath, "worktree", "add", "-b", branch, worktreePath, "origin/"+baseRef)
	}

	if err != nil {
		return "", fmt.Errorf("failed to create worktree for branch %s: %w", branch, err)
	}

	return worktreePath, nil
}

// Commit creates a commit with the given message in the repository.
// Returns the commit hash of the created commit.
func (g *gitOperations) Commit(ctx context.Context, repoPath, message string) (string, error) {
	// Add all changes
	_, err := g.runner.Run(ctx, repoPath, "add", ".")
	if err != nil {
		return "", fmt.Errorf("failed to stage changes in %s: %w", repoPath, err)
	}

	// Check if there are changes to commit
	statusOutput, err := g.runner.Run(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("failed to check git status in %s: %w", repoPath, err)
	}

	if len(strings.TrimSpace(statusOutput)) == 0 {
		return "", ErrNoChanges
	}

	// Create the commit
	_, err = g.runner.Run(ctx, repoPath, "commit", "-m", message)
	if err != nil {
		return "", fmt.Errorf("failed to create commit in %s: %w", repoPath, err)
	}

	// Get the commit hash
	hash, err := g.runner.Run(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash in %s: %w", repoPath, err)
	}
	hash = cleanGitOutput(hash)

	return hash, nil
}

// Push pushes the specified branch to the origin remote.
func (g *gitOperations) Push(ctx context.Context, repoPath, branch string) error {
	_, err := g.runner.Run(ctx, repoPath, "push", "origin", branch)
	if err != nil {
		return fmt.Errorf("failed to push branch %s from %s: %w", branch, repoPath, err)
	}

	return nil
}

// branchExists checks if a given branch reference exists.
func (g *gitOperations) branchExists(ctx context.Context, repoPath, ref string) bool {
	_, err := g.runner.Run(ctx, repoPath, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

// getDefaultBranch determines the default branch of the repository.
func (g *gitOperations) getDefaultBranch(ctx context.Context, repoPath string) (string, error) {
	output, err := g.runner.Run(ctx, repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		// Fall back to common default branch names
		for _, branch := range []string{"main", "master"} {
			if g.branchExists(ctx, repoPath, "refs/remotes/origin/"+branch) {
				return branch, nil
			}
		}
		return "", fmt.Errorf("failed to determine default branch and common names (main, master) not found: %w", err)
	}
	output = cleanGitOutput(output)

	// Extract branch name from "refs/remotes/origin/main"
	parts := strings.Split(output, "/")
	if len(parts) < 1 {
		return "", fmt.Errorf("unexpected ref format: %s", output)
	}

	return cleanGitOutput(parts[len(parts)-1]), nil
}

// extractRepoName extracts the repository name from a git URL.
func extractRepoName(repo string) string {
	// Handle various URL formats
	repo = strings.TrimSuffix(repo, ".git")

	if strings.Contains(repo, "/") {
		parts := strings.Split(repo, "/")
		return parts[len(parts)-1]
	}

	return repo
}

// buildCloneURL ensures the repo string is a valid cloneable URL.
func buildCloneURL(repo string) string {
	// If it doesn't have a protocol or git@, and is in owner/repo format, assume it's a GitHub repo.
	if !strings.HasPrefix(repo, "https://") &&
		!strings.HasPrefix(repo, "http://") &&
		!strings.HasPrefix(repo, "git@") &&
		strings.Count(repo, "/") == 1 {
		return "https://github.com/" + repo
	}
	return repo
}

// normalizeGitURL normalizes git URLs for comparison.
func normalizeGitURL(url string) string {
	url = cleanGitOutput(url)
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Convert SSH to HTTPS format for comparison
	if strings.HasPrefix(url, "git@github.com:") {
		url = strings.Replace(url, "git@github.com:", "https://github.com/", 1)
	}

	return strings.ToLower(url)
}

// cleanGitOutput trims whitespace from git command output to make comparisons robust.
func cleanGitOutput(output string) string {
	return strings.TrimSpace(output)
}
