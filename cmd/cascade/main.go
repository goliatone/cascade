package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/goliatone/cascade/internal/broker"
	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
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
		handleCLIError(err)
	}
}

// handleCLIError processes and exits with appropriate error codes
func handleCLIError(err error) {
	if err == nil {
		return
	}

	// Handle structured errors with appropriate exit codes
	if cliErr, ok := err.(*CLIError); ok {
		fmt.Fprintf(os.Stderr, "cascade: %s\n", cliErr.Message)
		if cliErr.Cause != nil {
			fmt.Fprintf(os.Stderr, "  Cause: %v\n", cliErr.Cause)
		}
		os.Exit(cliErr.ExitCode())
	}

	// Try to infer error type from error message patterns for better exit codes
	errorMsg := err.Error()

	// Configuration and validation errors
	if strings.Contains(errorMsg, "configuration") || strings.Contains(errorMsg, "config") {
		fmt.Fprintf(os.Stderr, "cascade: configuration error: %v\n", err)
		os.Exit(ExitConfigError)
	}

	// File system errors
	if strings.Contains(errorMsg, "no such file") || strings.Contains(errorMsg, "permission denied") ||
		strings.Contains(errorMsg, "file not found") || strings.Contains(errorMsg, "manifest") {
		fmt.Fprintf(os.Stderr, "cascade: file error: %v\n", err)
		os.Exit(ExitFileError)
	}

	// Validation errors
	if strings.Contains(errorMsg, "must be specified") || strings.Contains(errorMsg, "invalid") ||
		strings.Contains(errorMsg, "validation") || strings.Contains(errorMsg, "required") {
		fmt.Fprintf(os.Stderr, "cascade: validation error: %v\n", err)
		os.Exit(ExitValidationError)
	}

	// Network/connectivity errors
	if strings.Contains(errorMsg, "network") || strings.Contains(errorMsg, "connection") ||
		strings.Contains(errorMsg, "timeout") || strings.Contains(errorMsg, "unreachable") {
		fmt.Fprintf(os.Stderr, "cascade: network error: %v\n", err)
		os.Exit(ExitNetworkError)
	}

	// Generic error fallback
	fmt.Fprintf(os.Stderr, "cascade: %v\n", err)
	os.Exit(ExitGenericError)
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
executes changes, and manages pull requests through GitHub integration.

Configuration Sources (in precedence order):
  1. Command-line flags (highest priority)
  2. Environment variables (CASCADE_*)
  3. Configuration files (~/.config/cascade/config.yaml)
  4. Built-in defaults (lowest priority)

Exit Codes:
  0  - Success
  1  - Generic error
  2  - Configuration error (missing config, invalid values)
  3  - Validation error (missing required flags, invalid arguments)
  4  - Network error (connectivity issues, API failures)
  5  - File system error (missing files, permission issues)
  6  - State management error (corrupted state, lock failures)
  7  - Planning error (manifest parsing, dependency resolution)
  8  - Execution error (git operations, build failures)
  9  - User interruption (SIGINT, SIGTERM)
  10 - Resource exhaustion (disk space, memory)

Examples:
  cascade plan --module=github.com/example/lib --version=v1.2.3
  cascade release --manifest=deps.yaml --dry-run
  CASCADE_GITHUB_TOKEN=token cascade release --module=github.com/example/lib --version=v1.2.3`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeContainer(cmd)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			cleanupContainer()
		},
	}

	// Override Cobra's default error handling to use structured errors
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return newValidationError("invalid flag usage", err)
	})

	// Set up error handling for argument validation
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		// Allow subcommands to handle their own args
		if cmd.HasSubCommands() {
			return nil
		}
		return cobra.ArbitraryArgs(cmd, args)
	}

	// Add configuration flags
	config.AddFlags(cmd)

	// Add subcommands
	cmd.AddCommand(
		newManifestCommand(),
		newPlanCommand(),
		newReleaseCommand(),
		newResumeCommand(),
		newRevertCommand(),
	)

	return cmd
}

// initializeContainer sets up the dependency injection container with configuration
func initializeContainer(cmd *cobra.Command) error {
	start := time.Now()
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

	// Determine if this is a production command that requires credentials
	containerOptions := []di.Option{di.WithConfig(cfg)}
	if isProductionCommand(cmd) {
		containerOptions = append(containerOptions, di.WithProductionCredentials())
	}

	// Enable instrumentation if debug logging is enabled
	if cfg.Logging.Level == "debug" || cfg.Logging.Verbose {
		containerOptions = append(containerOptions, di.WithInstrumentation())
	}

	// Create container with configuration
	container, err = di.New(containerOptions...)
	if err != nil {
		return newConfigError("failed to initialize dependencies", err)
	}

	// Log container initialization metrics if logger is available
	if logger := container.Logger(); logger != nil {
		duration := time.Since(start)
		commandName := cmd.Name()
		if cmd.Parent() != nil {
			commandName = cmd.Parent().Name() + " " + cmd.Name()
		}
		logger.Debug("CLI container initialized",
			"command", commandName,
			"duration_ms", duration.Milliseconds(),
			"production_mode", isProductionCommand(cmd),
		)
	}

	return nil
}

// isProductionCommand determines if the given command requires production credentials.
// Production commands (release, resume, revert) create PRs and make API calls that require GitHub tokens.
// The plan command can work with stub implementations for dry-run scenarios.
func isProductionCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}

	// Walk up the command hierarchy to find the root command name
	current := cmd
	for current.Parent() != nil {
		current = current.Parent()
	}

	// Check the immediate subcommand of root
	if cmd.Parent() != nil && cmd.Parent().Name() == "cascade" {
		switch cmd.Name() {
		case "release", "resume", "revert":
			return true
		case "plan":
			return false
		}
	}

	return false
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

// newManifestCommand creates the manifest management subcommand with generate subcommand
func newManifestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage dependency manifests",
		Long: `Manifest management commands for creating and manipulating dependency manifests.
Use subcommands to perform specific manifest operations.`,
	}

	cmd.AddCommand(newManifestGenerateCommand())
	return cmd
}

// newManifestGenerateCommand creates the manifest generate subcommand
func newManifestGenerateCommand() *cobra.Command {
	var (
		moduleName      string
		modulePath      string
		repository      string
		version         string
		outputPath      string
		dependents      []string
		slackChannel    string
		webhook         string
		force           bool
		yes             bool
		nonInteractive  bool
		workspace       string
		maxDepth        int
		includePatterns []string
		excludePatterns []string
	)

	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen"},
		Short:   "Generate a new dependency manifest",
		Long: `Generate creates a new dependency manifest file with the specified module
and configuration options. The manifest follows the TDD defaults and includes
sensible default configurations for commit templates, PR templates, and notifications.

When --dependents is omitted, cascade will automatically discover dependent repositories
by scanning the workspace for Go modules that import the target module.

The command will display a summary of discovered dependents and default configurations
before proceeding. Use --yes or --non-interactive to skip confirmation prompts.

Examples:
  cascade manifest generate --module-path=github.com/example/lib --version=v1.2.3
  cascade manifest gen --module-path=github.com/example/lib --module-name=mylib --version=v1.2.3 --output=deps.yaml
  cascade manifest generate --module-path=github.com/example/lib --version=v1.2.3 --dependents=owner/repo1,owner/repo2
  cascade manifest generate --module-path=github.com/example/lib --version=v1.2.3 --workspace=/path/to/workspace --max-depth=3
  cascade manifest generate --module-path=github.com/example/lib --version=v1.2.3 --yes --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManifestGenerate(moduleName, modulePath, repository, version, outputPath, dependents, slackChannel, webhook, force, yes, nonInteractive, workspace, maxDepth, includePatterns, excludePatterns)
		},
	}

	// Required flags
	cmd.Flags().StringVar(&modulePath, "module-path", "", "Go module path (e.g., github.com/example/lib) [required]")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3, latest, or omit for local resolution)")

	// Optional flags
	cmd.Flags().StringVar(&moduleName, "module-name", "", "Human-friendly module name (defaults to basename of module path)")
	cmd.Flags().StringVar(&repository, "repository", "", "GitHub repository (defaults to module path without domain)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output file path (default: deps.yaml or workspace manifest path)")
	cmd.Flags().StringSliceVar(&dependents, "dependents", []string{}, "Dependent repositories (format: owner/repo). If omitted, discovers dependents in workspace")
	cmd.Flags().StringVar(&slackChannel, "slack-channel", "", "Default Slack notification channel")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Default webhook URL for notifications")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing manifest without prompting")
	cmd.Flags().BoolVar(&yes, "yes", false, "Automatically confirm all prompts")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Run in non-interactive mode (same as --yes)")

	// Workspace discovery flags
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory to scan for dependents (default: config workspace or current directory)")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "Maximum depth to scan in workspace directory (0 = no limit)")
	cmd.Flags().StringSliceVar(&includePatterns, "include", []string{}, "Directory patterns to include during discovery")
	cmd.Flags().StringSliceVar(&excludePatterns, "exclude", []string{}, "Directory patterns to exclude during discovery (e.g., vendor, .git)")

	// Mark required flags
	cmd.MarkFlagRequired("module-path")
	// version is no longer required as it can be resolved automatically

	return cmd
}

