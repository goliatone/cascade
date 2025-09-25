package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockGitCommandRunner implements GitCommandRunner for testing.
type mockGitCommandRunner struct {
	responses map[string]mockResponse
	calls     []mockCall
}

type mockResponse struct {
	output string
	err    error
}

type mockCall struct {
	dir  string
	args []string
}

func newMockGitCommandRunner() *mockGitCommandRunner {
	return &mockGitCommandRunner{
		responses: make(map[string]mockResponse),
		calls:     []mockCall{},
	}
}

func (m *mockGitCommandRunner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	m.calls = append(m.calls, mockCall{dir: dir, args: args})

	key := strings.Join(args, " ")
	if response, exists := m.responses[key]; exists {
		return response.output, response.err
	}

	// Default successful responses for common operations
	switch {
	case len(args) >= 2 && args[0] == "config" && args[1] == "--get":
		return "https://github.com/test/repo.git", nil
	case len(args) >= 1 && args[0] == "clone":
		return "", nil
	case len(args) >= 1 && args[0] == "fetch":
		return "", nil
	case len(args) >= 2 && args[0] == "show-ref":
		return "", nil // Default to branch not existing
	case len(args) >= 2 && args[0] == "branch":
		return "test-branch", nil
	case len(args) >= 1 && args[0] == "add":
		return "", nil
	case len(args) >= 2 && args[0] == "status":
		return "M  some-file.go", nil // Default to having changes
	case len(args) >= 1 && args[0] == "commit":
		return "", nil
	case len(args) >= 2 && args[0] == "rev-parse":
		return "abc123def456", nil
	case len(args) >= 1 && args[0] == "push":
		return "", nil
	case len(args) >= 1 && args[0] == "worktree":
		return "", nil
	case len(args) >= 1 && args[0] == "symbolic-ref":
		return "refs/remotes/origin/main", nil
	}

	return "", nil
}

func (m *mockGitCommandRunner) setResponse(args string, output string, err error) {
	m.responses[args] = mockResponse{output: output, err: err}
}

func TestGitOperations_EnsureClone(t *testing.T) {
	tests := []struct {
		name          string
		repo          string
		setupMock     func(*mockGitCommandRunner)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful clone new repository",
			repo: "https://github.com/test/repo.git",
			setupMock: func(m *mockGitCommandRunner) {
				// Repository doesn't exist (file system check), so clone will be called
				m.setResponse("clone https://github.com/test/repo.git", "", nil)
			},
			expectError: false,
		},
		{
			name: "clone fails with network error",
			repo: "https://github.com/test/nonexistent.git",
			setupMock: func(m *mockGitCommandRunner) {
				// Override the default clone response to simulate failure
				// This will be called by the actual implementation
			},
			expectError:   true,
			errorContains: "failed to clone repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := newMockGitCommandRunner()
			if tt.setupMock != nil {
				tt.setupMock(mockRunner)
			}

			git := NewGitOperationsWithRunner(mockRunner)
			ctx := context.Background()

			// Create temporary workspace
			tempDir, err := os.MkdirTemp("", "git-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tempDir)

			// Set up mock responses based on the actual commands that will be called
			repoName := extractRepoName(tt.repo)
			actualCloneCmd := "clone " + tt.repo + " " + filepath.Join(tempDir, repoName)

			if tt.expectError {
				// Set up error response for the clone command
				mockRunner.setResponse(actualCloneCmd, "", &GitError{
					Operation: "clone",
					Args:      []string{"clone", tt.repo, filepath.Join(tempDir, repoName)},
					Dir:       "",
					Err:       errors.New("repository not found"),
				})
			} else {
				mockRunner.setResponse(actualCloneCmd, "", nil)
			}

			repoPath, err := git.EnsureClone(ctx, tt.repo, tempDir)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			expectedPath := filepath.Join(tempDir, extractRepoName(tt.repo))
			if repoPath != expectedPath {
				t.Errorf("expected repo path %s, got %s", expectedPath, repoPath)
			}
		})
	}
}

