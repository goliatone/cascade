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

// newResumeCommand creates the resume subcommand
func newResumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resume [state-id]",
		Short: "Resume a previously interrupted operation",
		Long: `Resume continues a previously interrupted cascade operation
from its last known state using the state management system.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stateID := ""
			if len(args) > 0 {
				stateID = args[0]
			}
			return runResume(stateID)
		},
	}
}

func runResume(stateID string) error {
	start := time.Now()
	logger := container.Logger()
	cfg := container.Config()
	ctx := context.Background()

	defer func() {
		if logger != nil {
			logger.Debug("Resume command completed",
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

	manifestPath := resolveManifestPath("", cfg)
	manifestData, err := container.Manifest().Load(manifestPath)
	if err != nil {
		return newFileError("failed to load manifest", err)
	}

	plan, err := container.Planner().Plan(ctx, manifestData, planner.Target{Module: module, Version: version})
	if err != nil {
		return newPlanningError("failed to regenerate plan", err)
	}

	if cfg.Executor.DryRun {
		printResumeSummary(module, version, itemStates, plan)
		return nil
	}

	if err := ensureWorkspace(cfg.Workspace.Path); err != nil {
		return newExecutionError("failed to prepare workspace", err)
	}

	deps := newExecutionDeps()
	stateManager := container.State()
	tracker := newStateTracker(module, version, summary, stateManager, logger, itemStates)
	tracker.summary.RetryCount++
	tracker.saveSummary()

	statesByRepo := make(map[string]state.ItemState, len(itemStates))
	for _, st := range itemStates {
		statesByRepo[st.Repo] = st
	}

	executor := container.Executor()
	brokerSvc := container.Broker()

	retryCount := 0
	for i, item := range plan.Items {
		currentState, hasState := statesByRepo[item.Repo]
		if hasState && (currentState.Status == execpkg.StatusCompleted || currentState.Status == execpkg.StatusSkipped) {
			fmt.Printf("  %d. %s already %s\n", i+1, item.Repo, currentState.Status)
			continue
		}

		retryCount++
		fmt.Printf("  %d. Resuming %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)

		stateItem, err := processWorkItem(ctx, deps, cfg.Workspace.Path, item, executor, brokerSvc, logger, cfg.Executor.Timeout)
		if err != nil {
			logger.Warn("Resume attempt finished with errors", "repo", item.Repo, "error", err)
		}
		tracker.record(stateItem)
	}

	tracker.finalize()
	if retryCount == 0 {
		fmt.Printf("All work items for %s@%s are already complete\n", module, version)
	} else {
		fmt.Printf("Resume completed for %s@%s (reprocessed %d items)\n", module, version, retryCount)
	}
	return nil
}
