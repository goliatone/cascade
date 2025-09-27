package main

import (
	"context"
	"fmt"
	"time"

	"github.com/goliatone/cascade/internal/planner"
	"github.com/spf13/cobra"
)

// newPlanCommand creates the plan subcommand
func newPlanCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "plan [manifest]",
		Short: "Plan dependency updates without executing them",
		Long: `Plan analyzes the dependency manifest and creates an execution plan
showing what updates would be performed, without making any changes.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath := ""
			if len(args) > 0 {
				manifestPath = args[0]
			}
			return runPlan(manifestPath)
		},
	}
}

func runPlan(manifestPath string) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	config := container.Config()

	defer func() {
		if logger != nil {
			logger.Debug("Plan command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"manifest", manifestPath,
				"dry_run", config.Executor.DryRun,
			)
		}
	}()

	// Use default manifest path if none provided
	if manifestPath == "" {
		manifestPath = config.Workspace.ManifestPath
	}
	if manifestPath == "" {
		manifestPath = ".cascade.yaml" // Default fallback
	}

	logger.Info("Planning dependency updates", "manifest", manifestPath)

	// Load the manifest
	manifest, err := container.Manifest().Load(manifestPath)
	if err != nil {
		return newFileError("failed to load manifest", err)
	}

	// Create target from config or CLI args
	target := planner.Target{
		Module:  config.Module,
		Version: config.Version,
	}

	// Validate target is specified
	if target.Module == "" {
		return newValidationError("target module must be specified via --module flag or config", nil)
	}
	if target.Version == "" {
		return newValidationError("target version must be specified via --version flag or config", nil)
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

	fmt.Printf("Found %d work items:\n", len(plan.Items))
	for i, item := range plan.Items {
		fmt.Printf("  %d. %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)
	}

	return nil
}
