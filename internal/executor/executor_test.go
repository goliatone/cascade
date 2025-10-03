package executor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

func TestExecutor_ApplyProducesExpectedResult(t *testing.T) {
	// Test the basic execution flow with mocked dependencies
	ctx := context.Background()

	// Create mock implementations
	mockGit := &mockGitOperations{
		clonePath:  "/workspace/test-repo",
		workPath:   "/workspace/test-repo/worktree-branch",
		commitHash: "abc123",
	}
	mockGo := &mockGoOperations{}
	mockRunner := &mockCommandRunner{}
	mockLogger := &mockLogger{}

	// Create a basic work item
	workItem := planner.WorkItem{
		Repo:          "https://github.com/test/repo",
		SourceModule:  "github.com/goliatone/go-errors",
		SourceVersion: "v1.2.3",
		BranchName:    "update-go-errors-v1.2.3",
		CommitMessage: "Update go-errors to v1.2.3",
		Tests:         []manifest.Command{},
		ExtraCommands: []manifest.Command{},
		Skip:          false,
	}

	input := executor.WorkItemContext{
		Item:      workItem,
		Workspace: "/workspace",
		Git:       mockGit,
		Go:        mockGo,
		Runner:    mockRunner,
		Logger:    mockLogger,
	}

	exec := executor.New()
	result, err := exec.Apply(ctx, input)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Verify result matches expected structure
	if result.Status != executor.StatusCompleted {
		t.Errorf("expected status %s, got %s", executor.StatusCompleted, result.Status)
	}

	if result.CommitHash != "abc123" {
		t.Errorf("expected commit hash abc123, got %s", result.CommitHash)
	}
}

func TestExecutor_Apply_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		workItem       planner.WorkItem
		gitShouldFail  bool
		goShouldFail   bool
		cmdShouldFail  bool
		expectedStatus executor.Status
		expectedReason string
	}{
		{
			name: "successful execution",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{},
				ExtraCommands: []manifest.Command{},
			},
			expectedStatus: executor.StatusCompleted,
			expectedReason: "work item executed successfully",
		},
		{
			name: "skip flag enabled",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Skip:          true,
			},
			expectedStatus: executor.StatusSkipped,
			expectedReason: "work item marked for skip",
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
			gitShouldFail:  true,
			expectedStatus: executor.StatusFailed,
			expectedReason: "git clone failed: mock clone error",
		},
		{
			name: "go operations failure",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
			},
			goShouldFail:   true,
			expectedStatus: executor.StatusFailed,
			expectedReason: "dependency update failed: mock go get error",
		},
		{
			name: "test command failure",
			workItem: planner.WorkItem{
				Repo:          "https://github.com/test/repo",
				SourceModule:  "github.com/goliatone/go-errors",
				SourceVersion: "v1.2.3",
				BranchName:    "update-branch",
				CommitMessage: "Update dependency",
				Tests:         []manifest.Command{{Cmd: []string{"go", "test", "./..."}}},
			},
			cmdShouldFail:  true,
			expectedStatus: executor.StatusFailed,
			expectedReason: "test execution failed: command failed: mock command runner error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create mock implementations
			mockGit := &mockGitOperations{
				clonePath:  "/workspace/test-repo",
				workPath:   "/workspace/test-repo/worktree-branch",
				commitHash: "abc123",
				shouldFail: tt.gitShouldFail,
			}
			mockGo := &mockGoOperations{
				shouldFail: tt.goShouldFail,
			}
			mockRunner := &mockCommandRunner{
				shouldFail: tt.cmdShouldFail,
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

			if tt.expectedStatus == executor.StatusCompleted || tt.expectedStatus == executor.StatusSkipped {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Error("expected error but got none")
				}
			}

			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}

			if result.Reason != tt.expectedReason {
				t.Errorf("expected reason %q, got %q", tt.expectedReason, result.Reason)
			}
		})
	}
}

func TestExecutor_Apply_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		input  executor.WorkItemContext
		errMsg string
	}{
		{
			name: "missing repo",
			input: executor.WorkItemContext{
				Item: planner.WorkItem{
					SourceModule:  "github.com/test/module",
					BranchName:    "branch",
					CommitMessage: "message",
				},
				Workspace: "/workspace",
				Git:       &mockGitOperations{},
				Go:        &mockGoOperations{},
				Runner:    &mockCommandRunner{},
			},
			errMsg: "work item repo is required",
		},
		{
			name: "missing source module",
			input: executor.WorkItemContext{
				Item: planner.WorkItem{
					Repo:          "https://github.com/test/repo",
					BranchName:    "branch",
					CommitMessage: "message",
				},
				Workspace: "/workspace",
				Git:       &mockGitOperations{},
				Go:        &mockGoOperations{},
				Runner:    &mockCommandRunner{},
			},
			errMsg: "work item source module is required",
		},
		{
			name: "missing workspace",
			input: executor.WorkItemContext{
				Item: planner.WorkItem{
					Repo:          "https://github.com/test/repo",
					SourceModule:  "github.com/test/module",
					BranchName:    "branch",
					CommitMessage: "message",
				},
				Git:    &mockGitOperations{},
				Go:     &mockGoOperations{},
				Runner: &mockCommandRunner{},
			},
			errMsg: "workspace is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := executor.New()
			result, err := exec.Apply(context.Background(), tt.input)

			if err == nil {
				t.Fatal("expected validation error but got none")
			}

			if result.Status != executor.StatusFailed {
				t.Errorf("expected status %s, got %s", executor.StatusFailed, result.Status)
			}

			expectedReason := fmt.Sprintf("validation failed: %s", tt.errMsg)
			if result.Reason != expectedReason {
				t.Errorf("expected reason %q, got %q", expectedReason, result.Reason)
			}
		})
	}
}