func TestGitOperations_Commit(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		setupMock    func(*mockGitCommandRunner)
		expectError  bool
		expectedHash string
		errorType    error
	}{
		{
			name:    "successful commit",
			message: "test commit message",
			setupMock: func(m *mockGitCommandRunner) {
				m.setResponse("add .", "", nil)
				m.setResponse("status --porcelain", "M  some-file.go", nil)
				m.setResponse("commit -m test commit message", "", nil)
				m.setResponse("rev-parse HEAD", "abc123def456", nil)
			},
			expectError:  false,
			expectedHash: "abc123def456",
		},
		{
			name:    "no changes to commit",
			message: "test commit message",
			setupMock: func(m *mockGitCommandRunner) {
				m.setResponse("add .", "", nil)
				m.setResponse("status --porcelain", "", nil) // No changes
			},
			expectError: true,
			errorType:   ErrNoChanges,
		},
		{
			name:    "commit fails",
			message: "test commit message",
			setupMock: func(m *mockGitCommandRunner) {
				m.setResponse("add .", "", nil)
				m.setResponse("status --porcelain", "M  some-file.go", nil)
				m.setResponse("commit -m test commit message", "", errors.New("commit failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := newMockGitCommandRunner()
			if tt.setupMock != nil {
				tt.setupMock(mockRunner)
			}

			git := NewGitOperationsWithRunner(mockRunner)
			ctx := context.Background()

			hash, err := git.Commit(ctx, "/tmp/repo", tt.message)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorType != nil && !errors.Is(err, tt.errorType) {
					t.Errorf("expected error type %v, got: %v", tt.errorType, err)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if hash != tt.expectedHash {
				t.Errorf("expected hash %s, got %s", tt.expectedHash, hash)
			}
		})
	}
}

func TestGitOperations_EnsureClone_AllowsTrailingNewline(t *testing.T) {
	mockRunner := newMockGitCommandRunner()
	mockRunner.setResponse("config --get remote.origin.url", "https://github.com/test/repo.git\n", nil)

	git := NewGitOperationsWithRunner(mockRunner)
	ctx := context.Background()

	workspace, err := os.MkdirTemp("", "git-clone-*")
	if err != nil {
		t.Fatalf("failed to create temp workspace: %v", err)
	}
	defer os.RemoveAll(workspace)

	repoPath := filepath.Join(workspace, "repo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to set up repo directory: %v", err)
	}

	path, err := git.EnsureClone(ctx, "https://github.com/test/repo.git", workspace)
	if err != nil {
		t.Fatalf("EnsureClone returned error: %v", err)
	}

	if path != repoPath {
		t.Fatalf("expected repo path %s, got %s", repoPath, path)
	}
}

func TestGitOperations_EnsureWorktree_AllowsTrailingBranchNewline(t *testing.T) {
	const branch = "feature"

	mockRunner := newMockGitCommandRunner()
	mockRunner.setResponse("branch --show-current", branch+"\n", nil)

	git := NewGitOperationsWithRunner(mockRunner)
	ctx := context.Background()

	repoPath, err := os.MkdirTemp("", "git-worktree-*")
	if err != nil {
		t.Fatalf("failed to create repo path: %v", err)
	}
	defer os.RemoveAll(repoPath)

	worktreePath := filepath.Join(repoPath, ".worktrees", branch)
	if err := os.MkdirAll(filepath.Join(worktreePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to set up worktree directory: %v", err)
	}

	path, err := git.EnsureWorktree(ctx, repoPath, branch, "main")
	if err != nil {
		t.Fatalf("EnsureWorktree returned error: %v", err)
	}

	if path != worktreePath {
		t.Fatalf("expected worktree path %s, got %s", worktreePath, path)
	}
}

func TestGitOperations_getDefaultBranch_TrimsOutput(t *testing.T) {
	mockRunner := newMockGitCommandRunner()
	mockRunner.setResponse("symbolic-ref refs/remotes/origin/HEAD", "refs/remotes/origin/main\n", nil)

	gitIface := NewGitOperationsWithRunner(mockRunner)
	gitImpl, ok := gitIface.(*gitOperations)
	if !ok {
		t.Fatalf("expected *gitOperations implementation")
	}

	branch, err := gitImpl.getDefaultBranch(context.Background(), "/tmp/repo")
	if err != nil {
		t.Fatalf("getDefaultBranch returned error: %v", err)
	}

	if branch != "main" {
		t.Fatalf("expected branch 'main', got %q", branch)
	}
}

func TestGitOperations_Push(t *testing.T) {
	tests := []struct {
		name        string
		branch      string
		setupMock   func(*mockGitCommandRunner)
		expectError bool
	}{
		{
			name:   "successful push",
			branch: "feature-branch",
			setupMock: func(m *mockGitCommandRunner) {
				m.setResponse("push origin feature-branch", "", nil)
			},
			expectError: false,
		},
		{
			name:   "push fails",
			branch: "feature-branch",
			setupMock: func(m *mockGitCommandRunner) {
				m.setResponse("push origin feature-branch", "", errors.New("push failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := newMockGitCommandRunner()
			if tt.setupMock != nil {
				tt.setupMock(mockRunner)
			}

			git := NewGitOperationsWithRunner(mockRunner)
			ctx := context.Background()

			err := git.Push(ctx, "/tmp/repo", tt.branch)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGitOperations_ExtractRepoName(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		expected string
	}{
		{
			name:     "HTTPS URL with .git",
			repo:     "https://github.com/user/repo.git",
			expected: "repo",
		},
		{
			name:     "HTTPS URL without .git",
			repo:     "https://github.com/user/repo",
			expected: "repo",
		},
		{
			name:     "SSH URL",
			repo:     "git@github.com:user/repo.git",
			expected: "repo",
		},
		{
			name:     "Simple repo name",
			repo:     "repo",
			expected: "repo",
		},
		{
			name:     "Path with multiple slashes",
			repo:     "https://github.com/org/user/repo.git",
			expected: "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRepoName(tt.repo)
			if result != tt.expected {
				t.Errorf("extractRepoName(%s) = %s, want %s", tt.repo, result, tt.expected)
			}
		})
	}
}

func TestGitOperations_NormalizeGitURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "HTTPS URL with .git",
			url:      "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo",
		},
		{
			name:     "SSH URL",
			url:      "git@github.com:user/repo.git",
			expected: "https://github.com/user/repo",
		},
		{
			name:     "HTTPS URL without .git",
			url:      "https://github.com/user/repo",
			expected: "https://github.com/user/repo",
		},
		{
			name:     "Mixed case URL",
			url:      "https://GitHub.com/User/Repo.git",
			expected: "https://github.com/user/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeGitURL(tt.url)
			if result != tt.expected {
				t.Errorf("normalizeGitURL(%s) = %s, want %s", tt.url, result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkExtractRepoName(b *testing.B) {
	repo := "https://github.com/user/very-long-repository-name-for-testing.git"
	for i := 0; i < b.N; i++ {
		extractRepoName(repo)
	}
}

func BenchmarkNormalizeGitURL(b *testing.B) {
	url := "git@github.com:user/repository-name.git"
	for i := 0; i < b.N; i++ {
		normalizeGitURL(url)
	}
}
