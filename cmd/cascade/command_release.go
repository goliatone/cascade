package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goliatone/cascade/internal/broker"
	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/spf13/cobra"
)

// newReleaseCommand creates the release subcommand
func newReleaseCommand() *cobra.Command {
	var (
		manifestPath  string
		modulePath    string
		version       string
		checkStrategy string
		checkCacheTTL time.Duration
		checkParallel int
		checkTimeout  time.Duration
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
  cascade release .cascade.yaml                     # Explicit manifest file
  cascade release --check-strategy=remote           # Force remote checking for CI/CD`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestArg := ""
			if len(args) > 0 {
				manifestArg = args[0]
			}

			// Apply CLI flag overrides to config
			config := container.Config()
			if cmd.Flags().Changed("check-strategy") {
				config.Executor.CheckStrategy = checkStrategy
			}
			if cmd.Flags().Changed("check-cache-ttl") {
				config.Executor.CheckCacheTTL = checkCacheTTL
			}
			if cmd.Flags().Changed("check-parallel") {
				config.Executor.CheckParallel = checkParallel
			}
			if cmd.Flags().Changed("check-timeout") {
				config.Executor.CheckTimeout = checkTimeout
			}

			return runRelease(manifestPath, manifestArg, modulePath, version)
		},
	}

	// Flags for overriding auto-detected defaults
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to dependency manifest file (default: .cascade.yaml)")
	cmd.Flags().StringVar(&modulePath, "module", "", "Go module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3). Auto-detected from .version file or git tags if not provided")

	// Dependency checking flags
	cmd.Flags().StringVar(&checkStrategy, "check-strategy", "auto", "Dependency checking mode: local, remote, or auto")
	cmd.Flags().DurationVar(&checkCacheTTL, "check-cache-ttl", 5*time.Minute, "Cache expiration time for remote checks")
	cmd.Flags().IntVar(&checkParallel, "check-parallel", 0, "Number of parallel checks (0 = auto-detect)")
	cmd.Flags().DurationVar(&checkTimeout, "check-timeout", 30*time.Second, "Timeout for individual repository checks")

	return cmd
}

func runRelease(manifestFlag, manifestArg, modulePath, version string) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	cfg := container.Config()

	// ForceAll overrides SkipUpToDate (already set from flags)
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

	// Extract notification settings from manifest defaults
	var manifestNotifications *di.ManifestNotifications
	if manifestData.Defaults.Notifications.SlackChannel != "" || manifestData.Defaults.Notifications.Webhook != "" {
		manifestNotifications = &di.ManifestNotifications{
			SlackChannel: manifestData.Defaults.Notifications.SlackChannel,
			Webhook:      manifestData.Defaults.Notifications.Webhook,
		}
		logger.Debug("Found notification settings in manifest",
			"slack_channel", manifestNotifications.SlackChannel,
			"webhook", manifestNotifications.Webhook)
	}

	// Show planning statistics if dependency checking was enabled
	if cfg.Executor.SkipUpToDate && plan.Stats.TotalDependents > 0 {
		// Display strategy-specific header
		strategyLabel := plan.Stats.CheckStrategy
		if strategyLabel == "" {
			strategyLabel = "local"
		}

		fmt.Printf("\nDependency Checking (%s mode):\n", strategyLabel)

		// Show cache statistics for remote/auto modes
		if plan.Stats.CheckStrategy == "remote" || plan.Stats.CheckStrategy == "auto" {
			totalChecked := plan.Stats.CacheHits + plan.Stats.CacheMisses
			if totalChecked > 0 {
				fmt.Printf("  - Checked %d repositories (%d cached, %d fetched)\n",
					plan.Stats.TotalDependents,
					plan.Stats.CacheHits,
					plan.Stats.CacheMisses)
			}
		} else {
			fmt.Printf("  - Checked %d repositories\n", plan.Stats.TotalDependents)
		}

		if plan.Stats.SkippedUpToDate > 0 {
			fmt.Printf("  - %d repositories up-to-date, skipped\n", plan.Stats.SkippedUpToDate)
		}
		if plan.Stats.WorkItemsCreated > 0 {
			fmt.Printf("  - %d require updates\n", plan.Stats.WorkItemsCreated)
		}
		if plan.Stats.CheckErrors > 0 {
			fmt.Printf("  - %d check errors (included for safety)\n", plan.Stats.CheckErrors)
		}

		// Show performance metrics
		if plan.Stats.CheckDuration > 0 {
			durationSec := plan.Stats.CheckDuration.Seconds()
			if plan.Stats.ParallelChecks {
				fmt.Printf("  - Check duration: %.1fs (parallel: %d)\n",
					durationSec,
					cfg.Executor.CheckParallel)
			} else {
				fmt.Printf("  - Check duration: %.1fs\n", durationSec)
			}
		}

		// Add performance warnings
		showPerformanceWarnings(&plan.Stats, cfg.Executor.CheckParallel)

		fmt.Println()
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

	// Get broker with manifest notification settings if available
	var brokerSvc broker.Broker
	if manifestNotifications != nil {
		brokerSvc, err = container.BrokerWithManifestNotifications(manifestNotifications)
		if err != nil {
			return newExecutionError("failed to initialize broker with manifest notifications", err)
		}
	} else {
		brokerSvc = container.Broker()
	}

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
