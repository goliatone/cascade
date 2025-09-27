package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
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
	"github.com/goliatone/cascade/pkg/version"
	gh "github.com/google/go-github/v66/github"
	oauth2 "golang.org/x/oauth2"
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

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print Cascade version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return version.Print(cmd.OutOrStdout())
		},
	}
}

// newRootCommand creates the root cobra command with all subcommands
func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "cascade",
		Short:   "Cascade orchestrates automated dependency updates across Go repositories",
		Version: version.GetVersion(),
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
  cascade release --manifest=.cascade.yaml --dry-run
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
		newVersionCommand(),
	)

	return cmd
}

// initializeContainer sets up the dependency injection container with configuration
func initializeContainer(cmd *cobra.Command) error {
	start := time.Now()
	// Build configuration from flags, environment, and files
	// First extract config file path from flags if provided
	var configFile string
	if cmd.Flags().Changed("config") {
		configFile, _ = cmd.Flags().GetString("config")
	}

	builder := config.NewBuilder().
		FromFile(configFile). // Use explicit config file or auto-discover
		FromEnv().            // Load from environment
		FromFlags(cmd)        // Load from command flags (highest precedence)

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
		// GitHub discovery flags
		githubOrg             string
		githubIncludePatterns []string
		githubExcludePatterns []string
	)

	cmd := &cobra.Command{
		Use:     "generate",
		Aliases: []string{"gen"},
		Short:   "Generate a new dependency manifest",
		Long: `Generate creates a new dependency manifest file with intelligent defaults.
The command automatically detects module information from the current directory's go.mod
file and version from .version file or git tags when not explicitly provided.

Smart Defaults:
  - Module path: Auto-detected from go.mod in current directory tree
  - Version: Auto-detected from .version file, VERSION file, or latest git tag
  - Output file: .cascade.yaml (non-conflicting default)
  - GitHub org: Extracted from module path for GitHub.com modules
  - Workspace: Intelligently detected from module location (parent dir, $GOPATH/src/org/, etc.)

When --dependents is omitted, cascade will automatically discover dependent repositories
by scanning the workspace for Go modules that import the target module.

The command will display a summary of discovered dependents and default configurations
before proceeding. Use --yes or --non-interactive to skip confirmation prompts.

Examples:
  cascade manifest generate                                                    # Use all auto-detected defaults
  cascade manifest generate --version=v1.2.3                                 # Override just the version
  cascade manifest generate --output=.cascade.yaml                               # Custom output file
  cascade manifest generate --dependents=owner/repo1,owner/repo2             # Explicit dependents
  cascade manifest generate --workspace=/path/to/workspace --max-depth=3     # Custom workspace discovery
  cascade manifest generate --yes --dry-run                                  # Non-interactive dry run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runManifestGenerate(moduleName, modulePath, repository, version, outputPath, dependents, slackChannel, webhook, force, yes, nonInteractive, workspace, maxDepth, includePatterns, excludePatterns, githubOrg, githubIncludePatterns, githubExcludePatterns)
		},
	}

	// Module and version flags (auto-detected if not provided)
	cmd.Flags().StringVar(&modulePath, "module-path", "", "Go module path (e.g., github.com/example/lib). Auto-detected from go.mod if not provided")
	cmd.Flags().StringVar(&version, "version", "", "Target version (e.g., v1.2.3, latest). Auto-detected from .version file or git tags if not provided")

	// Optional configuration flags
	cmd.Flags().StringVar(&moduleName, "module-name", "", "Human-friendly module name (defaults to basename of module path)")
	cmd.Flags().StringVar(&repository, "repository", "", "GitHub repository (defaults to module path without domain)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output file path (default: .cascade.yaml)")
	cmd.Flags().StringSliceVar(&dependents, "dependents", []string{}, "Dependent repositories (format: owner/repo). If omitted, discovers dependents in workspace")
	cmd.Flags().StringVar(&slackChannel, "slack-channel", "", "Default Slack notification channel")
	cmd.Flags().StringVar(&webhook, "webhook", "", "Default webhook URL for notifications")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing manifest without prompting")
	cmd.Flags().BoolVar(&yes, "yes", false, "Automatically confirm all prompts")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "Run in non-interactive mode (same as --yes)")

	// Workspace discovery flags
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory to scan for dependents (default: auto-detected from module location)")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 0, "Maximum depth to scan in workspace directory (0 = no limit)")
	cmd.Flags().StringSliceVar(&includePatterns, "include", []string{}, "Directory patterns to include during discovery")
	cmd.Flags().StringSliceVar(&excludePatterns, "exclude", []string{}, "Directory patterns to exclude during discovery (e.g., vendor, .git)")

	// GitHub discovery flags
	cmd.Flags().StringVar(&githubOrg, "github-org", "", "GitHub organization to search for dependent repositories (auto-detected from module path if not provided)")
	cmd.Flags().StringSliceVar(&githubIncludePatterns, "github-include", []string{}, "Repository name patterns to include during GitHub discovery")
	cmd.Flags().StringSliceVar(&githubExcludePatterns, "github-exclude", []string{}, "Repository name patterns to exclude during GitHub discovery")

	// No required flags - all values can be auto-detected or have sensible defaults

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

func runManifestGenerate(moduleName, modulePath, repository, version, outputPath string, dependents []string, slackChannel, webhook string, force, yes, nonInteractive bool, workspace string, maxDepth int, includePatterns, excludePatterns []string, githubOrg string, githubIncludePatterns, githubExcludePatterns []string) error {
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

	// Detect module information when not explicitly provided
	finalModulePath := strings.TrimSpace(modulePath)
	moduleDir := ""
	if autoModulePath, autoModuleDir, err := detectModuleInfo(); err == nil {
		moduleDir = autoModuleDir
		if finalModulePath == "" {
			finalModulePath = autoModulePath
		}
	} else if finalModulePath == "" {
		return newValidationError("module path must be provided or go.mod must be present in the current directory", err)
	}
	modulePath = finalModulePath

	// Derive defaults from module path
	if moduleName == "" {
		moduleName = deriveModuleName(modulePath)
	}
	if repository == "" {
		repository = deriveRepository(modulePath)
	}
	if githubOrg == "" {
		githubOrg = deriveGitHubOrgFromModule(modulePath)
	}

	// Resolve version if not provided or if "latest" specified
	finalVersion := strings.TrimSpace(version)
	var versionWarnings []string
	if finalVersion == "" {
		detectedVersion, warnings := detectDefaultVersion(ctx, moduleDir)
		versionWarnings = append(versionWarnings, warnings...)
		finalVersion = detectedVersion
	}
	if finalVersion == "" || strings.EqualFold(finalVersion, "latest") {
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

	version = finalVersion

	// Resolve output path
	finalOutputPath := resolveGenerateOutputPath(outputPath, cfg)
	outputPath = finalOutputPath

	// Resolve discovery options if dependents not explicitly provided
	var discoveredDependents []manifest.DependentOptions
	workspaceDir := ""
	finalDependentOptions := []manifest.DependentOptions{}

	if len(dependents) == 0 {
		workspaceDir = resolveWorkspaceDir(workspace, cfg)
		mergedDependents, err := performMultiSourceDiscovery(ctx, modulePath, githubOrg, workspaceDir, maxDepth,
			includePatterns, excludePatterns, githubIncludePatterns, githubExcludePatterns, cfg, logger)
		if err != nil {
			logger.Warn("Discovery failed, proceeding with empty dependents list", "error", err)
		} else {
			discoveredDependents = mergedDependents
			finalDependentOptions = append(finalDependentOptions, discoveredDependents...)

			if len(discoveredDependents) > 0 {
				logger.Info("Discovery completed",
					"total_dependents", len(discoveredDependents),
					"dependents", dependentsOptionsToStrings(discoveredDependents))
			}
		}

		if len(discoveredDependents) > 0 && !yes && !nonInteractive {
			filteredDependents, err := promptForDependentSelection(discoveredDependents)
			if err != nil {
				return fmt.Errorf("dependent selection failed: %w", err)
			}
			discoveredDependents = filteredDependents
			finalDependentOptions = append([]manifest.DependentOptions{}, discoveredDependents...)
		}
	} else {
		finalDependentOptions = buildDependentOptions(dependents)
	}

	finalDependentNames := dependentsOptionsToStrings(finalDependentOptions)

	// Display discovery summary and handle confirmation
	if err := displayDiscoverySummary(modulePath, finalVersion, workspaceDir, discoveredDependents, finalDependentNames, versionWarnings, yes, nonInteractive, cfg.Executor.DryRun); err != nil {
		return err
	}

	logger.Info("Generating dependency manifest",
		"module", modulePath,
		"version", finalVersion,
		"output", finalOutputPath)

	// Build generate options with config defaults merged
	options := manifest.GenerateOptions{
		ModuleName:        moduleName,
		ModulePath:        modulePath,
		Repository:        repository,
		Version:           finalVersion,
		Dependents:        finalDependentOptions,
		DefaultBranch:     getDefaultBranch(cfg),
		DefaultLabels:     []string{"automation:cascade"},
		DefaultCommitTmpl: "chore(deps): bump {{ .Module }} to {{ .Version }}",
		DefaultTests:      getDefaultTests(cfg),
		DefaultNotifications: manifest.Notifications{
			SlackChannel: getDefaultSlackChannel(slackChannel, cfg),
			Webhook:      getDefaultWebhook(webhook, cfg),
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

// detectModuleInfo walks up from the current working directory to locate a go.mod file
// and returns the module path with the directory that contains it. It enables
// `cascade manifest generate` to infer sensible defaults without requiring flags.
func detectModuleInfo() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("determine working directory: %w", err)
	}

	for dir := cwd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		goModPath := filepath.Join(dir, "go.mod")
		info, err := os.Stat(goModPath)
		if err != nil {
			if os.IsNotExist(err) {
				if parent := filepath.Dir(dir); parent == dir {
					break
				}
				continue
			}
			return "", "", fmt.Errorf("stat go.mod: %w", err)
		}
		if info.IsDir() {
			continue
		}

		content, err := os.ReadFile(goModPath)
		if err != nil {
			return "", "", fmt.Errorf("read go.mod: %w", err)
		}
		modulePath := parseGoModModulePath(string(content))
		if modulePath == "" {
			return "", "", fmt.Errorf("module declaration not found in %s", goModPath)
		}
		return modulePath, dir, nil
	}

	return "", "", fmt.Errorf("go.mod not found in current tree")
}

// detectDefaultVersion inspects common local sources for a module version.
// Priority: `.version` file (or `VERSION`), then latest annotated tag in git.
// Returns any warnings encountered while probing so the CLI can surface them.
func detectDefaultVersion(ctx context.Context, moduleDir string) (string, []string) {
	var warnings []string

	if strings.TrimSpace(moduleDir) == "" {
		return "", warnings
	}

	// Look for marker files first so projects can override without git metadata.
	versionFiles := []string{filepath.Join(moduleDir, ".version"), filepath.Join(moduleDir, "VERSION")}
	for _, candidate := range versionFiles {
		data, err := os.ReadFile(candidate)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("failed to read %s: %v", candidate, err))
			}
			continue
		}
		if v := normalizeVersionString(string(data)); v != "" {
			return v, warnings
		}
	}

	// Fallback to git tags if repository information is available.
	if _, err := os.Stat(filepath.Join(moduleDir, ".git")); err == nil {
		cmd := exec.CommandContext(ctx, "git", "-C", moduleDir, "describe", "--tags", "--abbrev=0")
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		output, err := cmd.Output()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("git tag detection failed: %v", err))
		} else if v := normalizeVersionString(string(output)); v != "" {
			return v, warnings
		}
	} else if !os.IsNotExist(err) {
		warnings = append(warnings, fmt.Sprintf("git metadata unavailable: %v", err))
	}

	return "", warnings
}

// normalizeVersionString trims whitespace and ensures versions have the expected
// leading "v" when the underlying data omits it.
func normalizeVersionString(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	if trimmed[0] >= '0' && trimmed[0] <= '9' {
		return "v" + trimmed
	}
	return trimmed
}

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

// deriveGitHubOrgFromModule extracts the GitHub organization from a module path when available.
func deriveGitHubOrgFromModule(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 3 && parts[0] == "github.com" {
		return parts[1]
	}
	return ""
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

	// Default to hidden manifest in current directory to avoid clobbering existing files
	if abs, err := filepath.Abs(".cascade.yaml"); err == nil {
		return abs
	}

	return ".cascade.yaml"
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
			return filepath.Join(base, ".cascade.yaml")
		}
	}

	if abs, err := filepath.Abs(".cascade.yaml"); err == nil {
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

// resolveWorkspaceDir determines the workspace directory for discovery with intelligent defaults
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

	// Use manifest generator default workspace
	if cfg != nil && cfg.ManifestGenerator.DefaultWorkspace != "" {
		return cfg.ManifestGenerator.DefaultWorkspace
	}

	// workspace detection based on current module location
	if intelligentWorkspace := detectIntelligentWorkspace(); intelligentWorkspace != "" {
		// TODO: Add logging here when logger is available
		return intelligentWorkspace
	}

	// Fallback to $HOME/.cache/cascade for isolation
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "cascade")
	}

	// Last resort: current working directory
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	return "."
}

// detectIntelligentWorkspace attempts to detect a sensible workspace directory based on
// the current module's location and Go environment. It tries to find a directory that
// likely contains other Go modules that might depend on the current module.
func detectIntelligentWorkspace() string {
	// Get current module information
	modulePath, moduleDir, err := detectModuleInfo()
	if err != nil {
		return ""
	}

	// 1) use parent directory of current module if it contains multiple Go modules
	// e.g., ~/Development/GO/src/github.com/goliatone/go-errors -> ~/Development/GO/src/github.com/goliatone/
	if parentWorkspace := detectParentWorkspace(moduleDir, modulePath); parentWorkspace != "" {
		return parentWorkspace
	}

	// 2) check GOPATH/src/{hosting}/{org}/ directory
	// e.g., github.com/goliatone/go-errors -> $GOPATH/src/github.com/goliatone/
	if gopathOrgWorkspace := detectGopathOrgWorkspace(modulePath); gopathOrgWorkspace != "" {
		return gopathOrgWorkspace
	}

	// 3) check GOPATH/src/ directory for broader discovery
	if gopathWorkspace := detectGopathWorkspace(); gopathWorkspace != "" {
		return gopathWorkspace
	}

	return ""
}

// detectParentWorkspace checks if the parent directories of the current module
// contain other Go modules, indicating this is a multi-module workspace
func detectParentWorkspace(moduleDir, modulePath string) string {
	if moduleDir == "" {
		return ""
	}

	// Extract organization from module path (e.g., "goliatone" from "github.com/goliatone/go-errors")
	org := extractOrgFromModulePath(modulePath)
	if org == "" {
		return ""
	}

	// Walk up the directory tree looking for a directory that contains multiple Go modules
	current := moduleDir
	for i := 0; i < 5; i++ { // Limit traversal to avoid going too far up
		parent := filepath.Dir(current)
		if parent == current || parent == "/" || parent == "." {
			break
		}

		// Check if this directory name matches the organization
		if filepath.Base(parent) == org {
			// Validate this directory contains multiple Go modules
			if isValidWorkspace(parent) {
				return parent
			}
		}

		// Also check if parent contains multiple modules (even if not named after org)
		if isValidWorkspace(parent) && containsMultipleModules(parent) {
			return parent
		}

		current = parent
	}

	return ""
}

// detectGopathOrgWorkspace checks $GOPATH/src/{hosting}/{org}/ for a workspace
func detectGopathOrgWorkspace(modulePath string) string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Try default GOPATH
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}

	// Parse module path to extract hosting and org
	// e.g., github.com/goliatone/go-errors -> hosting=github.com, org=goliatone
	parts := strings.Split(modulePath, "/")
	if len(parts) < 3 {
		return ""
	}

	hosting := parts[0]
	org := parts[1]

	// Check $GOPATH/src/{hosting}/{org}/
	orgPath := filepath.Join(gopath, "src", hosting, org)
	if isValidWorkspace(orgPath) {
		return orgPath
	}

	return ""
}

// detectGopathWorkspace checks $GOPATH/src/ as a broader workspace
func detectGopathWorkspace() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		// Try default GOPATH
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}

	srcPath := filepath.Join(gopath, "src")
	if isValidWorkspace(srcPath) && containsMultipleModules(srcPath) {
		return srcPath
	}

	return ""
}

// extractOrgFromModulePath extracts the organization from a module path
func extractOrgFromModulePath(modulePath string) string {
	parts := strings.Split(modulePath, "/")
	if len(parts) >= 2 {
		switch parts[0] {
		case "github.com", "gitlab.com", "bitbucket.org":
			return parts[1]
		}
	}
	return ""
}

// isValidWorkspace checks if a directory exists and is readable
func isValidWorkspace(dir string) bool {
	if dir == "" {
		return false
	}

	info, err := os.Stat(dir)
	if err != nil {
		return false
	}

	return info.IsDir()
}

// containsMultipleModules checks if a directory contains multiple Go modules
func containsMultipleModules(dir string) bool {
	moduleCount := 0
	maxCheck := 50 // Limit to avoid scanning huge directories

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Stop if we've checked too many entries
		if moduleCount >= maxCheck {
			return filepath.SkipDir
		}

		// Skip deep nested directories
		if strings.Count(strings.TrimPrefix(path, dir), string(filepath.Separator)) > 3 {
			return filepath.SkipDir
		}

		// Skip common non-module directories
		base := filepath.Base(path)
		if base == ".git" || base == "vendor" || base == "node_modules" || base == ".cache" {
			return filepath.SkipDir
		}

		if info.Name() == "go.mod" {
			moduleCount++
			if moduleCount >= 2 {
				return filepath.SkipAll // Found multiple modules, we can stop
			}
		}

		return nil
	})

	if err != nil {
		return false
	}

	return moduleCount >= 2
}

// discoverWorkspaceDependents uses the workspace discovery to find dependent modules
func discoverWorkspaceDependents(ctx context.Context, targetModule, workspaceDir string, maxDepth int, includePatterns, excludePatterns []string, cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {
	discovery := manifest.NewWorkspaceDiscovery()

	// Apply config defaults for discovery options
	finalMaxDepth := getDiscoveryMaxDepth(maxDepth, cfg)
	finalIncludePatterns := getDiscoveryIncludePatterns(includePatterns, cfg)
	finalExcludePatterns := getDiscoveryExcludePatterns(excludePatterns, cfg)

	options := manifest.DiscoveryOptions{
		WorkspaceDir:    workspaceDir,
		TargetModule:    targetModule,
		MaxDepth:        finalMaxDepth,
		IncludePatterns: finalIncludePatterns,
		ExcludePatterns: finalExcludePatterns,
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

// Config defaults merging functions for manifest generation

// getDefaultBranch returns the configured default branch or fallback
func getDefaultBranch(cfg *config.Config) string {
	if cfg != nil && cfg.ManifestGenerator.DefaultBranch != "" {
		return cfg.ManifestGenerator.DefaultBranch
	}
	return "main"
}

// getDefaultTests returns the configured default test commands or fallback
func getDefaultTests(cfg *config.Config) []manifest.Command {
	if cfg != nil && cfg.ManifestGenerator.Tests.Command != "" {
		testCmd := manifest.Command{}

		// Parse command - respect the exact command from config
		parts := strings.Fields(cfg.ManifestGenerator.Tests.Command)
		if len(parts) > 0 {
			testCmd.Cmd = parts
		} else {
			// Fallback to shell execution for complex commands
			testCmd.Cmd = []string{"sh", "-c", cfg.ManifestGenerator.Tests.Command}
		}

		if cfg.ManifestGenerator.Tests.WorkingDirectory != "" {
			testCmd.Dir = cfg.ManifestGenerator.Tests.WorkingDirectory
		}

		return []manifest.Command{testCmd}
	}

	// Default fallback
	return []manifest.Command{
		{Cmd: []string{"go", "test", "./...", "-race", "-count=1"}},
	}
}

// getDefaultSlackChannel returns the CLI-provided channel or config default
func getDefaultSlackChannel(cliChannel string, cfg *config.Config) string {
	if cliChannel != "" {
		return cliChannel
	}
	if cfg != nil && len(cfg.ManifestGenerator.Notifications.Channels) > 0 {
		// Use first configured channel as Slack channel
		return cfg.ManifestGenerator.Notifications.Channels[0]
	}
	if cfg != nil && cfg.Integration.Slack.Channel != "" {
		return cfg.Integration.Slack.Channel
	}
	return ""
}

// getDefaultWebhook returns the CLI-provided webhook or config default
func getDefaultWebhook(cliWebhook string, cfg *config.Config) string {
	if cliWebhook != "" {
		return cliWebhook
	}
	if cfg != nil && cfg.Integration.Slack.WebhookURL != "" {
		return cfg.Integration.Slack.WebhookURL
	}
	return ""
}

// getDiscoveryMaxDepth returns the CLI-provided maxDepth or config default
func getDiscoveryMaxDepth(cliMaxDepth int, cfg *config.Config) int {
	if cliMaxDepth > 0 {
		return cliMaxDepth
	}
	if cfg != nil && cfg.ManifestGenerator.Discovery.MaxDepth > 0 {
		return cfg.ManifestGenerator.Discovery.MaxDepth
	}
	return 0 // 0 means unlimited depth
}

// getDiscoveryIncludePatterns returns the CLI-provided patterns or config defaults
func getDiscoveryIncludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.IncludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.IncludePatterns
	}
	return []string{} // Empty means include all
}

// getDiscoveryExcludePatterns returns the CLI-provided patterns or config defaults
func getDiscoveryExcludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.ExcludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.ExcludePatterns
	}
	return []string{"vendor/*", ".git/*", "node_modules/*"} // Sensible defaults
}

// getGitHubIncludePatterns returns the include patterns for GitHub discovery
func getGitHubIncludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns
	}
	return nil
}

// getGitHubExcludePatterns returns the exclude patterns for GitHub discovery
func getGitHubExcludePatterns(cliPatterns []string, cfg *config.Config) []string {
	if len(cliPatterns) > 0 {
		return cliPatterns
	}
	if cfg != nil && len(cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns) > 0 {
		return cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns
	}
	return nil
}

// newGitHubClient constructs a GitHub client using configuration and shared HTTP client settings
func newGitHubClient(ctx context.Context, cfg *config.Config) (*gh.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration required for GitHub discovery")
	}

	token := strings.TrimSpace(cfg.Integration.GitHub.Token)
	if token == "" {
		token = strings.TrimSpace(os.Getenv(config.EnvGitHubToken))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GH_TOKEN"))
	}
	if token == "" {
		return nil, fmt.Errorf("github token required for discovery")
	}

	var baseHTTP *http.Client
	if container != nil {
		baseHTTP = container.HTTPClient()
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthClient := oauth2.NewClient(ctx, ts)

	if baseHTTP != nil {
		if transport, ok := oauthClient.Transport.(*oauth2.Transport); ok {
			if baseHTTP.Transport != nil {
				transport.Base = baseHTTP.Transport
			}
		}
		if baseHTTP.Timeout > 0 {
			oauthClient.Timeout = baseHTTP.Timeout
		}
	}

	endpoint := strings.TrimSpace(cfg.Integration.GitHub.Endpoint)
	if endpoint == "" {
		return gh.NewClient(oauthClient), nil
	}

	baseURL, uploadURL := normalizeEnterpriseEndpoints(endpoint)
	client, err := gh.NewEnterpriseClient(baseURL, uploadURL, oauthClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub enterprise client: %w", err)
	}
	return client, nil
}

// normalizeEnterpriseEndpoints mirrors pkg/di provider logic for GitHub enterprise endpoints
func normalizeEnterpriseEndpoints(endpoint string) (string, string) {
	base := strings.TrimSpace(endpoint)
	if base == "" {
		return "", ""
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	trimmed := strings.TrimSuffix(base, "/")
	if strings.HasSuffix(trimmed, "/api/v3") {
		prefix := strings.TrimSuffix(trimmed, "/api/v3")
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		return prefix + "api/v3/", prefix + "api/uploads/"
	}

	return base, base
}

// matchesRepoPatterns evaluates include/exclude patterns against repository names
func matchesRepoPatterns(repo string, includePatterns, excludePatterns []string) bool {
	repoLower := strings.ToLower(repo)
	repoName := repoLower
	if idx := strings.Index(repoLower, "/"); idx >= 0 {
		repoName = repoLower[idx+1:]
	}

	matchesPattern := func(pattern string) bool {
		pattern = strings.ToLower(pattern)
		if ok, _ := path.Match(pattern, repoLower); ok {
			return true
		}
		if ok, _ := path.Match(pattern, repoName); ok {
			return true
		}
		return false
	}

	for _, pattern := range excludePatterns {
		if matchesPattern(pattern) {
			return false
		}
	}

	if len(includePatterns) == 0 {
		return true
	}

	for _, pattern := range includePatterns {
		if matchesPattern(pattern) {
			return true
		}
	}

	return false
}

// fetchModuleInfoFromGitHub downloads go.mod content and extracts module information
func fetchModuleInfoFromGitHub(ctx context.Context, client *gh.Client, repo *gh.Repository, goModPath string) (string, string, error) {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()

	file, _, resp, err := client.Repositories.GetContents(ctx, owner, name, goModPath, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return "", "", fmt.Errorf("go.mod not found at %s", goModPath)
		}
		return "", "", err
	}

	content, err := file.GetContent()
	if err != nil {
		return "", "", err
	}

	modulePath := parseGoModModulePath(content)
	if modulePath == "" {
		modulePath = fmt.Sprintf("github.com/%s/%s", owner, name)
	}

	localPath := path.Dir(goModPath)
	if localPath == "." || localPath == "/" {
		localPath = "."
	}

	return modulePath, localPath, nil
}

// parseGoModModulePath extracts the module path from go.mod content
func parseGoModModulePath(content string) string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

// resolveGitHubOrg returns the GitHub organization from CLI flag or config
func resolveGitHubOrg(cliOrg string, cfg *config.Config) string {
	// CLI flag takes priority
	if cliOrg != "" {
		return cliOrg
	}

	// Check config for GitHub discovery organization
	if cfg != nil && cfg.ManifestGenerator.Discovery.GitHub.Organization != "" {
		return cfg.ManifestGenerator.Discovery.GitHub.Organization
	}

	// Check config for general GitHub organization (fallback)
	if cfg != nil && cfg.Integration.GitHub.Organization != "" {
		return cfg.Integration.GitHub.Organization
	}

	return ""
}

// discoverGitHubDependents discovers dependent repositories in a GitHub organization
// This is a placeholder implementation for Task 3.2 - actual implementation comes in Task 3.1
func discoverGitHubDependents(ctx context.Context, targetModule, organization string, includePatterns, excludePatterns []string, cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration required for GitHub discovery")
	}

	client, err := newGitHubClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	finalInclude := includePatterns
	finalExclude := excludePatterns

	if len(finalInclude) == 0 {
		finalInclude = cfg.ManifestGenerator.Discovery.GitHub.IncludePatterns
	}
	if len(finalExclude) == 0 {
		finalExclude = cfg.ManifestGenerator.Discovery.GitHub.ExcludePatterns
	}

	return discoverGitHubDependentsWithClient(ctx, client, targetModule, organization, finalInclude, finalExclude, logger)
}

// discoverGitHubDependentsWithClient executes GitHub discovery using a prepared client (primarily for testing)
func discoverGitHubDependentsWithClient(ctx context.Context, client *gh.Client, targetModule, organization string, includePatterns, excludePatterns []string, logger di.Logger) ([]manifest.DependentOptions, error) {
	if client == nil {
		return nil, fmt.Errorf("github client is required")
	}

	query := fmt.Sprintf("org:%s \"%s\" path:go.mod", organization, targetModule)
	options := &gh.SearchOptions{ListOptions: gh.ListOptions{PerPage: 100}}

	dependents := make([]manifest.DependentOptions, 0)
	fetchedRepos := make(map[string]struct{})

	for {
		results, resp, err := client.Search.Code(ctx, query, options)
		if err != nil {
			return nil, fmt.Errorf("github code search failed: %w", err)
		}

		for _, item := range results.CodeResults {
			repo := item.GetRepository()
			fullName := repo.GetFullName()

			if !matchesRepoPatterns(fullName, includePatterns, excludePatterns) {
				continue
			}

			modulePath, localModulePath, err := fetchModuleInfoFromGitHub(ctx, client, repo, item.GetPath())
			if err != nil {
				if logger != nil {
					logger.Warn("Failed to fetch module info from GitHub",
						"repository", fullName,
						"path", item.GetPath(),
						"error", err)
				}
				continue
			}

			key := fmt.Sprintf("%s|%s|%s", fullName, modulePath, localModulePath)
			if _, exists := fetchedRepos[key]; exists {
				continue
			}
			fetchedRepos[key] = struct{}{}

			dependents = append(dependents, manifest.DependentOptions{
				Repository:      fullName,
				ModulePath:      modulePath,
				LocalModulePath: localModulePath,
				DiscoverySource: "github",
			})
		}

		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	return dependents, nil
}

// performMultiSourceDiscovery performs discovery from multiple sources and merges the results.
// This implements Task 3.3: Result Merging & Conflict Resolution.
func performMultiSourceDiscovery(ctx context.Context, targetModule, githubOrg, workspace string, maxDepth int,
	includePatterns, excludePatterns, githubIncludePatterns, githubExcludePatterns []string,
	cfg *config.Config, logger di.Logger) ([]manifest.DependentOptions, error) {

	var githubDependents []manifest.DependentOptions
	var workspaceDependents []manifest.DependentOptions
	var discoveryErrors []error

	// Step 1: Attempt GitHub discovery if organization is specified
	finalGitHubOrg := resolveGitHubOrg(githubOrg, cfg)
	shouldRunGitHub := finalGitHubOrg != ""
	if githubOrg == "" && cfg != nil && !cfg.ManifestGenerator.Discovery.GitHub.Enabled {
		shouldRunGitHub = false
	}
	if shouldRunGitHub {
		finalGitHubInclude := getGitHubIncludePatterns(githubIncludePatterns, cfg)
		finalGitHubExclude := getGitHubExcludePatterns(githubExcludePatterns, cfg)

		if logger != nil {
			logger.Info("Attempting GitHub discovery", "organization", finalGitHubOrg)
		}

		ghDeps, err := discoverGitHubDependents(ctx, targetModule, finalGitHubOrg,
			finalGitHubInclude, finalGitHubExclude, cfg, logger)
		if err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Errorf("GitHub discovery failed: %w", err))
			if logger != nil {
				logger.Warn("GitHub discovery failed", "error", err)
			}
		} else {
			githubDependents = ghDeps
			if logger != nil && len(githubDependents) > 0 {
				logger.Info("GitHub discovery completed",
					"organization", finalGitHubOrg,
					"found_dependents", len(githubDependents))
			}
		}
	}

	// Step 2: Attempt workspace discovery
	workspaceDir := resolveWorkspaceDir(workspace, cfg)
	if workspaceDir != "" {
		if logger != nil {
			logger.Info("Attempting workspace discovery", "workspace", workspaceDir)
		}

		wsDeps, err := discoverWorkspaceDependents(ctx, targetModule, workspaceDir, maxDepth,
			includePatterns, excludePatterns, cfg, logger)
		if err != nil {
			discoveryErrors = append(discoveryErrors, fmt.Errorf("workspace discovery failed: %w", err))
			if logger != nil {
				logger.Warn("Workspace discovery failed", "error", err)
			}
		} else {
			workspaceDependents = wsDeps
			if logger != nil && len(workspaceDependents) > 0 {
				logger.Info("Workspace discovery completed",
					"workspace", workspaceDir,
					"found_dependents", len(workspaceDependents))
			}
		}
	}

	// Step 3: Merge and deduplicate results
	mergedDependents := mergeDiscoveryResults(githubDependents, workspaceDependents, logger)

	if len(mergedDependents) == 0 {
		if len(discoveryErrors) > 0 {
			// Return the first error if no results were found and errors occurred
			return nil, discoveryErrors[0]
		}
		if logger != nil {
			logger.Info("No dependent repositories discovered")
		}
	} else if logger != nil {
		logger.Info("Discovery results merged",
			"github_dependents", len(githubDependents),
			"workspace_dependents", len(workspaceDependents),
			"merged_total", len(mergedDependents))
	}

	return mergedDependents, nil
}

// mergeDiscoveryResults merges and deduplicates discovery results from multiple sources.
// Deduplication is based on repository name and module path pairs.
func mergeDiscoveryResults(githubDependents, workspaceDependents []manifest.DependentOptions, logger di.Logger) []manifest.DependentOptions {
	// Use a map to deduplicate based on repo+module pair
	dependentMap := make(map[string]manifest.DependentOptions)

	// Add workspace dependents first (they may have more accurate local paths)
	for _, dep := range workspaceDependents {
		key := dependentKey(dep.Repository, dep.ModulePath)
		dep.DiscoverySource = "workspace"
		dependentMap[key] = dep
	}

	// Add GitHub dependents, potentially overriding workspace entries
	conflictCount := 0
	for _, dep := range githubDependents {
		key := dependentKey(dep.Repository, dep.ModulePath)
		if existing, exists := dependentMap[key]; exists {
			// Conflict detected - merge the entries, preferring more complete information
			merged := mergeConflictingDependents(existing, dep, logger)
			dependentMap[key] = merged
			conflictCount++
		} else {
			dep.DiscoverySource = "github"
			dependentMap[key] = dep
		}
	}

	// Convert map back to slice
	result := make([]manifest.DependentOptions, 0, len(dependentMap))
	for _, dep := range dependentMap {
		result = append(result, dep)
	}

	if logger != nil && conflictCount > 0 {
		logger.Info("Resolved discovery conflicts",
			"conflicts", conflictCount,
			"final_count", len(result))
	}

	return result
}

// dependentKey creates a unique key for deduplication based on repository and module path.
func dependentKey(repository, modulePath string) string {
	return fmt.Sprintf("%s|%s", repository, modulePath)
}

// mergeConflictingDependents merges two DependentOptions that refer to the same repo/module.
// It prefers more complete information and logs the merge decisions.
func mergeConflictingDependents(existing, new manifest.DependentOptions, logger di.Logger) manifest.DependentOptions {
	merged := existing

	// Prefer non-empty local module paths (workspace discovery usually provides these)
	if merged.LocalModulePath == "." && new.LocalModulePath != "." && new.LocalModulePath != "" {
		merged.LocalModulePath = new.LocalModulePath
	}

	// Track both discovery sources
	if existing.DiscoverySource != "" && new.DiscoverySource != "" {
		merged.DiscoverySource = fmt.Sprintf("%s+%s", existing.DiscoverySource, new.DiscoverySource)
	} else if new.DiscoverySource != "" {
		merged.DiscoverySource = new.DiscoverySource
	}

	if logger != nil {
		logger.Debug("Merged conflicting dependents",
			"repository", merged.Repository,
			"module_path", merged.ModulePath,
			"sources", merged.DiscoverySource,
			"local_module_path", merged.LocalModulePath)
	}

	return merged
}

// promptForDependentSelection allows users to interactively select which discovered
// dependents to include in the manifest.
func promptForDependentSelection(dependents []manifest.DependentOptions) ([]manifest.DependentOptions, error) {
	if len(dependents) == 0 {
		return dependents, nil
	}

	fmt.Printf("\nDiscovered %d dependent repositories:\n\n", len(dependents))

	// Display the discovered dependents with indices
	for i, dep := range dependents {
		source := dep.DiscoverySource
		if source == "" {
			source = "unknown"
		}
		fmt.Printf("  %d. %s (module: %s, source: %s)\n", i+1, dep.Repository, dep.ModulePath, source)
		if dep.LocalModulePath != "." && dep.LocalModulePath != "" {
			fmt.Printf("     Local path: %s\n", dep.LocalModulePath)
		}
	}

	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  a - include all")
	fmt.Println("  n - include none")
	fmt.Println("  1,2,3 - include specific repositories by number")
	fmt.Println("  1-3,5 - include ranges and specific repositories")
	fmt.Print("\nSelect dependents to include [a]: ")

	var input string
	fmt.Scanln(&input)

	// Default to "all" if no input provided
	if input == "" || input == "a" || input == "all" {
		return dependents, nil
	}

	// Handle "none" case
	if input == "n" || input == "none" {
		return []manifest.DependentOptions{}, nil
	}

	// Parse selection indices
	selectedIndices, err := parseSelectionInput(input, len(dependents))
	if err != nil {
		return nil, fmt.Errorf("invalid selection: %w", err)
	}

	// Build result with selected dependents
	result := make([]manifest.DependentOptions, 0, len(selectedIndices))
	for _, index := range selectedIndices {
		result = append(result, dependents[index])
	}

	fmt.Printf("Selected %d dependents for inclusion.\n", len(result))
	return result, nil
}

// parseSelectionInput parses user input for dependent selection.
// Supports formats like "1,2,3", "1-3,5", etc.
func parseSelectionInput(input string, maxIndex int) ([]int, error) {
	if input == "" {
		return nil, fmt.Errorf("empty input")
	}

	var indices []int
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle range (e.g., "1-3")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start number in range %s: %w", part, err)
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end number in range %s: %w", part, err)
			}

			if start < 1 || end > maxIndex || start > end {
				return nil, fmt.Errorf("invalid range %s: must be between 1 and %d", part, maxIndex)
			}

			for i := start; i <= end; i++ {
				indices = append(indices, i-1) // Convert to 0-based indexing
			}
		} else {
			// Handle single number
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}

			if num < 1 || num > maxIndex {
				return nil, fmt.Errorf("number %d out of range: must be between 1 and %d", num, maxIndex)
			}

			indices = append(indices, num-1) // Convert to 0-based indexing
		}
	}

	// Remove duplicates
	uniqueIndices := make([]int, 0, len(indices))
	seen := make(map[int]bool)
	for _, index := range indices {
		if !seen[index] {
			uniqueIndices = append(uniqueIndices, index)
			seen[index] = true
		}
	}

	return uniqueIndices, nil
}