// Command implementations

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
		manifestPath = "deps.yaml" // Default fallback
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

func runManifestGenerate(moduleName, modulePath, repository, version, outputPath string, dependents []string, slackChannel, webhook string, force, yes, nonInteractive bool, workspace string, maxDepth int, includePatterns, excludePatterns []string) error {
	start := time.Now()
	ctx := context.Background()
	logger := container.Logger()
	cfg := container.Config()

	defer func() {
		if logger != nil {
			logger.Debug("Manifest generate command completed",
				"duration_ms", time.Since(start).Milliseconds(),
				"module_path", modulePath,
				"version", version,
				"output", outputPath,
				"dry_run", cfg.Executor.DryRun,
			)
		}
	}()

	// Derive defaults from module path
	if moduleName == "" {
		moduleName = deriveModuleName(modulePath)
	}
	if repository == "" {
		repository = deriveRepository(modulePath)
	}

	// Resolve version if not provided or if "latest" specified
	finalVersion := version
	var versionWarnings []string
	if finalVersion == "" || finalVersion == "latest" {
		workspaceDir := resolveWorkspaceDir(workspace, cfg)
		resolvedVersion, warnings, err := resolveVersionFromWorkspace(ctx, modulePath, finalVersion, workspaceDir, logger)
		if err != nil {
			if finalVersion == "" {
				return newValidationError("version resolution failed and no explicit version provided", err)
			} else {
				return newValidationError("latest version resolution failed", err)
			}
		}
		finalVersion = resolvedVersion
		versionWarnings = warnings
	}

	// Resolve output path
	finalOutputPath := resolveGenerateOutputPath(outputPath, cfg)

	// Resolve workspace discovery options if dependents not explicitly provided
	finalDependents := dependents
	var discoveredDependents []manifest.DependentOptions
	workspaceDir := ""

	if len(dependents) == 0 {
		workspaceDir = resolveWorkspaceDir(workspace, cfg)
		var err error
		discoveredDependents, err = discoverWorkspaceDependents(ctx, modulePath, workspaceDir, maxDepth, includePatterns, excludePatterns, logger)
		if err != nil {
			logger.Warn("Workspace discovery failed, proceeding with empty dependents list", "error", err)
		} else {
			finalDependents = dependentsOptionsToStrings(discoveredDependents)
			if len(finalDependents) > 0 {
				logger.Info("Discovered dependents in workspace",
					"count", len(finalDependents),
					"workspace", workspaceDir,
					"dependents", finalDependents)
			}
		}
	}

	// Display discovery summary and handle confirmation
	if err := displayDiscoverySummary(modulePath, finalVersion, workspaceDir, discoveredDependents, finalDependents, versionWarnings, yes, nonInteractive, cfg.Executor.DryRun); err != nil {
		return err
	}

	logger.Info("Generating dependency manifest",
		"module", modulePath,
		"version", finalVersion,
		"output", finalOutputPath)

	// Build generate options
	options := manifest.GenerateOptions{
		ModuleName:        moduleName,
		ModulePath:        modulePath,
		Repository:        repository,
		Version:           finalVersion,
		Dependents:        buildDependentOptions(finalDependents),
		DefaultBranch:     "main",
		DefaultLabels:     []string{"automation:cascade"},
		DefaultCommitTmpl: "chore(deps): bump {{ .Module }} to {{ .Version }}",
		DefaultTests: []manifest.Command{
			{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}},
		},
		DefaultNotifications: manifest.Notifications{
			SlackChannel: slackChannel,
			Webhook:      webhook,
		},
		DefaultPRConfig: manifest.PRConfig{
			TitleTemplate: "chore(deps): bump {{ .Module }} to {{ .Version }}",
			BodyTemplate:  "Automated dependency update for {{ .Module }} to {{ .Version }}",
		},
	}

	// Generate manifest
	generator := container.ManifestGenerator()
	generatedManifest, err := generator.Generate(ctx, options)
	if err != nil {
		return newValidationError("failed to generate manifest", err)
	}

	// Serialize to YAML
	yamlData, err := yaml.Marshal(generatedManifest)
	if err != nil {
		return newConfigError("failed to serialize manifest to YAML", err)
	}

	// Handle dry-run vs actual file writing
	if cfg.Executor.DryRun {
		fmt.Printf("DRY RUN: Would write manifest to %s\n", finalOutputPath)
		fmt.Printf("--- Generated Manifest ---\n%s", string(yamlData))
		return nil
	}

	// Check for existing file and handle overwrite logic
	if _, err := os.Stat(finalOutputPath); err == nil {
		if !force {
			fmt.Printf("File %s already exists. Overwrite? [y/N]: ", finalOutputPath)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" && response != "YES" {
				fmt.Println("Manifest generation cancelled.")
				return nil
			}
		} else {
			logger.Info("Overwriting existing manifest with --force flag", "path", finalOutputPath)
		}
	}

	// Write to file
	if err := os.WriteFile(finalOutputPath, yamlData, 0644); err != nil {
		return newFileError("failed to write manifest file", err)
	}

	fmt.Printf("Manifest generated successfully: %s\n", finalOutputPath)
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

