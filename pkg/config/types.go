package config

import "time"

// Config represents the complete configuration for Cascade operations.
// It aggregates all configuration aspects including workspace, execution,
// integration, logging, and state management settings.
type Config struct {
	// Workspace contains workspace and temporary directory settings
	Workspace WorkspaceConfig `json:"workspace" yaml:"workspace"`

	// Executor contains executor-specific settings like timeouts and concurrency
	Executor ExecutorConfig `json:"executor" yaml:"executor"`

	// Integration contains settings for external integrations (GitHub, Slack, etc.)
	Integration IntegrationConfig `json:"integration" yaml:"integration"`

	// Logging contains logging level and output configuration
	Logging LoggingConfig `json:"logging" yaml:"logging"`

	// State contains state persistence settings
	State StateConfig `json:"state" yaml:"state"`

	// Target module and version for cascade operations
	// These are typically specified via command-line flags
	Module  string `json:"module,omitempty" yaml:"module,omitempty"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	setFlags boolFlags `json:"-" yaml:"-"`
}

type boolFlags struct {
	executorDryRun bool
	loggingVerbose bool
	loggingQuiet   bool
	stateEnabled   bool
}

// WorkspaceConfig manages workspace and temporary directory settings.
// It defines where Cascade performs its operations and stores temporary files.
type WorkspaceConfig struct {
	// Path is the primary workspace directory where operations are performed.
	// Default: $XDG_CACHE_HOME/cascade or ~/.cache/cascade
	Path string `json:"path" yaml:"path" validate:"required"`

	// TempDir is an optional override for temporary directory location.
	// If empty, uses system default temporary directory.
	TempDir string `json:"temp_dir,omitempty" yaml:"temp_dir,omitempty"`

	// ManifestPath is the path to the deps.yaml manifest file.
	// Required for most operations unless specified via command-line flags.
	ManifestPath string `json:"manifest_path,omitempty" yaml:"manifest_path,omitempty"`
}

// ExecutorConfig contains executor-specific settings that control
// operation timeouts, concurrency limits, and execution behavior.
type ExecutorConfig struct {
	// Timeout is the default timeout duration for operations.
	// Default: 5 minutes
	Timeout time.Duration `json:"timeout" yaml:"timeout"`

	// ConcurrentLimit is the maximum number of concurrent executions.
	// Default: CPU count or 4, whichever is smaller
	ConcurrentLimit int `json:"concurrent_limit" yaml:"concurrent_limit" validate:"min=1"`

	// DryRun enables preview mode without making actual changes.
	// Can be overridden by command-line flags.
	DryRun bool `json:"dry_run" yaml:"dry_run"`
}

// IntegrationConfig manages settings for external service integrations
// including GitHub, Slack, and other third-party services.
type IntegrationConfig struct {
	// GitHub contains GitHub API integration settings
	GitHub GitHubConfig `json:"github" yaml:"github"`

	// Slack contains Slack notification integration settings
	Slack SlackConfig `json:"slack" yaml:"slack"`
}

// GitHubConfig contains GitHub API integration settings including
// authentication tokens and API endpoint configuration.
type GitHubConfig struct {
	// Token is the GitHub authentication token for API access.
	// Should be loaded from environment variables or secure files.
	Token string `json:"token,omitempty" yaml:"token,omitempty"`

	// Endpoint is the GitHub API endpoint URL.
	// Default: https://api.github.com for GitHub.com
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`

	// Organization is the default GitHub organization for operations.
	Organization string `json:"organization,omitempty" yaml:"organization,omitempty"`
}

// SlackConfig contains Slack integration settings for notifications
// and webhook-based communication.
type SlackConfig struct {
	// Token is the Slack bot token for API access.
	Token string `json:"token,omitempty" yaml:"token,omitempty"`

	// WebhookURL is the Slack webhook URL for sending notifications.
	WebhookURL string `json:"webhook_url,omitempty" yaml:"webhook_url,omitempty"`

	// Channel is the default Slack channel for notifications.
	Channel string `json:"channel,omitempty" yaml:"channel,omitempty"`
}

// LoggingConfig manages logging level, output format, and
// structured logging configuration.
type LoggingConfig struct {
	// Level controls the logging verbosity level.
	// Valid values: debug, info, warn, error
	// Default: info
	Level string `json:"level" yaml:"level" validate:"oneof=debug info warn error"`

	// Format controls the log output format.
	// Valid values: text, json
	// Default: text
	Format string `json:"format" yaml:"format" validate:"oneof=text json"`

	// Verbose enables verbose logging output.
	// Equivalent to setting Level to "debug"
	Verbose bool `json:"verbose" yaml:"verbose"`

	// Quiet suppresses non-essential output.
	// Equivalent to setting Level to "warn"
	Quiet bool `json:"quiet" yaml:"quiet"`
}

// StateConfig manages state persistence settings including
// directory location and retention policies.
type StateConfig struct {
	// Dir is the directory where state files are persisted.
	// Default: $XDG_STATE_HOME/cascade or ~/.local/state/cascade
	Dir string `json:"dir" yaml:"dir"`

	// RetentionCount is the number of state snapshots to retain.
	// Default: 10
	RetentionCount int `json:"retention_count" yaml:"retention_count" validate:"min=1"`

	// Enabled controls whether state persistence is active.
	// Default: true
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// Environment variable mapping constants for configuration parsing
const (
	// Workspace environment variables
	EnvWorkspacePath = "CASCADE_WORKSPACE"
	EnvTempDir       = "CASCADE_TEMP_DIR"
	EnvManifestPath  = "CASCADE_MANIFEST"

	// Executor environment variables
	EnvTimeout         = "CASCADE_TIMEOUT"
	EnvConcurrentLimit = "CASCADE_CONCURRENT_LIMIT"
	EnvDryRun          = "CASCADE_DRY_RUN"

	// GitHub integration environment variables
	EnvGitHubToken    = "CASCADE_GITHUB_TOKEN"
	EnvGitHubEndpoint = "CASCADE_GITHUB_ENDPOINT"
	EnvGitHubOrg      = "CASCADE_GITHUB_ORG"

	// Slack integration environment variables
	EnvSlackToken   = "CASCADE_SLACK_TOKEN"
	EnvSlackWebhook = "CASCADE_SLACK_WEBHOOK"
	EnvSlackChannel = "CASCADE_SLACK_CHANNEL"

	// Logging environment variables
	EnvLogLevel  = "CASCADE_LOG_LEVEL"
	EnvLogFormat = "CASCADE_LOG_FORMAT"
	EnvVerbose   = "CASCADE_VERBOSE"
	EnvQuiet     = "CASCADE_QUIET"

	// State environment variables
	EnvStateDir       = "CASCADE_STATE_DIR"
	EnvStateRetention = "CASCADE_STATE_RETENTION"
	EnvStateEnabled   = "CASCADE_STATE_ENABLED"
)

// New returns a Config populated with safe zero values.
func New() *Config {
	return &Config{}
}
