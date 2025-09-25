package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagConfig represents flag parsing configuration and results
type FlagConfig struct {
	// Command-line flag values
	Workspace  string
	Manifest   string
	Module     string
	Version    string
	DryRun     bool
	Verbose    bool
	Quiet      bool
	Timeout    time.Duration
	Parallel   int
	ConfigFile string

	// GitHub integration flags
	GitHubToken    string
	GitHubEndpoint string
	GitHubOrg      string

	// Slack integration flags
	SlackToken   string
	SlackWebhook string
	SlackChannel string

	// State flags
	StateDir     string
	StateEnabled bool

	// Logging flags
	LogLevel  string
	LogFormat string
}

// AddFlags adds all configuration flags to the provided cobra command.
// This function defines all available command-line flags with their
// default values, help text, and validation rules.
func AddFlags(cmd *cobra.Command) *FlagConfig {
	fc := &FlagConfig{}

	// Workspace and basic operation flags (persistent flags are inherited by subcommands)
	cmd.PersistentFlags().StringVarP(&fc.Workspace, "workspace", "w", "",
		"Workspace directory for operations (default: $XDG_CACHE_HOME/cascade)")
	cmd.PersistentFlags().StringVarP(&fc.Manifest, "manifest", "m", "",
		"Path to deps.yaml manifest file")
	cmd.PersistentFlags().StringVar(&fc.Module, "module", "",
		"Target module for operations")
	cmd.PersistentFlags().StringVar(&fc.Version, "version", "",
		"Target version for operations")
	cmd.PersistentFlags().StringVarP(&fc.ConfigFile, "config", "c", "",
		"Configuration file path")

	// Execution control flags
	cmd.PersistentFlags().BoolVarP(&fc.DryRun, "dry-run", "n", false,
		"Preview mode without making changes")
	cmd.PersistentFlags().DurationVar(&fc.Timeout, "timeout", 5*time.Minute,
		"Operation timeout duration")
	cmd.PersistentFlags().IntVarP(&fc.Parallel, "parallel", "p", 0,
		"Number of parallel executions (0 = auto)")

	// Logging control flags
	cmd.PersistentFlags().BoolVarP(&fc.Verbose, "verbose", "v", false,
		"Verbose logging output (equivalent to --log-level=debug)")
	cmd.PersistentFlags().BoolVarP(&fc.Quiet, "quiet", "q", false,
		"Suppress non-essential output (equivalent to --log-level=warn)")
	cmd.PersistentFlags().StringVar(&fc.LogLevel, "log-level", "",
		"Logging level (debug, info, warn, error)")
	cmd.PersistentFlags().StringVar(&fc.LogFormat, "log-format", "",
		"Log output format (text, json)")

	// GitHub integration flags
	cmd.Flags().StringVar(&fc.GitHubToken, "github-token", "",
		"GitHub authentication token")
	cmd.Flags().StringVar(&fc.GitHubEndpoint, "github-endpoint", "",
		"GitHub API endpoint URL")
	cmd.Flags().StringVar(&fc.GitHubOrg, "github-org", "",
		"GitHub organization")

	// Slack integration flags
	cmd.Flags().StringVar(&fc.SlackToken, "slack-token", "",
		"Slack bot token")
	cmd.Flags().StringVar(&fc.SlackWebhook, "slack-webhook", "",
		"Slack webhook URL")
	cmd.Flags().StringVar(&fc.SlackChannel, "slack-channel", "",
		"Slack channel for notifications")

	// State management flags
	cmd.Flags().StringVar(&fc.StateDir, "state-dir", "",
		"State persistence directory")
	cmd.Flags().BoolVar(&fc.StateEnabled, "state", true,
		"Enable state persistence")

	// Mark verbose and quiet as mutually exclusive
	cmd.MarkFlagsMutuallyExclusive("verbose", "quiet")
	cmd.MarkFlagsMutuallyExclusive("verbose", "log-level")
	cmd.MarkFlagsMutuallyExclusive("quiet", "log-level")

	return fc
}