// Helper functions for manifest generation

// deriveModuleName extracts the module name from the module path
func deriveModuleName(modulePath string) string {
	if modulePath == "" {
		return ""
	}
	// Extract the last part after the final slash
	parts := strings.Split(modulePath, "/")
	return parts[len(parts)-1]
}

// deriveRepository converts module path to repository format for known hosts
func deriveRepository(modulePath string) string {
	if modulePath == "" {
		return ""
	}

	// For common hosting providers, extract the repository part (owner/repo)
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 3 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			return strings.Join(parts[1:3], "/")
		}
	}

	// For non-hosted URLs or unknown hosts, preserve the original module path
	// This prevents breaking URLs like go.uber.org/zap into invalid repository names
	return modulePath
}

// deriveLocalModulePath calculates the relative path from repository root to module
func deriveLocalModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 4 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			// For hosted repos, everything after org/repo is the local path
			return strings.Join(parts[3:], "/")
		}
	}
	// For non-hosted URLs or short paths, default to root
	// This handles cases like go.uber.org/zap where the entire URL is the "repository"
	return "."
}

// buildDependentOptions converts string list to DependentOptions slice
func buildDependentOptions(dependents []string) []manifest.DependentOptions {
	if len(dependents) == 0 {
		return []manifest.DependentOptions{}
	}

	options := make([]manifest.DependentOptions, len(dependents))
	for i, dep := range dependents {
		// Handle format: owner/repo or full module path
		repo := strings.TrimSpace(dep)
		modulePath := ""

		// If it looks like a GitHub repository, convert to module path
		if strings.Count(repo, "/") == 1 && !strings.Contains(repo, ".") {
			modulePath = "github.com/" + repo
		} else {
			modulePath = repo
			repo = deriveRepository(repo)
		}

		options[i] = manifest.DependentOptions{
			Repository:      repo,
			ModulePath:      modulePath,
			LocalModulePath: deriveLocalModulePath(modulePath),
		}
	}

	return options
}

