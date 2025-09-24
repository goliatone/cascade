package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// New returns a stub executor implementation.
func New() Executor {
	return &executor{}
}

type executor struct{}

func (e *executor) Apply(ctx context.Context, input WorkItemContext) (*Result, error) {
	// Validate inputs
	if err := e.validateInput(input); err != nil {
		return &Result{
			Status: StatusFailed,
			Reason: fmt.Sprintf("validation failed: %v", err),
		}, err
	}

	// Handle skip flag
	if input.Item.Skip {
		return &Result{
			Status: StatusSkipped,
			Reason: "work item marked for skip",
		}, nil
	}

	result := &Result{
		Status: StatusFailed, // Start pessimistic, update on success
	}

	// Clone/prepare repository using GitOperations
	if input.Logger != nil {
		input.Logger.Info("cloning repository", "repo", input.Item.Repo, "workspace", input.Workspace)
	}

	repoPath, err := input.Git.EnsureClone(ctx, input.Item.Repo, input.Workspace)
	if err != nil {
		e.handleExecutionError(result, err, "git clone")
		return result, err
	}

	// Create worktree for branch
	if input.Logger != nil {
		input.Logger.Info("creating worktree", "branch", input.Item.BranchName)
	}

	workPath, err := input.Git.EnsureWorktree(ctx, repoPath, input.Item.BranchName)
	if err != nil {
		e.handleExecutionError(result, err, "git worktree")
		return result, err
	}

	// Update module dependencies using GoOperations
	if input.Logger != nil {
		input.Logger.Info("updating module", "module", input.Item.SourceModule, "version", input.Item.SourceVersion)
	}

	err = input.Go.Get(ctx, workPath, input.Item.SourceModule, input.Item.SourceVersion)
	if err != nil {
		e.handleExecutionError(result, err, "dependency update")
		return result, err
	}

	// Run go mod tidy
	if input.Logger != nil {
		input.Logger.Info("running go mod tidy")
	}

	err = input.Go.Tidy(ctx, workPath)
	if err != nil {
		e.handleExecutionError(result, err, "go mod tidy")
		return result, err
	}

	// Execute tests using CommandRunner
	if input.Logger != nil {
		input.Logger.Info("executing tests", "count", len(input.Item.Tests))
	}

	testResults, testErr := e.executeCommands(ctx, input, workPath, input.Item.Tests)
	result.TestResults = testResults

	// Execute extra commands using CommandRunner
	if input.Logger != nil {
		input.Logger.Info("executing extra commands", "count", len(input.Item.ExtraCommands))
	}

	extraResults, extraErr := e.executeCommands(ctx, input, workPath, input.Item.ExtraCommands)
	result.ExtraResults = extraResults

	// Handle partial success scenarios
	if testErr != nil && extraErr != nil {
		// Both tests and extra commands failed
		e.handleExecutionError(result, testErr, "test and extra command execution")
		return result, testErr
	} else if testErr != nil {
		// Tests failed but extra commands succeeded (or there were none)
		e.handleExecutionError(result, testErr, "test execution")
		return result, testErr
	} else if extraErr != nil {
		// Tests passed but extra commands failed - this is a partial success
		result.Status = StatusManualReview
		result.Reason = fmt.Sprintf("tests passed but extra commands failed: %v", extraErr)
		// Continue with commit/push since tests passed
	}

	// Commit changes
	if input.Logger != nil {
		input.Logger.Info("committing changes", "message", input.Item.CommitMessage)
	}

	commitHash, err := input.Git.Commit(ctx, workPath, input.Item.CommitMessage)
	if err != nil {
		// Check if it's a "no changes" error - this might be expected in some cases
		if errors.Is(err, ErrNoChanges) {
			result.Status = StatusCompleted
			result.Reason = "no changes to commit"
			return result, nil
		}
		e.handleExecutionError(result, err, "git commit")
		return result, err
	}
	result.CommitHash = commitHash

	// Push changes
	if input.Logger != nil {
		input.Logger.Info("pushing changes", "branch", input.Item.BranchName)
	}

	err = input.Git.Push(ctx, workPath, input.Item.BranchName)
	if err != nil {
		e.handleExecutionError(result, err, "git push")
		return result, err
	}

	// Determine final status if not already set to manual review
	if result.Status != StatusManualReview {
		result.Status = StatusCompleted
		result.Reason = "work item executed successfully"
	}

	if input.Logger != nil {
		input.Logger.Info("work item completed", "status", result.Status, "commit", commitHash)
	}

	return result, nil
}

func (e *executor) validateInput(input WorkItemContext) error {
	if input.Item.Repo == "" {
		return fmt.Errorf("work item repo is required")
	}
	if input.Item.SourceModule == "" {
		return fmt.Errorf("work item source module is required")
	}
	if input.Item.BranchName == "" {
		return fmt.Errorf("work item branch name is required")
	}
	if input.Item.CommitMessage == "" {
		return fmt.Errorf("work item commit message is required")
	}
	if input.Workspace == "" {
		return fmt.Errorf("workspace is required")
	}
	if input.Git == nil {
		return fmt.Errorf("git operations is required")
	}
	if input.Go == nil {
		return fmt.Errorf("go operations is required")
	}
	if input.Runner == nil {
		return fmt.Errorf("command runner is required")
	}
	return nil
}

func (e *executor) executeCommands(ctx context.Context, input WorkItemContext, workPath string, commands []manifest.Command) ([]CommandResult, error) {
	var results []CommandResult

	for _, cmd := range commands {
		timeout := input.Item.Timeout
		if timeout <= 0 {
			timeout = 5 * time.Minute // default timeout
		}

		result, err := input.Runner.Run(ctx, workPath, cmd, input.Item.Env, timeout)
		results = append(results, result)

		if err != nil {
			return results, fmt.Errorf("command failed: %v", err)
		}

		// Check if command result has an error
		if result.Err != nil {
			return results, fmt.Errorf("command execution error: %v", result.Err)
		}
	}

	return results, nil
}

// handleExecutionError determines the appropriate status and reason based on the error type
func (e *executor) handleExecutionError(result *Result, err error, operation string) {
	switch {
	case IsGitError(err):
		result.Status = e.determineGitErrorStatus(err)
		result.Reason = fmt.Sprintf("%s failed: %v", operation, err)
	case IsGoError(err):
		result.Status = StatusFailed // Go errors are usually permanent
		result.Reason = fmt.Sprintf("%s failed: %v", operation, err)
	case IsCommandError(err):
		result.Status = StatusFailed // Command failures are usually test/build failures
		result.Reason = fmt.Sprintf("%s failed: %v", operation, err)
	case IsWorkspaceError(err):
		result.Status = StatusFailed // Workspace errors are usually environmental
		result.Reason = fmt.Sprintf("%s failed: %v", operation, err)
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		result.Status = StatusFailed // Timeout/cancellation - could be retriable but mark as failed
		result.Reason = fmt.Sprintf("%s timed out or was canceled: %v", operation, err)
	default:
		result.Status = StatusFailed
		result.Reason = fmt.Sprintf("%s failed: %v", operation, err)
	}
}

// determineGitErrorStatus analyzes git errors to determine if they're retriable
func (e *executor) determineGitErrorStatus(_ error) Status {
	// For now, treat all git errors as failed
	// In the future, we could distinguish between network errors (retriable)
	// and authentication/permission errors (permanent)
	return StatusFailed
}

