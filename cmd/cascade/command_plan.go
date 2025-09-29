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
		manifestPath string
		modulePath   string
		version      string
		skipUpToDate bool
		forceAll     bool
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
  cascade plan custom-manifest.yaml              # Use custom manifest file`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestArg := ""
			if len(args) > 0 {
				manifestArg = args[0]
			}
			return runPlan(manifestPath, manifestArg, modulePath, version, skipUpToDate, forceAll)
		},
	}

	// Module and version flags (auto-detected if not provided)
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Manifest file path (default: .cascade.yaml)")
	cmd.Flags().StringVar(&modulePath, "module", "", "Target module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3). Auto-detected from .version file or git tags if not provided")
	cmd.Flags().BoolVar(&skipUpToDate, "skip-up-to-date", true, "Skip dependents that are already up-to-date (default: true)")
	cmd.Flags().BoolVar(&forceAll, "force-all", false, "Process all dependents regardless of current version")

	return cmd
}

func runPlan(manifestFlag, manifestArg, moduleFlag, versionFlag string, skipUpToDate, forceAll bool) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	config := container.Config()

	// Apply flag values to configuration
	config.Executor.SkipUpToDate = skipUpToDate
	config.Executor.ForceAll = forceAll

	// ForceAll overrides SkipUpToDate
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
		fmt.Printf("\nChecked %d potential dependents:\n", plan.Stats.TotalDependents)
		if plan.Stats.SkippedUpToDate > 0 {
			fmt.Printf("  - %d repositories already up-to-date, skipped\n", plan.Stats.SkippedUpToDate)
		}
		if plan.Stats.WorkItemsCreated > 0 {
			fmt.Printf("  - %d require updates\n", plan.Stats.WorkItemsCreated)
		}
		if plan.Stats.CheckErrors > 0 {
			fmt.Printf("  - %d check errors (included for safety)\n", plan.Stats.CheckErrors)
		}
		fmt.Println()
	}

	fmt.Printf("Found %d work items:\n", len(plan.Items))
	for i, item := range plan.Items {
		fmt.Printf("  %d. %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)
	}

	return nil
}
