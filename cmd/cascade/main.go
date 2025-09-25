package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

// Exit codes for different error types
const (
	ExitSuccess         = 0  // Successful execution
	ExitGenericError    = 1  // Generic error
	ExitConfigError     = 2  // Configuration error
	ExitValidationError = 3  // Input validation error
	ExitNetworkError    = 4  // Network/connectivity error
	ExitFileError       = 5  // File system error
	ExitStateError      = 6  // State management error
	ExitPlanningError   = 7  // Planning phase error
	ExitExecutionError  = 8  // Execution phase error
	ExitInterruptError  = 9  // User interruption (SIGINT, etc.)
	ExitResourceError   = 10 // Resource exhaustion (disk, memory, etc.)
)

// Error types for structured error handling
type CLIError struct {
	Code    int
	Message string
	Cause   error
}

func (e *CLIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *CLIError) ExitCode() int {
	return e.Code
}

// Global variables for CLI state
var (
	container di.Container
	cfg       *config.Config
)

func main() {
	if err := execute(); err != nil {
		// Handle structured errors with appropriate exit codes
		if cliErr, ok := err.(*CLIError); ok {
			fmt.Fprintf(os.Stderr, "cascade: %s\n", cliErr.Message)
			if cliErr.Cause != nil {
				fmt.Fprintf(os.Stderr, "  Cause: %v\n", cliErr.Cause)
			}
			os.Exit(cliErr.ExitCode())
		}

		// Handle other error types
		fmt.Fprintf(os.Stderr, "cascade: %v\n", err)
		os.Exit(ExitGenericError)
	}
}

// execute is the main entry point that sets up and runs the CLI
func execute() error {
	rootCmd := newRootCommand()
	return rootCmd.Execute()
}

