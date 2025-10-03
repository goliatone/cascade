package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/cascade/internal/planner"
	"github.com/spf13/cobra"
)

// newPlanCommand creates the plan subcommand
func newPlanCommand() *cobra.Command {
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
		Use:   "plan [manifest]",
		Short: "Plan dependency updates without executing them",
		Long: `Plan analyzes the dependency manifest and creates an execution plan
showing what updates would be performed, without making any changes.

Smart Defaults:
  - Manifest path: Auto-detected as .cascade.yaml or from positional argument
  - Module path: Auto-detected from go.mod in current directory tree
  - Version: Auto-detected from .version file, VERSION file, or latest git tag

Examples:
  cascade plan                                    # Use all auto-detected defaults
  cascade plan --module=github.com/example/lib   # Override just the module
  cascade plan --version=v1.2.3                  # Override just the version
  cascade plan custom-manifest.yaml              # Use custom manifest file
  cascade plan --check-strategy=remote           # Force remote checking for CI/CD`,
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

			return runPlan(manifestPath, manifestArg, modulePath, version)
		},
	}

	// Module and version flags (auto-detected if not provided)
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Manifest file path (default: .cascade.yaml)")
	cmd.Flags().StringVar(&modulePath, "module", "", "Target module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3). Auto-detected from .version file or git tags if not provided")

	// Dependency checking flags
	cmd.Flags().StringVar(&checkStrategy, "check-strategy", "auto", "Dependency checking mode: local, remote, or auto")
	cmd.Flags().DurationVar(&checkCacheTTL, "check-cache-ttl", 5*time.Minute, "Cache expiration time for remote checks")
	cmd.Flags().IntVar(&checkParallel, "check-parallel", 0, "Number of parallel checks (0 = auto-detect)")
	cmd.Flags().DurationVar(&checkTimeout, "check-timeout", 30*time.Second, "Timeout for individual repository checks")

	return cmd
}

func runPlan(manifestFlag, manifestArg, moduleFlag, versionFlag string) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	config := container.Config()

	// ForceAll overrides SkipUpToDate (already set from flags)
	if config.Executor.ForceAll {
		config.Executor.SkipUpToDate = false
	}

	// Resolve manifest path using same logic as manifest generate
	manifestPath := resolvePlanManifestPath(manifestFlag, manifestArg, config)

	defer func() {
		if logger != nil {
			logger.Debug("Plan command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"manifest", manifestPath,
				"dry_run", config.Executor.DryRun,
			)
		}
	}()

	// Detect module information when not explicitly provided
	finalModulePath := strings.TrimSpace(moduleFlag)
	moduleDir := ""
	if autoModulePath, autoModuleDir, err := detectModuleInfo(); err == nil {
		moduleDir = autoModuleDir
		if finalModulePath == "" {
			finalModulePath = autoModulePath
		}
	} else if finalModulePath == "" && config.Module == "" {
		return newValidationError("module path must be provided via --module flag, config, or go.mod must be present in the current directory", err)
	}

	// Use config fallback if no flag or auto-detection
	if finalModulePath == "" {
		finalModulePath = config.Module
	}

	// Resolve version if not provided
	finalVersion := strings.TrimSpace(versionFlag)
	var versionWarnings []string
	if finalVersion == "" {
		detectedVersion, warnings := detectDefaultVersion(ctx, moduleDir)
		versionWarnings = append(versionWarnings, warnings...)
		finalVersion = detectedVersion
	}

	// Use config fallback if no flag or auto-detection
	if finalVersion == "" {
		finalVersion = config.Version
	}

	// Validate target is specified
	if finalModulePath == "" {
		return newValidationError("target module must be specified via --module flag, config, or go.mod detection", nil)
	}
	if finalVersion == "" {
		return newValidationError("target version must be specified via --version flag, config, or version detection", nil)
	}

	// Display any version detection warnings
	for _, warning := range versionWarnings {
		logger.Warn("Version detection warning", "warning", warning)
	}

	logger.Info("Planning dependency updates",
		"manifest", manifestPath,
		"module", finalModulePath,
		"version", finalVersion)

	// Load the manifest
	manifest, err := container.Manifest().Load(manifestPath)
	if err != nil {
		return newFileError("failed to load manifest", err)
	}

	// Create target with resolved values
	target := planner.Target{
		Module:  finalModulePath,
		Version: finalVersion,
	}

	// Generate the plan
	plan, err := container.Planner().Plan(ctx, manifest, target)
	if err != nil {
		return newPlanningError("failed to generate plan", err)
	}

	// Display the plan
	if config.Executor.DryRun {
		fmt.Printf("DRY RUN: Planning updates for %s@%s\n", target.Module, target.Version)
	} else {
		fmt.Printf("Planning updates for %s@%s\n", target.Module, target.Version)
	}

	// Show planning statistics if dependency checking was enabled
	if config.Executor.SkipUpToDate && plan.Stats.TotalDependents > 0 {
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
					config.Executor.CheckParallel)
			} else {
				fmt.Printf("  - Check duration: %.1fs\n", durationSec)
			}
		}

		// Add performance warnings
		showPerformanceWarnings(&plan.Stats, config.Executor.CheckParallel)

		fmt.Println()
	}

	fmt.Printf("Found %d work items:\n", len(plan.Items))
	for i, item := range plan.Items {
		fmt.Printf("  %d. %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)

		if len(item.Tests) > 0 {
			fmt.Println("     Tests:")
			for _, cmd := range item.Tests {
				fmt.Printf("       - %s\n", strings.Join(cmd.Cmd, " "))
			}
		}

		if len(item.ExtraCommands) > 0 {
			fmt.Println("     Extra Commands:")
			for _, cmd := range item.ExtraCommands {
				fmt.Printf("       - %s\n", strings.Join(cmd.Cmd, " "))
			}
		}
	}

	return nil
}

// showPerformanceWarnings displays performance-related warnings based on check statistics.
func showPerformanceWarnings(stats *planner.PlanStats, configuredParallel int) {
	// Warn if remote checking takes >30s total
	if stats.CheckDuration > 30*time.Second {
		fmt.Printf("  ⚠ Warning: Dependency checks took %.1fs (>30s)\n", stats.CheckDuration.Seconds())

		// Suggest increasing parallelism if checks are slow and not already parallelized
		if !stats.ParallelChecks || configuredParallel < 4 {
			fmt.Printf("  ⚠ Consider increasing parallelism with --check-parallel=8\n")
		}
	}

	// Warn about cache misses in repeated runs (only for remote/auto strategies)
	if (stats.CheckStrategy == "remote" || stats.CheckStrategy == "auto") && stats.CacheMisses > 0 {
		total := stats.CacheHits + stats.CacheMisses
		if total > 0 {
			hitRate := float64(stats.CacheHits) / float64(total)
			// If cache hit rate is below 50% and we have a significant number of checks
			if hitRate < 0.5 && total > 5 {
				fmt.Printf("  ⚠ Low cache hit rate (%.0f%%). Repeated runs may be slower than expected.\n", hitRate*100)
			}
		}
	}
}
