package main

import (
	"fmt"
	"os"
	"time"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
	"github.com/spf13/cobra"
)

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
		newWorkflowCommand(),
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
