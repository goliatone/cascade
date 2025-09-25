package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

// Global variables for CLI state
var (
	container di.Container
	cfg       *config.Config
)

func main() {
	if err := execute(); err != nil {
		fmt.Fprintf(os.Stderr, "cascade: %v\n", err)
		os.Exit(1)
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
		return fmt.Errorf("failed to build configuration: %w", err)
	}

	// Create container with configuration
	container, err = di.New(di.WithConfig(cfg))
	if err != nil {
		return fmt.Errorf("failed to initialize dependencies: %w", err)
	}

	return nil
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

// Command implementations (stubbed for now)

func runPlan(manifestPath string) error {
	logger := container.Logger()
	logger.Info("Planning dependency updates", "manifest", manifestPath)

	// TODO: Implement plan logic using container services
	fmt.Println("Plan command not yet implemented")
	return nil
}

func runRelease(manifestPath string) error {
	logger := container.Logger()
	logger.Info("Executing dependency updates", "manifest", manifestPath)

	// TODO: Implement release logic using container services
	fmt.Println("Release command not yet implemented")
	return nil
}

func runResume(stateID string) error {
	logger := container.Logger()
	logger.Info("Resuming operation", "stateID", stateID)

	// TODO: Implement resume logic using container services
	fmt.Println("Resume command not yet implemented")
	return nil
}

func runRevert(stateID string) error {
	logger := container.Logger()
	logger.Info("Reverting operation", "stateID", stateID)

	// TODO: Implement revert logic using container services
	fmt.Println("Revert command not yet implemented")
	return nil
}
