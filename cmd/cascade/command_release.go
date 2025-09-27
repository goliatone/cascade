package main

import (
	"context"
	"fmt"
	"time"

	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/spf13/cobra"
)

// newReleaseCommand creates the release subcommand
func newReleaseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "release [manifest]",
		Short: "Execute planned dependency updates",
		Long: `Release executes the dependency update plan, creating branches,
making changes, and submitting pull requests as configured.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath := ""
			if len(args) > 0 {
				manifestPath = args[0]
			}
			return runRelease(manifestPath)
		},
	}
}

func runRelease(manifestPath string) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	cfg := container.Config()

	defer func() {
		if logger != nil {
			logger.Debug("Release command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"manifest", manifestPath,
				"dry_run", cfg.Executor.DryRun,
			)
		}
	}()

	manifestPath = resolveManifestPath(manifestPath, cfg)
	if manifestPath == "" {
		return newValidationError("manifest path not provided and no default configured", nil)
	}

	if err := ensureWorkspace(cfg.Workspace.Path); err != nil {
		return newExecutionError("failed to prepare workspace", err)
	}

	logger.Info("Executing dependency updates", "manifest", manifestPath)

	target := planner.Target{Module: cfg.Module, Version: cfg.Version}
	if target.Module == "" {
		return newValidationError("target module must be specified via --module flag or config", nil)
	}
	if target.Version == "" {
		return newValidationError("target version must be specified via --version flag or config", nil)
	}

	manifestData, err := container.Manifest().Load(manifestPath)
	if err != nil {
		return newFileError("failed to load manifest", err)
	}

	plan, err := container.Planner().Plan(ctx, manifestData, target)
	if err != nil {
		return newPlanningError("failed to generate plan", err)
	}

	if len(plan.Items) == 0 {
		fmt.Printf("No work items produced for %s@%s\n", target.Module, target.Version)
		return nil
	}

	if cfg.Executor.DryRun {
		fmt.Printf("DRY RUN: Would execute updates for %s@%s\n", target.Module, target.Version)
		fmt.Printf("Would process %d work items:\n", len(plan.Items))
		for i, item := range plan.Items {
			fmt.Printf("  %d. %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)
		}
		return nil
	}

	deps := newExecutionDeps()
	stateManager := container.State()
	summary := &state.Summary{Module: target.Module, Version: target.Version, StartTime: time.Now()}
	tracker := newStateTracker(target.Module, target.Version, summary, stateManager, logger, nil)

	executor := container.Executor()
	brokerSvc := container.Broker()

	fmt.Printf("Executing updates for %s@%s\n", target.Module, target.Version)
	for i, item := range plan.Items {
		fmt.Printf("  %d. %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)
		itemState, err := processWorkItem(ctx, deps, cfg.Workspace.Path, item, executor, brokerSvc, logger, cfg.Executor.Timeout)
		if err != nil {
			logger.Warn("Work item completed with errors", "repo", item.Repo, "error", err)
		}
		tracker.record(itemState)

		switch itemState.Status {
		case execpkg.StatusCompleted:
			if itemState.PRURL != "" {
				fmt.Printf("    ✓ PR: %s\n", itemState.PRURL)
			} else {
				fmt.Printf("    ✓ Completed with commit %s\n", itemState.CommitHash)
			}
		case execpkg.StatusManualReview:
			fmt.Printf("    ! Manual review required: %s\n", itemState.Reason)
		case execpkg.StatusSkipped:
			fmt.Printf("    ⏭ Skipped: %s\n", itemState.Reason)
		default:
			fmt.Printf("    ✗ Failed: %s\n", itemState.Reason)
		}
	}

	tracker.finalize()
	fmt.Printf("Release execution completed for %s@%s\n", target.Module, target.Version)
	return nil
}