// resolveGenerateOutputPath determines where to write the generated manifest
func resolveGenerateOutputPath(outputPath string, cfg *config.Config) string {
	// Use explicit output path if provided
	if outputPath != "" {
		if !filepath.IsAbs(outputPath) {
			if abs, err := filepath.Abs(outputPath); err == nil {
				return abs
			}
		}
		return outputPath
	}

	// Use config workspace manifest path
	if cfg != nil && cfg.Workspace.ManifestPath != "" {
		return cfg.Workspace.ManifestPath
	}

	// Default to current directory deps.yaml
	if abs, err := filepath.Abs("deps.yaml"); err == nil {
		return abs
	}

	return "deps.yaml"
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

// resolveManifestPath determines the manifest path respecting CLI input, config defaults, and workspace.
func resolveManifestPath(manifestPath string, cfg *config.Config) string {
	path := strings.TrimSpace(manifestPath)
	if path != "" {
		if !filepath.IsAbs(path) {
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
		}
		return path
	}

	if cfg != nil {
		if candidate := strings.TrimSpace(cfg.Workspace.ManifestPath); candidate != "" {
			return candidate
		}
		if base := strings.TrimSpace(cfg.Workspace.Path); base != "" {
			return filepath.Join(base, "deps.yaml")
		}
	}

	if abs, err := filepath.Abs("deps.yaml"); err == nil {
		return abs
	}

	return ""
}

// ensureWorkspace guarantees the workspace directory exists before execution.
func ensureWorkspace(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("workspace path is empty")
	}

	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve workspace path: %w", err)
		}
		path = abs
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create workspace directory: %w", err)
	}

	return nil
}

