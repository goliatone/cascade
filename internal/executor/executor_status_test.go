package executor_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

func TestExecutor_Apply_StatusLogic(t *testing.T) {
	tests := []struct {
		name                   string
		workItem               planner.WorkItem
		gitError               error
		goError                error
		testError              error
		extraError             error
		commitError            error
		pushError              error
		expectedStatus         executor.Status
		expectedReasonContains string
	}{
		{
			name: "complete success",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{{Cmd: []string{"go", "test", "./..."}}},
				ExtraCommands: []manifest.Command{{Cmd: []string{"go", "vet", "./..."}}},
			},
			expectedStatus:         executor.StatusCompleted,
			expectedReasonContains: "work item executed successfully",
		},
		{
			name: "partial success - tests pass, extra commands fail",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{{Cmd: []string{"go", "test", "./..."}}},
				ExtraCommands: []manifest.Command{{Cmd: []string{"go", "vet", "./..."}}},
			},
			extraError:             fmt.Errorf("vet failed"),
			expectedStatus:         executor.StatusManualReview,
			expectedReasonContains: "tests passed but extra commands failed",
		},
		{
			name: "git clone failure",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
			},
			gitError:               &executor.GitOperationError{Repo: "test", Operation: "clone", Err: fmt.Errorf("network error")},
			expectedStatus:         executor.StatusFailed,
			expectedReasonContains: "git clone failed",
		},
		{
			name: "go operation failure",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
			},
			goError:                &executor.GoOperationError{Module: "test", Version: "v1.0.0", Err: fmt.Errorf("module not found")},
			expectedStatus:         executor.StatusFailed,
			expectedReasonContains: "dependency update failed",
		},
		{
			name: "command execution failure",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{{Cmd: []string{"go", "test", "./..."}}},
			},
			testError:              &executor.CommandExecutionError{Command: []string{"go", "test"}, Dir: "/test", ExitCode: 1, Err: fmt.Errorf("test failed")},
			expectedStatus:         executor.StatusFailed,
			expectedReasonContains: "test execution failed",
		},
		{
			name: "workspace error",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
			},
			gitError:               &executor.WorkspaceError{Path: "/workspace", Operation: "create", Err: fmt.Errorf("permission denied")},
			expectedStatus:         executor.StatusFailed,
			expectedReasonContains: "git clone failed",
		},
		{
			name: "timeout scenario",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{{Cmd: []string{"go", "test", "./..."}}},
			},
			testError:              context.DeadlineExceeded,
			expectedStatus:         executor.StatusFailed,
			expectedReasonContains: "timed out or was canceled",
		},
		{
			name: "no changes to commit",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
			},
			commitError:            executor.ErrNoChanges,
			expectedStatus:         executor.StatusCompleted,
			expectedReasonContains: "no changes to commit",
		},
		{
			name: "both tests and extra commands fail",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{{Cmd: []string{"go", "test", "./..."}}},
				ExtraCommands: []manifest.Command{{Cmd: []string{"go", "vet", "./..."}}},
			},
			testError:              fmt.Errorf("tests failed"),
			extraError:             fmt.Errorf("vet failed"),
			expectedStatus:         executor.StatusFailed,
			expectedReasonContains: "test and extra command execution failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock implementations with specific error behaviors
			mockGit := &advancedMockGitOperations{
				clonePath:   "/workspace/test-repo",
				workPath:    "/workspace/test-repo/worktree-branch",
				commitHash:  "abc123",
				cloneError:  tt.gitError,
				commitError: tt.commitError,
				pushError:   tt.pushError,
			}
			mockGo := &advancedMockGoOperations{
				getError:  tt.goError,
				tidyError: tt.goError,
			}
			mockRunner := &advancedMockCommandRunner{
				testError:  tt.testError,
				extraError: tt.extraError,
			}
			mockLogger := &mockLogger{}

			input := executor.WorkItemContext{
				Item:      tt.workItem,
				Workspace: "/workspace",
				Git:       mockGit,
				Go:        mockGo,
				Runner:    mockRunner,
				Logger:    mockLogger,
			}

			exec := executor.New()
			result, err := exec.Apply(ctx, input)

			// Verify status
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}

			// Verify reason contains expected text
			if tt.expectedReasonContains != "" && !strings.Contains(result.Reason, tt.expectedReasonContains) {
				t.Errorf("expected reason to contain %q, got %q", tt.expectedReasonContains, result.Reason)
			}

			// Verify error expectations
			switch tt.expectedStatus {
			case executor.StatusCompleted:
				if err != nil && !errors.Is(err, executor.ErrNoChanges) {
					t.Errorf("unexpected error for completed status: %v", err)
				}
			case executor.StatusSkipped:
				if err != nil {
					t.Errorf("unexpected error for skipped status: %v", err)
				}
			case executor.StatusManualReview:
				// Manual review can succeed with warnings - don't expect error for successful runs
				// This is expected behavior for partial success
			case executor.StatusFailed:
				if err == nil {
					t.Error("expected error for failed status but got none")
				}
			}

			// Basic structure validation
			if result == nil {
				t.Error("Result should not be nil")
			}
		})
	}
}

// Advanced mock implementations for comprehensive testing
type advancedMockGitOperations struct {
	clonePath     string
	workPath      string
	commitHash    string
	cloneError    error
	worktreeError error
	commitError   error
	pushError     error
}

func (m *advancedMockGitOperations) EnsureClone(ctx context.Context, repo, workspace string) (string, error) {
	if m.cloneError != nil {
		return "", m.cloneError
	}
	return m.clonePath, nil
}

func (m *advancedMockGitOperations) EnsureWorktree(ctx context.Context, repoPath, branch string, base string) (string, error) {
	if m.worktreeError != nil {
		return "", m.worktreeError
	}
	return m.workPath, nil
}

func (m *advancedMockGitOperations) Commit(ctx context.Context, repoPath, message string) (string, error) {
	if m.commitError != nil {
		return "", m.commitError
	}
	return m.commitHash, nil
}

func (m *advancedMockGitOperations) Push(ctx context.Context, repoPath, branch string) error {
	return m.pushError
}

type advancedMockGoOperations struct {
	getError  error
	tidyError error
}

func (m *advancedMockGoOperations) Get(ctx context.Context, repoPath, module, version string) error {
	return m.getError
}

func (m *advancedMockGoOperations) Tidy(ctx context.Context, repoPath string) error {
	return m.tidyError
}

type advancedMockCommandRunner struct {
	testError  error
	extraError error
}

func (m *advancedMockCommandRunner) Run(ctx context.Context, repoPath string, cmd manifest.Command, env map[string]string, timeout time.Duration) (executor.CommandResult, error) {
	result := executor.CommandResult{
		Command: cmd,
		Output:  "mock command output",
	}

	// Determine if this is a test command vs extra command based on context
	// This is a simplified heuristic for testing
	isTest := len(cmd.Cmd) > 0 && (cmd.Cmd[0] == "go" && len(cmd.Cmd) > 1 && cmd.Cmd[1] == "test")

	if isTest && m.testError != nil {
		result.Err = m.testError
		return result, m.testError
	} else if !isTest && m.extraError != nil {
		result.Err = m.extraError
		return result, m.extraError
	}

	return result, nil
}
