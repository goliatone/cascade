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

	// ManifestGenerator contains defaults for manifest generation operations
	ManifestGenerator ManifestGeneratorConfig `json:"manifest_generator" yaml:"manifest_generator"`

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

// ManifestGeneratorConfig contains default settings for manifest generation
// operations to reduce the need for command-line flags.
type ManifestGeneratorConfig struct {
	// DefaultWorkspace is the default workspace directory for discovering dependent modules.
	// If empty, uses current directory or config workspace path.
	DefaultWorkspace string `json:"default_workspace,omitempty" yaml:"default_workspace,omitempty"`

	// Tests contains default test command configurations.
	Tests TestsConfig `json:"tests" yaml:"tests"`

	// Notifications contains default notification settings for manifest operations.
	Notifications NotificationsConfig `json:"notifications" yaml:"notifications"`

	// TemplateProfiles contains predefined template configurations.
	TemplateProfiles map[string]TemplateProfileConfig `json:"template_profiles,omitempty" yaml:"template_profiles,omitempty"`

	// DefaultBranch is the default branch name to use for dependency updates.
	// Default: "cascade/update-deps"
	DefaultBranch string `json:"default_branch,omitempty" yaml:"default_branch,omitempty"`

	// DiscoverySettings contains settings for automatic dependent discovery.
	Discovery DiscoveryConfig `json:"discovery" yaml:"discovery"`
}

// TestsConfig contains default test command configurations.
type TestsConfig struct {
	// Command is the default test command to run for discovered dependents.
	// Default: "go test ./..."
	Command string `json:"command,omitempty" yaml:"command,omitempty"`

	// Timeout is the default timeout for test execution.
	// Default: 5 minutes
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// WorkingDirectory is the default working directory for test execution.
	// If empty, uses the module root directory.
	WorkingDirectory string `json:"working_directory,omitempty" yaml:"working_directory,omitempty"`
}

// NotificationsConfig contains default notification settings.
type NotificationsConfig struct {
	// Enabled controls whether notifications are sent by default.
	// Default: false
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Channels contains the default notification channels to use.
	Channels []string `json:"channels,omitempty" yaml:"channels,omitempty"`

	// OnSuccess controls whether to send notifications on successful operations.
	// Default: false
	OnSuccess bool `json:"on_success" yaml:"on_success"`

	// OnFailure controls whether to send notifications on failed operations.
	// Default: true
	OnFailure bool `json:"on_failure" yaml:"on_failure"`
}

// TemplateProfileConfig contains predefined template configurations
// that can be referenced by name during manifest generation.
type TemplateProfileConfig struct {
	// Name is the profile identifier used in CLI commands.
	Name string `json:"name" yaml:"name" validate:"required"`

	// Description provides a human-readable description of the profile.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Tests overrides the default test configuration for this profile.
	Tests *TestsConfig `json:"tests,omitempty" yaml:"tests,omitempty"`

	// Notifications overrides the default notification configuration for this profile.
	Notifications *NotificationsConfig `json:"notifications,omitempty" yaml:"notifications,omitempty"`

	// Branch overrides the default branch name for this profile.
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`
}

// DiscoveryConfig contains settings for automatic dependent discovery.
type DiscoveryConfig struct {
	// Enabled controls whether automatic discovery is enabled by default.
	// Default: true
	Enabled bool `json:"enabled" yaml:"enabled"`

	// MaxDepth limits how deep to scan for dependent modules in workspace discovery.
	// Default: 3
	MaxDepth int `json:"max_depth,omitempty" yaml:"max_depth,omitempty" validate:"min=1"`

	// IncludePatterns contains glob patterns for directories/files to include in discovery.
	IncludePatterns []string `json:"include_patterns,omitempty" yaml:"include_patterns,omitempty"`

	// ExcludePatterns contains glob patterns for directories/files to exclude from discovery.
	// Default: ["vendor/*", ".git/*", "node_modules/*"]
	ExcludePatterns []string `json:"exclude_patterns,omitempty" yaml:"exclude_patterns,omitempty"`

	// Interactive controls whether to prompt for confirmation of discovered dependents.
	// Default: true (can be overridden by --yes or --non-interactive flags)
	Interactive bool `json:"interactive" yaml:"interactive"`

	// GitHub contains settings for GitHub organization discovery.
	GitHub GitHubDiscoveryConfig `json:"github" yaml:"github"`
}

// GitHubDiscoveryConfig contains settings for GitHub organization discovery.
type GitHubDiscoveryConfig struct {
	// Organization is the default GitHub organization to search for dependent repositories.
	Organization string `json:"organization,omitempty" yaml:"organization,omitempty"`

	// IncludePatterns contains patterns for repository names to include during GitHub discovery.
	IncludePatterns []string `json:"include_patterns,omitempty" yaml:"include_patterns,omitempty"`

	// ExcludePatterns contains patterns for repository names to exclude during GitHub discovery.
	ExcludePatterns []string `json:"exclude_patterns,omitempty" yaml:"exclude_patterns,omitempty"`

	// Enabled controls whether GitHub discovery is enabled by default.
	// Default: false (only when explicitly requested via --github-org flag)
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

	// Manifest Generator environment variables
	EnvManifestGeneratorWorkspace            = "CASCADE_MANIFEST_GENERATOR_WORKSPACE"
	EnvManifestGeneratorTestCommand          = "CASCADE_MANIFEST_GENERATOR_TEST_COMMAND"
	EnvManifestGeneratorTestTimeout          = "CASCADE_MANIFEST_GENERATOR_TEST_TIMEOUT"
	EnvManifestGeneratorNotificationsEnable  = "CASCADE_MANIFEST_GENERATOR_NOTIFICATIONS_ENABLED"
	EnvManifestGeneratorDefaultBranch        = "CASCADE_MANIFEST_GENERATOR_DEFAULT_BRANCH"
	EnvManifestGeneratorDiscoveryEnabled     = "CASCADE_MANIFEST_GENERATOR_DISCOVERY_ENABLED"
	EnvManifestGeneratorDiscoveryMaxDepth    = "CASCADE_MANIFEST_GENERATOR_DISCOVERY_MAX_DEPTH"
	EnvManifestGeneratorDiscoveryInteractive = "CASCADE_MANIFEST_GENERATOR_DISCOVERY_INTERACTIVE"

	// GitHub Discovery environment variables
	EnvManifestGeneratorGitHubOrg             = "CASCADE_MANIFEST_GENERATOR_GITHUB_ORG"
	EnvManifestGeneratorGitHubEnabled         = "CASCADE_MANIFEST_GENERATOR_GITHUB_ENABLED"
	EnvManifestGeneratorGitHubIncludePatterns = "CASCADE_MANIFEST_GENERATOR_GITHUB_INCLUDE_PATTERNS"
	EnvManifestGeneratorGitHubExcludePatterns = "CASCADE_MANIFEST_GENERATOR_GITHUB_EXCLUDE_PATTERNS"
)

// New returns a Config populated with safe zero values.
func New() *Config {
	return &Config{}
}