// resolveModuleVersion extracts the module/version pair either from CLI state identifier or config.
func resolveModuleVersion(stateID string, cfg *config.Config) (string, string, error) {
	if trimmed := strings.TrimSpace(stateID); trimmed != "" {
		parts := splitModuleVersion(trimmed)
		if parts == nil {
			return "", "", fmt.Errorf("state identifier must be in module@version format: %s", stateID)
		}
		return parts[0], parts[1], nil
	}

	if cfg == nil {
		return "", "", fmt.Errorf("module and version must be provided via flags or state identifier")
	}

	module := strings.TrimSpace(cfg.Module)
	version := strings.TrimSpace(cfg.Version)
	if module == "" || version == "" {
		return "", "", fmt.Errorf("module and version must be provided via --module and --version flags or state identifier")
	}

	return module, version, nil
}

// printResumeSummary reports the work items that would be processed during a dry-run resume.
func printResumeSummary(module, version string, itemStates []state.ItemState, plan *planner.Plan) {
	fmt.Printf("DRY RUN: Would resume cascade for %s@%s\n", module, version)
	if plan == nil || len(plan.Items) == 0 {
		fmt.Println("No work items available in regenerated plan")
		return
	}

	stateByRepo := make(map[string]state.ItemState, len(itemStates))
	for _, st := range itemStates {
		stateByRepo[st.Repo] = st
	}

	fmt.Printf("Plan contains %d work items:\n", len(plan.Items))
	for i, item := range plan.Items {
		status := "pending"
		reason := ""
		if st, ok := stateByRepo[item.Repo]; ok {
			if st.Status != "" {
				status = string(st.Status)
			}
			reason = st.Reason
		}
		fmt.Printf("  %d. %s (%s) -> %s [%s]", i+1, item.Repo, item.Module, item.BranchName, status)
		if strings.TrimSpace(reason) != "" {
			fmt.Printf(" - %s", reason)
		}
		fmt.Println()
	}
}

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

			if _, notifyErr := broker.Notify(ctx, item, result); notifyErr != nil {
				errs = append(errs, fmt.Errorf("broker notify: %w", notifyErr))
				itemState.Reason = appendReason(itemState.Reason, fmt.Sprintf("notification failed: %v", notifyErr))
			}
		}
	}

	return itemState, errors.Join(errs...)
}

// stateTracker persists per-item state and run summary updates during orchestration.
type stateTracker struct {
	module   string
	version  string
	summary  *state.Summary
	manager  state.Manager
	logger   di.Logger
	existing map[string]state.ItemState
}

func newStateTracker(module, version string, summary *state.Summary, manager state.Manager, logger di.Logger, existing []state.ItemState) *stateTracker {
	if summary == nil {
		summary = &state.Summary{
			Module:    module,
			Version:   version,
			StartTime: time.Now(),
		}
	} else {
		if summary.Module == "" {
			summary.Module = module
		}
		if summary.Version == "" {
			summary.Version = version
		}
		if summary.StartTime.IsZero() {
			summary.StartTime = time.Now()
		}
	}

	tracker := &stateTracker{
		module:   module,
		version:  version,
		summary:  summary,
		manager:  manager,
		logger:   logger,
		existing: make(map[string]state.ItemState, len(existing)),
	}

	for _, st := range existing {
		tracker.existing[st.Repo] = st
	}

	tracker.saveSummary()
	return tracker
}