// ValidateFlags validates flag combinations and values.
// Returns an error if any validation rules are violated.
func (fc *FlagConfig) ValidateFlags() error {
	var errors []string

	// Validate timeout
	if fc.Timeout <= 0 {
		errors = append(errors, "timeout must be positive")
	}

	// Validate parallel count
	if fc.Parallel < 0 {
		errors = append(errors, "parallel count must be non-negative")
	}

	// Validate log level if specified
	if fc.LogLevel != "" {
		validLevels := []string{"debug", "info", "warn", "error"}
		isValid := false
		for _, level := range validLevels {
			if fc.LogLevel == level {
				isValid = true
				break
			}
		}
		if !isValid {
			errors = append(errors, fmt.Sprintf("log-level must be one of: %s", strings.Join(validLevels, ", ")))
		}
	}

	// Validate log format if specified
	if fc.LogFormat != "" {
		validFormats := []string{"text", "json"}
		isValid := false
		for _, format := range validFormats {
			if fc.LogFormat == format {
				isValid = true
				break
			}
		}
		if !isValid {
			errors = append(errors, fmt.Sprintf("log-format must be one of: %s", strings.Join(validFormats, ", ")))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("flag validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}

// ToConfig converts flag configuration to a Config struct.
// This applies flag values with proper precedence over environment variables.
// Deprecated: Use ToConfigWithCommand for proper flag change detection.
func (fc *FlagConfig) ToConfig() (*Config, error) {
	return fc.ToConfigWithCommand(nil)
}

// ToConfigWithCommand converts flag configuration to a Config struct with command context.
// This applies flag values with proper precedence over environment variables.
// The command parameter is used for detecting which flags were explicitly changed.
func (fc *FlagConfig) ToConfigWithCommand(cmd *cobra.Command) (*Config, error) {
	// Start with environment-based configuration
	config, err := LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to load environment configuration: %w", err)
	}

	// Override with flag values (flags have higher precedence)
	if fc.Workspace != "" {
		config.Workspace.Path = fc.Workspace
	}

	if fc.Manifest != "" {
		config.Workspace.ManifestPath = fc.Manifest
	}

	if fc.Timeout > 0 {
		config.Executor.Timeout = fc.Timeout
	}

	if fc.Parallel > 0 {
		config.Executor.ConcurrentLimit = fc.Parallel
	}

	// Dry run flag
	if fc.DryRun {
		config.Executor.DryRun = fc.DryRun
	}

	// Logging configuration with mutual exclusivity
	if fc.Verbose {
		config.Logging.Verbose = true
		config.Logging.Level = "debug"
	} else if fc.Quiet {
		config.Logging.Quiet = true
		config.Logging.Level = "warn"
	} else if fc.LogLevel != "" {
		config.Logging.Level = fc.LogLevel
	}

	if fc.LogFormat != "" {
		config.Logging.Format = fc.LogFormat
	}

	// GitHub integration flags
	if fc.GitHubToken != "" {
		config.Integration.GitHub.Token = fc.GitHubToken
	}

	if fc.GitHubEndpoint != "" {
		config.Integration.GitHub.Endpoint = fc.GitHubEndpoint
	}

	if fc.GitHubOrg != "" {
		config.Integration.GitHub.Organization = fc.GitHubOrg
	}

	// Slack integration flags
	if fc.SlackToken != "" {
		config.Integration.Slack.Token = fc.SlackToken
	}

	if fc.SlackWebhook != "" {
		config.Integration.Slack.WebhookURL = fc.SlackWebhook
	}

	if fc.SlackChannel != "" {
		config.Integration.Slack.Channel = fc.SlackChannel
	}

	// State management flags
	if fc.StateDir != "" {
		config.State.Dir = fc.StateDir
	}

	// State enabled is handled specially - only override if explicitly set
	if cmd != nil && cmd.Flags().Changed("state") {
		config.State.Enabled = fc.StateEnabled
	}

	// Target module and version for cascade operations
	if fc.Module != "" {
		config.Module = fc.Module
	}

	if fc.Version != "" {
		config.Version = fc.Version
	}

	// Also check if flags were changed on the command directly (for persistent flags)
	if cmd != nil {
		if cmd.Flags().Changed("module") {
			if val, err := cmd.Flags().GetString("module"); err == nil && val != "" {
				config.Module = val
			}
		}
		if cmd.Flags().Changed("version") {
			if val, err := cmd.Flags().GetString("version"); err == nil && val != "" {
				config.Version = val
			}
		}
	}

	return config, nil
}

// LoadFromFlags loads configuration from command-line flags using cobra.
// This is the main entry point for flag-based configuration.
func LoadFromFlags(cmd *cobra.Command) (*Config, error) {
	if cmd == nil {
		return nil, fmt.Errorf("command cannot be nil")
	}

	// cmd.Flags() returns both local and inherited flags
	fc := extractFlagConfig(cmd.Flags())

	// Validate flags
	if err := fc.ValidateFlags(); err != nil {
		return nil, err
	}

	// Convert to config with proper precedence, passing the command for flag change detection
	config, err := fc.ToConfigWithCommand(cmd)
	if err != nil {
		return nil, err
	}

	return config, nil
}

// extractFlagConfig extracts flag values from a flag set into FlagConfig
func extractFlagConfig(flags *pflag.FlagSet) *FlagConfig {
	fc := &FlagConfig{}

	// Try to extract from both regular and persistent flags
	if flags.Changed("workspace") {
		fc.Workspace, _ = flags.GetString("workspace")
	}
	if flags.Changed("manifest") {
		fc.Manifest, _ = flags.GetString("manifest")
	}
	if flags.Changed("module") {
		fc.Module, _ = flags.GetString("module")
	}
	if flags.Changed("version") {
		fc.Version, _ = flags.GetString("version")
	}
	if flags.Changed("config") {
		fc.ConfigFile, _ = flags.GetString("config")
	}
	if flags.Changed("dry-run") {
		fc.DryRun, _ = flags.GetBool("dry-run")
	}
	// Always get timeout value since it has a default and validation expects it
	fc.Timeout, _ = flags.GetDuration("timeout")

	if flags.Changed("parallel") {
		fc.Parallel, _ = flags.GetInt("parallel")
	}
	if flags.Changed("verbose") {
		fc.Verbose, _ = flags.GetBool("verbose")
	}
	if flags.Changed("quiet") {
		fc.Quiet, _ = flags.GetBool("quiet")
	}
	if flags.Changed("log-level") {
		fc.LogLevel, _ = flags.GetString("log-level")
	}
	if flags.Changed("log-format") {
		fc.LogFormat, _ = flags.GetString("log-format")
	}
	if flags.Changed("github-token") {
		fc.GitHubToken, _ = flags.GetString("github-token")
	}
	if flags.Changed("github-endpoint") {
		fc.GitHubEndpoint, _ = flags.GetString("github-endpoint")
	}
	if flags.Changed("github-org") {
		fc.GitHubOrg, _ = flags.GetString("github-org")
	}
	if flags.Changed("slack-token") {
		fc.SlackToken, _ = flags.GetString("slack-token")
	}
	if flags.Changed("slack-webhook") {
		fc.SlackWebhook, _ = flags.GetString("slack-webhook")
	}
	if flags.Changed("slack-channel") {
		fc.SlackChannel, _ = flags.GetString("slack-channel")
	}
	if flags.Changed("state-dir") {
		fc.StateDir, _ = flags.GetString("state-dir")
	}
	if flags.Changed("state") {
		fc.StateEnabled, _ = flags.GetBool("state")
	}

	return fc
}