func TestExecutor_Apply_UsesProvidedCommandsAndEnv(t *testing.T) {
	ctx := context.Background()
	workspace := "/workspace"
	workPath := workspace + "/test-repo/worktree"

	mockGit := &mockGitOperations{
		clonePath:  workspace + "/test-repo",
		workPath:   workPath,
		commitHash: "abc123",
	}
	mockGo := &mockGoOperations{}
	recordingRunner := &recordingCommandRunner{}
	logger := &mockLogger{}

	workItem := planner.WorkItem{
		Repo:          "https://github.com/test/repo",
		SourceModule:  "github.com/goliatone/go-errors",
		SourceVersion: "v1.2.3",
		Branch:        "main",
		BranchName:    "update-branch",
		CommitMessage: "update dependency",
		Tests: []manifest.Command{
			{Cmd: []string{"task", "test"}, Dir: "tests"},
			{Cmd: []string{"go", "test", "./..."}},
		},
		ExtraCommands: []manifest.Command{
			{Cmd: []string{"task", "lint"}},
		},
		Env: map[string]string{
			"FOO": "bar",
			"BAR": "baz",
		},
		Timeout: time.Minute,
	}

	input := executor.WorkItemContext{
		Item:      workItem,
		Workspace: workspace,
		Git:       mockGit,
		Go:        mockGo,
		Runner:    recordingRunner,
		Logger:    logger,
	}

	result, err := executor.New().Apply(ctx, input)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Status != executor.StatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}

	expectedCommands := append([]manifest.Command{}, workItem.Tests...)
	expectedCommands = append(expectedCommands, workItem.ExtraCommands...)

	if len(recordingRunner.calls) != len(expectedCommands) {
		t.Fatalf("expected %d command calls, got %d", len(expectedCommands), len(recordingRunner.calls))
	}

	for i, call := range recordingRunner.calls {
		expected := expectedCommands[i]
		if len(call.cmd.Cmd) != len(expected.Cmd) {
			t.Fatalf("call %d command mismatch: got %#v, want %#v", i, call.cmd.Cmd, expected.Cmd)
		}
		for j, arg := range expected.Cmd {
			if call.cmd.Cmd[j] != arg {
				t.Fatalf("call %d command arg mismatch at %d: got %s, want %s", i, j, call.cmd.Cmd[j], arg)
			}
		}
		if call.cmd.Dir != expected.Dir {
			t.Fatalf("call %d dir mismatch: got %s, want %s", i, call.cmd.Dir, expected.Dir)
		}
		if call.timeout != workItem.Timeout {
			t.Fatalf("call %d timeout mismatch: got %s, want %s", i, call.timeout, workItem.Timeout)
		}
		if len(call.env) != len(workItem.Env) {
			t.Fatalf("call %d env length mismatch", i)
		}
		for k, v := range workItem.Env {
			if call.env[k] != v {
				t.Fatalf("call %d env mismatch for %s: got %s, want %s", i, k, call.env[k], v)
			}
		}
	}
}

// Mock implementations for testing
type mockGitOperations struct {
	clonePath  string
	workPath   string
	commitHash string
	shouldFail bool
}

func (m *mockGitOperations) EnsureClone(ctx context.Context, repo, workspace string) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("mock clone error")
	}
	return m.clonePath, nil
}

func (m *mockGitOperations) EnsureWorktree(ctx context.Context, repoPath, branch string, base string) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("mock worktree error")
	}
	return m.workPath, nil
}

func (m *mockGitOperations) Commit(ctx context.Context, repoPath, message string) (string, error) {
	if m.shouldFail {
		return "", fmt.Errorf("mock commit error")
	}
	return m.commitHash, nil
}

func (m *mockGitOperations) Push(ctx context.Context, repoPath, branch string) error {
	if m.shouldFail {
		return fmt.Errorf("mock push error")
	}
	return nil
}

type mockGoOperations struct {
	shouldFail bool
}

func (m *mockGoOperations) Get(ctx context.Context, repoPath, module, version string) error {
	if m.shouldFail {
		return fmt.Errorf("mock go get error")
	}
	return nil
}

func (m *mockGoOperations) Tidy(ctx context.Context, repoPath string) error {
	if m.shouldFail {
		return fmt.Errorf("mock go tidy error")
	}
	return nil
}

type mockCommandRunner struct {
	shouldFail bool
}

func (m *mockCommandRunner) Run(ctx context.Context, repoPath string, cmd manifest.Command, env map[string]string, timeout time.Duration) (executor.CommandResult, error) {
	result := executor.CommandResult{
		Command: cmd,
		Output:  "mock command output",
	}

	if m.shouldFail {
		result.Err = fmt.Errorf("mock command error")
		return result, fmt.Errorf("mock command runner error")
	}

	return result, nil
}

type recordingCommandRunner struct {
	calls []commandCall
}

type commandCall struct {
	cmd     manifest.Command
	env     map[string]string
	timeout time.Duration
}

func (r *recordingCommandRunner) Run(ctx context.Context, repoPath string, cmd manifest.Command, env map[string]string, timeout time.Duration) (executor.CommandResult, error) {
	envCopy := make(map[string]string, len(env))
	for k, v := range env {
		envCopy[k] = v
	}
	r.calls = append(r.calls, commandCall{cmd: cmd, env: envCopy, timeout: timeout})
	return executor.CommandResult{Command: cmd}, nil
}

type mockLogger struct{}

func (m *mockLogger) Info(msg string, args ...any)  {}
func (m *mockLogger) Error(msg string, args ...any) {}
func (m *mockLogger) Debug(msg string, args ...any) {}