func (t *stateTracker) record(item state.ItemState) {
	if t == nil || item.Repo == "" {
		return
	}

	prev, hasPrev := t.existing[item.Repo]
	if hasPrev {
		if item.Attempts <= prev.Attempts {
			item.Attempts = prev.Attempts + 1
		}
		if item.PRURL == "" {
			item.PRURL = prev.PRURL
		}
	}

	if item.Attempts == 0 {
		item.Attempts = 1
	}
	if item.LastUpdated.IsZero() {
		item.LastUpdated = time.Now()
	}

	t.existing[item.Repo] = item
	replaced := false
	for i := range t.summary.Items {
		if t.summary.Items[i].Repo == item.Repo {
			t.summary.Items[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		t.summary.Items = append(t.summary.Items, item)
	}

	t.summary.EndTime = item.LastUpdated
	if t.manager != nil {
		if err := t.manager.SaveItemState(t.module, t.version, item); err != nil && t.logger != nil {
			t.logger.Warn("failed to persist item state", "repo", item.Repo, "error", err)
		}
	}

	t.saveSummary()
}

func (t *stateTracker) saveSummary() {
	if t == nil || t.manager == nil || t.summary == nil {
		return
	}

	if err := t.manager.SaveSummary(t.summary); err != nil && t.logger != nil {
		t.logger.Warn("failed to persist run summary", "module", t.module, "version", t.version, "error", err)
	}
}

func (t *stateTracker) finalize() {
	if t == nil {
		return
	}

	t.summary.EndTime = time.Now()
	t.saveSummary()
}

// runGitCommand executes a git subcommand using the provided runner.
func runGitCommand(ctx context.Context, runner execpkg.GitCommandRunner, repoPath string, args ...string) error {
	if runner == nil {
		return fmt.Errorf("git command runner not configured")
	}
	if len(args) == 0 {
		return fmt.Errorf("git command requires arguments")
	}
	_, err := runner.Run(ctx, repoPath, args...)
	return err
}

// extractPRNumber parses a pull request URL and extracts the numeric identifier.
func extractPRNumber(prURL string) (int, error) {
	parsed, err := url.Parse(prURL)
	if err != nil {
		return 0, fmt.Errorf("invalid PR URL: %w", err)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 0 {
		return 0, fmt.Errorf("no path segments in PR URL: %s", prURL)
	}
	num, err := strconv.Atoi(segments[len(segments)-1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse PR number from URL %s: %w", prURL, err)
	}
	return num, nil
}

// appendReason concatenates reason strings with a delimiter while avoiding duplicates.
func appendReason(existing, addition string) string {
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return strings.TrimSpace(existing)
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}

// resolveWorkspaceDir determines the workspace directory for discovery
func resolveWorkspaceDir(workspace string, cfg *config.Config) string {
	// Use explicit workspace if provided
	if workspace != "" {
		if !filepath.IsAbs(workspace) {
			if abs, err := filepath.Abs(workspace); err == nil {
				return abs
			}
		}
		return workspace
	}

	// Use config workspace path
	if cfg != nil && cfg.Workspace.Path != "" {
		return cfg.Workspace.Path
	}

	// Default to current directory
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	return "."
}

// discoverWorkspaceDependents uses the workspace discovery to find dependent modules
func discoverWorkspaceDependents(ctx context.Context, targetModule, workspaceDir string, maxDepth int, includePatterns, excludePatterns []string, logger di.Logger) ([]manifest.DependentOptions, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	options := manifest.DiscoveryOptions{
		WorkspaceDir:    workspaceDir,
		TargetModule:    targetModule,
		MaxDepth:        maxDepth,
		IncludePatterns: includePatterns,
		ExcludePatterns: excludePatterns,
	}

	dependents, err := discovery.DiscoverDependents(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("workspace discovery failed: %w", err)
	}

	if logger != nil {
		logger.Debug("Workspace discovery completed",
			"target_module", targetModule,
			"workspace", workspaceDir,
			"found_dependents", len(dependents),
			"max_depth", maxDepth)
	}

	return dependents, nil
}

// dependentsOptionsToStrings converts DependentOptions to string slice for CLI compatibility
func dependentsOptionsToStrings(dependents []manifest.DependentOptions) []string {
	if len(dependents) == 0 {
		return []string{}
	}

	result := make([]string, len(dependents))
	for i, dep := range dependents {
		// Use repository format (owner/repo) for CLI compatibility
		result[i] = dep.Repository
	}

	return result
}

// resolveVersionFromWorkspace resolves the module version using workspace discovery
func resolveVersionFromWorkspace(ctx context.Context, modulePath, version, workspaceDir string, logger di.Logger) (string, []string, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	var strategy manifest.VersionResolutionStrategy
	allowNetwork := true

	if version == "latest" {
		strategy = manifest.VersionResolutionLatest
	} else {
		// Auto strategy: try local first, then network
		strategy = manifest.VersionResolutionAuto
	}

	options := manifest.VersionResolutionOptions{
		WorkspaceDir:       workspaceDir,
		TargetModule:       modulePath,
		Strategy:           strategy,
		AllowNetworkAccess: allowNetwork,
	}

	resolution, err := discovery.ResolveVersion(ctx, options)
	if err != nil {
		return "", nil, err
	}

	// Log resolution source
	if logger != nil {
		logger.Info("Version resolved",
			"module", modulePath,
			"version", resolution.Version,
			"source", string(resolution.Source),
			"source_path", resolution.SourcePath)
	}

	return resolution.Version, resolution.Warnings, nil
}

// displayDiscoverySummary shows the discovery results and handles user confirmation
func displayDiscoverySummary(modulePath, version, workspaceDir string, discoveredDependents []manifest.DependentOptions, finalDependents, versionWarnings []string, yes, nonInteractive, dryRun bool) error {
	// Always show summary if discovery was performed or if we have dependents
	shouldShowSummary := workspaceDir != "" || len(finalDependents) > 0

	if !shouldShowSummary {
		return nil
	}

	// Display summary
	fmt.Printf("Generating manifest for %s@%s\n", modulePath, version)

	if workspaceDir != "" {
		fmt.Printf("Discovery workspace: %s\n", workspaceDir)
	}

	if len(discoveredDependents) > 0 {
		fmt.Printf("Discovered %d dependent repositories:\n", len(discoveredDependents))
		for i, dep := range discoveredDependents {
			fmt.Printf("  %d. %s (module: %s)\n", i+1, dep.Repository, dep.ModulePath)
		}
	} else if len(finalDependents) > 0 {
		fmt.Printf("Using %d configured dependent repositories:\n", len(finalDependents))
		for i, dep := range finalDependents {
			fmt.Printf("  %d. %s\n", i+1, dep)
		}
	} else {
		fmt.Println("No dependent repositories found or configured.")
	}

	// Show version warnings if any
	if len(versionWarnings) > 0 {
		fmt.Println("\nVersion Resolution Warnings:")
		for _, warning := range versionWarnings {
			fmt.Printf("  ! %s\n", warning)
		}
	}

	// Default test commands that will be applied
	fmt.Println("\nDefault configurations:")
	fmt.Println("  Branch: main")
	fmt.Println("  Labels: [automation:cascade]")
	fmt.Println("  Test commands: go test ./... -race -count=1")
	fmt.Println("  Commit template: chore(deps): bump {{ .Module }} to {{ .Version }}")
	fmt.Println("  PR title: chore(deps): bump {{ .Module }} to {{ .Version }}")

	// Handle confirmation unless in dry-run mode, yes flag, or non-interactive mode
	if !dryRun && !yes && !nonInteractive {
		fmt.Printf("\nProceed with manifest generation? [Y/n]: ")
		var response string
		fmt.Scanln(&response)

		// Default to yes if empty response, check for explicit no
		if response != "" && (response == "n" || response == "N" || response == "no" || response == "NO") {
			fmt.Println("Manifest generation cancelled.")
			return fmt.Errorf("manifest generation cancelled by user")
		}
	}

	if dryRun {
		fmt.Println("\n--- DRY RUN: Would proceed with manifest generation ---")
	} else if yes || nonInteractive {
		fmt.Println("\n--- Proceeding with manifest generation ---")
	}

	return nil
}
