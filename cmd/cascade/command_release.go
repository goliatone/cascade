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
	var (
		manifestPath string
		modulePath   string
		version      string
		skipUpToDate bool
		forceAll     bool
	)

	cmd := &cobra.Command{
		Use:   "release [manifest]",
		Short: "Execute planned dependency updates",
		Long: `Release executes the dependency update plan, creating branches,
making changes, and submitting pull requests as configured.

Smart Defaults:
  - Manifest path: Auto-detected as .cascade.yaml (non-conflicting default)
  - Module path: Auto-detected from go.mod in current directory tree
  - Version: Auto-detected from .version file, VERSION file, or latest git tag

Examples:
  cascade release                                    # Use all auto-detected defaults
  cascade release --module=github.com/example/lib   # Override just the module
  cascade release --version=v1.2.3                  # Override just the version
  cascade release .cascade.yaml                     # Explicit manifest file`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestArg := ""
			if len(args) > 0 {
				manifestArg = args[0]
			}
			return runRelease(manifestPath, manifestArg, modulePath, version, skipUpToDate, forceAll)
		},
	}

	// Flags for overriding auto-detected defaults
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to dependency manifest file (default: .cascade.yaml)")
	cmd.Flags().StringVar(&modulePath, "module", "", "Go module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3). Auto-detected from .version file or git tags if not provided")
	cmd.Flags().BoolVar(&skipUpToDate, "skip-up-to-date", true, "Skip dependents that are already up-to-date (default: true)")
	cmd.Flags().BoolVar(&forceAll, "force-all", false, "Process all dependents regardless of current version")

	return cmd
}

func runRelease(manifestFlag, manifestArg, modulePath, version string, skipUpToDate, forceAll bool) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	cfg := container.Config()

	// Apply flag values to configuration
	cfg.Executor.SkipUpToDate = skipUpToDate
	cfg.Executor.ForceAll = forceAll

	// ForceAll overrides SkipUpToDate
	if cfg.Executor.ForceAll {
		cfg.Executor.SkipUpToDate = false
	}

	defer func() {
		if logger != nil {
			logger.Debug("Release command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"manifest", manifestFlag,
				"dry_run", cfg.Executor.DryRun,
			)
		}
	}()

	// Apply default discovery logic for manifest path
	finalManifestPath := resolvePlanManifestPath(manifestFlag, manifestArg, cfg)
	if finalManifestPath == "" {
		return newValidationError("manifest path not provided and no default configured", nil)
	}

	// Apply default discovery logic for module path
	finalModulePath := modulePath
	if finalModulePath == "" && cfg != nil {
		finalModulePath = cfg.Module // Use config as fallback
	}

	var moduleDir string
	var err error
	finalModulePath, moduleDir, err = applyModuleDefaults(finalModulePath)
	if err != nil {
		return err
	}

	// Apply default discovery logic for version
	finalVersion := version
	if finalVersion == "" && cfg != nil {
		finalVersion = cfg.Version // Use config as fallback
	}

	var versionWarnings []string
	finalVersion, versionWarnings, err = applyVersionDefaults(ctx, finalVersion, moduleDir, cfg)
	if err != nil {
		return err
	}

	// Log version warnings if any
	if len(versionWarnings) > 0 && logger != nil {
		for _, warning := range versionWarnings {
			logger.Warn("Version resolution warning", "warning", warning)
		}
	}

	if err := ensureWorkspace(cfg.Workspace.Path); err != nil {
		return newExecutionError("failed to prepare workspace", err)
	}

	logger.Info("Executing dependency updates",
		"manifest", finalManifestPath,
		"module", finalModulePath,
		"version", finalVersion)

	target := planner.Target{Module: finalModulePath, Version: finalVersion}

	manifestData, err := container.Manifest().Load(finalManifestPath)
	if err != nil {
		return newFileError("failed to load manifest", err)
	}

	plan, err := container.Planner().Plan(ctx, manifestData, target)
	if err != nil {
		return newPlanningError("failed to generate plan", err)
	}

	// Show planning statistics if dependency checking was enabled
	if cfg.Executor.SkipUpToDate && plan.Stats.TotalDependents > 0 {
		fmt.Printf("Checked %d potential dependents:\n", plan.Stats.TotalDependents)
		if plan.Stats.SkippedUpToDate > 0 {
			fmt.Printf("  - %d repositories already up-to-date, skipped\n", plan.Stats.SkippedUpToDate)
		}
		if plan.Stats.WorkItemsCreated > 0 {
			fmt.Printf("  - %d require updates\n", plan.Stats.WorkItemsCreated)
		}
		if plan.Stats.CheckErrors > 0 {
			fmt.Printf("  - %d check errors (included for safety)\n", plan.Stats.CheckErrors)
		}
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