// newRootCommand creates the root cobra command with all subcommands
func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cascade",
		Short: "Cascade orchestrates automated dependency updates across Go repositories",
		Long: `Cascade is a CLI tool that orchestrates automated dependency updates across
multiple Go repositories. It reads dependency manifests, plans updates,
executes changes, and manages pull requests through GitHub integration.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeContainer(cmd)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			cleanupContainer()
		},
	}

	// Add configuration flags
	config.AddFlags(cmd)

	// Add subcommands
	cmd.AddCommand(
		newPlanCommand(),
		newReleaseCommand(),
		newResumeCommand(),
		newRevertCommand(),
	)

	return cmd
}

// initializeContainer sets up the dependency injection container with configuration
func initializeContainer(cmd *cobra.Command) error {
	// Build configuration from flags, environment, and files
	builder := config.NewBuilder().
		FromFile("").  // Auto-discover config file
		FromEnv().     // Load from environment
		FromFlags(cmd) // Load from command flags (highest precedence)

	var err error
	cfg, err = builder.Build()
	if err != nil {
		return newConfigError("failed to build configuration", err)
	}

	// Create container with configuration
	container, err = di.New(di.WithConfig(cfg))
	if err != nil {
		return newConfigError("failed to initialize dependencies", err)
	}

	return nil
}

// cleanupContainer performs cleanup of container resources
func cleanupContainer() {
	if container != nil {
		if err := container.Close(); err != nil {
			// Log cleanup errors but don't fail the program
			if logger := container.Logger(); logger != nil {
				logger.Warn("Container cleanup errors", "error", err)
			} else {
				// Fallback to stderr if logger unavailable
				fmt.Fprintf(os.Stderr, "cascade: container cleanup warning: %v\n", err)
			}
		}
	}
}

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

// Command implementations

func runPlan(manifestPath string) error {
	ctx := context.Background()
	logger := container.Logger()
	config := container.Config()

	// Use default manifest path if none provided
	if manifestPath == "" {
		manifestPath = config.Workspace.ManifestPath
	}
	if manifestPath == "" {
		manifestPath = "deps.yaml" // Default fallback
	}

	logger.Info("Planning dependency updates", "manifest", manifestPath)

	// Load the manifest
	manifest, err := container.Manifest().Load(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	// Create target from config or CLI args
	target := planner.Target{
		Module:  config.Module,
		Version: config.Version,
	}

	// Validate target is specified
	if target.Module == "" {
		return fmt.Errorf("target module must be specified via --module flag or config")
	}
	if target.Version == "" {
		return fmt.Errorf("target version must be specified via --version flag or config")
	}

	// Generate the plan
	plan, err := container.Planner().Plan(ctx, manifest, target)
	if err != nil {
		return fmt.Errorf("failed to generate plan: %w", err)
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

func runRelease(manifestPath string) error {
	ctx := context.Background()
	logger := container.Logger()
	config := container.Config()

	// Use default manifest path if none provided
	if manifestPath == "" {
		manifestPath = config.Workspace.ManifestPath
	}
	if manifestPath == "" {
		manifestPath = "deps.yaml" // Default fallback
	}

	logger.Info("Executing dependency updates", "manifest", manifestPath)

	// Create target from config
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

	// Load the manifest
	manifest, err := container.Manifest().Load(manifestPath)
	if err != nil {
		return newFileError("failed to load manifest", err)
	}

	// Generate the plan
	plan, err := container.Planner().Plan(ctx, manifest, target)
	if err != nil {
		return newPlanningError("failed to generate plan", err)
	}

	if config.Executor.DryRun {
		fmt.Printf("DRY RUN: Would execute updates for %s@%s\n", target.Module, target.Version)
		fmt.Printf("Would process %d work items:\n", len(plan.Items))
		for i, item := range plan.Items {
			fmt.Printf("  %d. %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)
		}
		return nil
	}

	// Save initial state
	stateManager := container.State()
	summary := &state.Summary{
		Module:     target.Module,
		Version:    target.Version,
		StartTime:  time.Now(),
		RetryCount: 0,
	}

	if err := stateManager.SaveSummary(summary); err != nil {
		logger.Warn("Failed to save initial state", "error", err)
	}

	fmt.Printf("Executing updates for %s@%s\n", target.Module, target.Version)
	fmt.Printf("Processing %d work items:\n", len(plan.Items))

	// For now, just show what would be executed
	// TODO: Implement actual execution using container.Executor()
	for i, item := range plan.Items {
		fmt.Printf("  %d. Processing %s (%s) -> %s\n", i+1, item.Repo, item.Module, item.BranchName)
		// TODO: Execute work item using container.Executor().Apply()
	}

	fmt.Println("Release execution not fully implemented - showing plan only")
	return nil
}

func runResume(stateID string) error {
	logger := container.Logger()
	config := container.Config()

	// Parse stateID as module@version if provided, otherwise use config
	var module, version string
	if stateID != "" {
		// Try to parse as module@version
		if parts := splitModuleVersion(stateID); len(parts) == 2 {
			module, version = parts[0], parts[1]
		} else {
			return fmt.Errorf("invalid state ID format: expected module@version, got %s", stateID)
		}
	} else {
		// Use from config
		module, version = config.Module, config.Version
		if module == "" || version == "" {
			return fmt.Errorf("state ID or module/version must be specified")
		}
	}

	logger.Info("Resuming operation", "module", module, "version", version)

	// Load existing state
	stateManager := container.State()
	summary, err := stateManager.LoadSummary(module, version)
	if err != nil {
		if err == state.ErrNotFound {
			return fmt.Errorf("no saved state found for %s@%s", module, version)
		}
		return fmt.Errorf("failed to load state: %w", err)
	}

	itemStates, err := stateManager.LoadItemStates(module, version)
	if err != nil {
		return fmt.Errorf("failed to load item states: %w", err)
	}

	fmt.Printf("Resuming cascade for %s@%s\n", module, version)
	fmt.Printf("Started: %s\n", summary.StartTime.Format(time.RFC3339))
	fmt.Printf("Retry count: %d\n", summary.RetryCount)
	fmt.Printf("Items: %d\n", len(itemStates))

	// Show status of each item
	for _, item := range itemStates {
		fmt.Printf("  - %s: %s", item.Repo, item.Status)
		if item.Reason != "" {
			fmt.Printf(" (%s)", item.Reason)
		}
		if item.PRURL != "" {
			fmt.Printf(" - PR: %s", item.PRURL)
		}
		fmt.Println()
	}

	fmt.Println("Resume execution not fully implemented - showing state only")
	return nil
}

func runRevert(stateID string) error {
	logger := container.Logger()
	config := container.Config()

	// Parse stateID as module@version if provided, otherwise use config
	var module, version string
	if stateID != "" {
		// Try to parse as module@version
		if parts := splitModuleVersion(stateID); len(parts) == 2 {
			module, version = parts[0], parts[1]
		} else {
			return fmt.Errorf("invalid state ID format: expected module@version, got %s", stateID)
		}
	} else {
		// Use from config
		module, version = config.Module, config.Version
		if module == "" || version == "" {
			return fmt.Errorf("state ID or module/version must be specified")
		}
	}

	logger.Info("Reverting operation", "module", module, "version", version)

	// Load existing state
	stateManager := container.State()
	summary, err := stateManager.LoadSummary(module, version)
	if err != nil {
		if err == state.ErrNotFound {
			return fmt.Errorf("no saved state found for %s@%s", module, version)
		}
		return fmt.Errorf("failed to load state: %w", err)
	}

	itemStates, err := stateManager.LoadItemStates(module, version)
	if err != nil {
		return fmt.Errorf("failed to load item states: %w", err)
	}

	if config.Executor.DryRun {
		fmt.Printf("DRY RUN: Would revert cascade for %s@%s\n", module, version)
		fmt.Printf("Would revert %d items:\n", len(itemStates))
		for _, item := range itemStates {
			if item.PRURL != "" {
				fmt.Printf("  - Close PR: %s (%s)\n", item.PRURL, item.Repo)
			}
			if item.CommitHash != "" {
				fmt.Printf("  - Cleanup branch: %s (%s)\n", item.Branch, item.Repo)
			}
		}
		return nil
	}

	fmt.Printf("Reverting cascade for %s@%s\n", module, version)
	fmt.Printf("Started: %s\n", summary.StartTime.Format(time.RFC3339))

	// TODO: Implement actual revert logic using broker to close PRs and cleanup branches
	for _, item := range itemStates {
		fmt.Printf("  - Reverting %s", item.Repo)
		if item.PRURL != "" {
			fmt.Printf(" (close PR: %s)", item.PRURL)
		}
		fmt.Println()
	}

	fmt.Println("Revert execution not fully implemented - showing plan only")
	return nil
}

// Helper function to split module@version strings
func splitModuleVersion(stateID string) []string {
	parts := strings.Split(stateID, "@")
	if len(parts) != 2 {
		return nil
	}
	return parts
}

// Error creation helpers for structured error handling

func newConfigError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitConfigError, Message: message, Cause: cause}
}

func newValidationError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitValidationError, Message: message, Cause: cause}
}

func newFileError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitFileError, Message: message, Cause: cause}
}

func newStateError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitStateError, Message: message, Cause: cause}
}

func newPlanningError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitPlanningError, Message: message, Cause: cause}
}

func newExecutionError(message string, cause error) *CLIError {
	return &CLIError{Code: ExitExecutionError, Message: message, Cause: cause}
}
