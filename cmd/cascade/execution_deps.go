package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/goliatone/cascade/internal/broker"
	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/di"
)

// executionDeps bundles executor dependencies shared across work items.
type executionDeps struct {
	git       execpkg.GitOperations
	gitRunner execpkg.GitCommandRunner
	goTool    execpkg.GoOperations
	command   execpkg.CommandRunner
}

func newExecutionDeps() executionDeps {
	gitRunner := execpkg.NewDefaultGitCommandRunner()
	return executionDeps{
		git:       execpkg.NewGitOperationsWithRunner(gitRunner),
		gitRunner: gitRunner,
		goTool:    execpkg.NewGoOperations(),
		command:   execpkg.NewCommandRunner(),
	}
}

// processWorkItem executes a single work item and coordinates broker/state integration.
func processWorkItem(ctx context.Context, deps executionDeps, workspace string, item planner.WorkItem, executor execpkg.Executor, broker broker.Broker, logger di.Logger, defaultTimeout time.Duration) (state.ItemState, error) {
	itemCopy := item
	if itemCopy.Timeout <= 0 {
		itemCopy.Timeout = defaultTimeout
	}

	workCtx := ctx
	var cancel context.CancelFunc
	if itemCopy.Timeout > 0 {
		workCtx, cancel = context.WithTimeout(ctx, itemCopy.Timeout)
		defer cancel()
	}

	result, execErr := executor.Apply(workCtx, execpkg.WorkItemContext{
		Item:      itemCopy,
		Workspace: workspace,
		Git:       deps.git,
		Go:        deps.goTool,
		Runner:    deps.command,
		Logger:    logger,
	})

	itemState := state.ItemState{
		Repo:        item.Repo,
		Branch:      item.BranchName,
		LastUpdated: time.Now(),
		Attempts:    1,
	}

	if result != nil {
		itemState.Status = result.Status
		itemState.Reason = result.Reason
		itemState.CommitHash = result.CommitHash
		logs := append([]execpkg.CommandResult{}, result.TestResults...)
		logs = append(logs, result.ExtraResults...)
		itemState.CommandLogs = logs
	} else {
		itemState.Status = execpkg.StatusFailed
		itemState.Reason = appendReason(itemState.Reason, "executor returned no result")
	}

	var errs []error
	if execErr != nil {
		errs = append(errs, execErr)
	}

	// Handle PR creation for successful or manual review statuses
	if execErr == nil && result != nil {
		switch result.Status {
		case execpkg.StatusCompleted, execpkg.StatusManualReview:
			pr, prErr := broker.EnsurePR(ctx, item, result)
			if prErr != nil {
				errs = append(errs, fmt.Errorf("broker ensure PR: %w", prErr))
				itemState.Reason = appendReason(itemState.Reason, fmt.Sprintf("PR creation failed: %v", prErr))
			} else if pr != nil {
				itemState.PRURL = pr.URL
			}
		}
	}

	// Send notifications for all results (success or failure)
	// The notifier will handle on_success/on_failure flags from manifest
	if result != nil {
		if _, notifyErr := broker.Notify(ctx, item, result); notifyErr != nil {
			errs = append(errs, fmt.Errorf("broker notify: %w", notifyErr))
			itemState.Reason = appendReason(itemState.Reason, fmt.Sprintf("notification failed: %v", notifyErr))
		}
	}

	return itemState, errors.Join(errs...)
}
