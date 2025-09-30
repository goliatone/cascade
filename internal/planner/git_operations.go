package planner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/goliatone/cascade/internal/manifest"
)

// gitOperations defines interface for git operations (for testability).
type gitOperations interface {
	parseCloneURL(dependent manifest.Dependent) (string, error)
	fetchGoMod(ctx context.Context, cloneURL, ref string) (string, error)
}

// gitOperationsImpl is the real implementation of git operations.
type gitOperationsImpl struct {
	timeout time.Duration
}

// newGitOperations creates a new git operations implementation.
func newGitOperations(timeout time.Duration) gitOperations {
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	return &gitOperationsImpl{timeout: timeout}
}

// parseCloneURL converts a Dependent to a git clone URL.
// It handles GitHub/GitLab/Bitbucket formats and supports SSH and HTTPS URLs.
func (g *gitOperationsImpl) parseCloneURL(dependent manifest.Dependent) (string, error) {
	// If CloneURL is explicitly set, use it
	if dependent.CloneURL != "" {
		return dependent.CloneURL, nil
	}

	// If Repo is empty, we can't construct a URL
	if dependent.Repo == "" {
		return "", fmt.Errorf("repo field is empty")
	}

	repo := dependent.Repo

	// If it already looks like a full URL (starts with http(s):// or git@), use as-is
	if strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "git@") {
		return repo, nil
	}

	// Otherwise, assume it's in the format "github.com/user/repo" or "user/repo"
	// and convert to HTTPS URL
	parts := strings.Split(repo, "/")

	switch len(parts) {
	case 2:
		// Format: "user/repo" - assume GitHub
		return fmt.Sprintf("https://github.com/%s/%s.git", parts[0], parts[1]), nil
	case 3:
		// Format: "github.com/user/repo" or "gitlab.com/user/repo"
		host := parts[0]
		user := parts[1]
		repoName := parts[2]
		return fmt.Sprintf("https://%s/%s/%s.git", host, user, repoName), nil
	default:
		// If we can't parse it, try using it as-is with github.com prefix
		return fmt.Sprintf("https://github.com/%s.git", repo), nil
	}
}

// fetchGoMod performs a shallow clone and retrieves the go.mod file contents.
func (g *gitOperationsImpl) fetchGoMod(ctx context.Context, cloneURL, ref string) (string, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// Create temporary directory for clone
	tmpDir, err := os.MkdirTemp("", "cascade-clone-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) // Clean up on all paths

	// Perform shallow clone
	if err := g.shallowClone(ctx, cloneURL, ref, tmpDir); err != nil {
		return "", fmt.Errorf("shallow clone: %w", err)
	}

	// Read go.mod file
	goModPath := filepath.Join(tmpDir, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	return string(content), nil
}

// shallowClone performs a shallow git clone (depth=1) of the specified repository.
func (g *gitOperationsImpl) shallowClone(ctx context.Context, cloneURL, ref, destPath string) error {
	// Default to main branch if no ref specified
	if ref == "" {
		ref = "refs/heads/main"
	}

	// Ensure ref is a full reference name
	if !strings.HasPrefix(ref, "refs/") {
		// Assume it's a branch name
		ref = "refs/heads/" + ref
	}

	// Get authentication method
	auth, err := g.authMethod(cloneURL)
	if err != nil {
		return fmt.Errorf("setup auth: %w", err)
	}

	// Configure clone options for shallow clone
	opts := &git.CloneOptions{
		URL:           cloneURL,
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.ReferenceName(ref),
		Auth:          auth,
		Tags:          git.NoTags,
		Progress:      nil, // Suppress progress output
	}

	// Perform clone
	_, err = git.PlainCloneContext(ctx, destPath, false, opts)
	if err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	return nil
}

// authMethod returns the appropriate authentication method for the clone URL.
// It tries GitHub token first, then SSH keys, and falls back to no auth for public repos.
func (g *gitOperationsImpl) authMethod(cloneURL string) (transport.AuthMethod, error) {
	// Try GitHub token first (most common in CI/CD)
	if token := g.getGitHubToken(); token != "" {
		return &http.BasicAuth{
			Username: "x-access-token",
			Password: token,
		}, nil
	}

	// Try SSH key for git@ URLs
	if strings.HasPrefix(cloneURL, "git@") {
		return g.getSSHAuth()
	}

	// Public repository - no auth needed
	return nil, nil
}

// getGitHubToken retrieves GitHub token from environment variables.
// It checks multiple common environment variable names.
func (g *gitOperationsImpl) getGitHubToken() string {
	// Check common GitHub token environment variables
	for _, envVar := range []string{"GITHUB_TOKEN", "GH_TOKEN", "CASCADE_GITHUB_TOKEN"} {
		if token := os.Getenv(envVar); token != "" {
			return token
		}
	}
	return ""
}

// getSSHAuth returns SSH authentication using the default SSH key.
func (g *gitOperationsImpl) getSSHAuth() (transport.AuthMethod, error) {
	// Get SSH key path from environment or use default
	sshKeyPath := os.Getenv("SSH_KEY_PATH")
	if sshKeyPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		sshKeyPath = filepath.Join(home, ".ssh", "id_rsa")
	}

	// Check if key file exists
	if _, err := os.Stat(sshKeyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SSH key not found at %s", sshKeyPath)
	}

	// Create SSH auth method
	auth, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
	if err != nil {
		return nil, fmt.Errorf("create SSH auth: %w", err)
	}

	return auth, nil
}
