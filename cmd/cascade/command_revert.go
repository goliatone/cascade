package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goliatone/cascade/internal/broker"
	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/state"
	"github.com/spf13/cobra"
)

// newRevertCommand creates the revert subcommand
func newRevertCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "revert [state-id]",
		Short: "Revert changes from a cascade operation",
		Long: `Revert undoes changes made by a cascade operation,
closing pull requests and cleaning up branches as needed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stateID := ""
			if len(args) > 0 {
				stateID = args[0]
			}
			return runRevert(stateID)
		},
	}
}

func runRevert(stateID string) error {
	start := time.Now()
	logger := container.Logger()
	cfg := container.Config()
	ctx := context.Background()

	defer func() {
		if logger != nil {
			logger.Debug("Revert command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"state_id", stateID,
				"dry_run", cfg.Executor.DryRun,
			)
		}
	}()

	module, version, err := resolveModuleVersion(stateID, cfg)
	if err != nil {
		return newValidationError(err.Error(), nil)
	}

	summary, err := container.State().LoadSummary(module, version)
	if err != nil {
		if err == state.ErrNotFound {
			return fmt.Errorf("no saved state found for %s@%s", module, version)
		}
		return newStateError("failed to load summary", err)
	}

	itemStates, err := container.State().LoadItemStates(module, version)
	if err != nil {
		return newStateError("failed to load item states", err)
	}

	if len(itemStates) == 0 {
		fmt.Printf("No state recorded for %s@%s\n", module, version)
		return nil
	}

	if cfg.Executor.DryRun {
		fmt.Printf("DRY RUN: Would revert cascade for %s@%s\n", module, version)
		for _, item := range itemStates {
			fmt.Printf("  - %s (branch: %s", item.Repo, item.Branch)
			if item.PRURL != "" {
				fmt.Printf(", PR: %s", item.PRURL)
			}
			fmt.Println(")")
		}
		return nil
	}

	if err := ensureWorkspace(cfg.Workspace.Path); err != nil {
		return newExecutionError("failed to prepare workspace", err)
	}

	deps := newExecutionDeps()
	stateManager := container.State()
	tracker := newStateTracker(module, version, summary, stateManager, logger, itemStates)
	brokerSvc := container.Broker()

	fmt.Printf("Reverting cascade for %s@%s\n", module, version)
	for _, item := range itemStates {
		fmt.Printf("  - Reverting %s\n", item.Repo)
		repoPath, err := deps.git.EnsureClone(ctx, item.Repo, cfg.Workspace.Path)
		if err != nil {
			logger.Warn("Failed to clone repository for revert", "repo", item.Repo, "error", err)
			continue
		}

		if item.Branch != "" {
			if err := runGitCommand(ctx, deps.gitRunner, repoPath, "push", "origin", "--delete", item.Branch); err != nil {
				logger.Warn("Failed to delete remote branch", "repo", item.Repo, "branch", item.Branch, "error", err)
			} else {
				fmt.Printf("    ✓ Deleted remote branch %s\n", item.Branch)
			}
			if err := runGitCommand(ctx, deps.gitRunner, repoPath, "branch", "-D", item.Branch); err != nil {
				logger.Warn("Failed to delete local branch", "repo", item.Repo, "branch", item.Branch, "error", err)
			}
		}

		if item.PRURL != "" {
			if number, err := extractPRNumber(item.PRURL); err == nil {
				pr := &broker.PullRequest{Repo: item.Repo, Number: number, URL: item.PRURL}
				message := "Cascade has reverted this update. Please close this pull request if appropriate."
				if commentErr := brokerSvc.Comment(ctx, pr, message); commentErr != nil {
					logger.Warn("Failed to leave revert comment", "repo", item.Repo, "pr", item.PRURL, "error", commentErr)
				}
			} else {
				logger.Warn("Unable to parse PR number from URL", "repo", item.Repo, "pr", item.PRURL, "error", err)
			}
		}

		item.Status = execpkg.StatusFailed
		item.Reason = appendReason(item.Reason, "reverted via cascade CLI")
		item.LastUpdated = time.Now()
		tracker.record(item)
	}

	tracker.finalize()
	fmt.Printf("Revert completed for %s@%s\n", module, version)
	return nil
}
