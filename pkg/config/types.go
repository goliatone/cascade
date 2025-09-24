package config

// Config aggregates all cascade configuration inputs.
type Config struct {
	Workspace    WorkspaceConfig   `json:"workspace" yaml:"workspace"`
	Executor     ExecutorConfig    `json:"executor" yaml:"executor"`
	Integrations IntegrationConfig `json:"integrations" yaml:"integrations"`
	Logging      LoggingConfig     `json:"logging" yaml:"logging"`
	State        StateConfig       `json:"state" yaml:"state"`
}

// WorkspaceConfig captures path-related settings for cascade operations.
type WorkspaceConfig struct {
	Root string `json:"root" yaml:"root"`
	Temp string `json:"temp" yaml:"temp"`
}

// ExecutorConfig contains execution-time controls such as timeouts and parallelism.
type ExecutorConfig struct {
	Timeout   string `json:"timeout" yaml:"timeout"`
	Parallel  int    `json:"parallel" yaml:"parallel"`
	DryRun    bool   `json:"dry_run" yaml:"dry_run"`
	Verbosity string `json:"verbosity" yaml:"verbosity"`
}

// IntegrationConfig holds tokens and endpoints for external systems the CLI talks to.
type IntegrationConfig struct {
	GitHubToken    string `json:"github_token" yaml:"github_token"`
	GitHubEndpoint string `json:"github_endpoint" yaml:"github_endpoint"`
	SlackWebhook   string `json:"slack_webhook" yaml:"slack_webhook"`
}

// LoggingConfig defines how cascade should emit logs.
type LoggingConfig struct {
	Level       string `json:"level" yaml:"level"`
	Format      string `json:"format" yaml:"format"`
	Destination string `json:"destination" yaml:"destination"`
}

// StateConfig controls state persistence behaviour.
type StateConfig struct {
	Directory     string `json:"directory" yaml:"directory"`
	Retention     int    `json:"retention" yaml:"retention"`
	DisableResume bool   `json:"disable_resume" yaml:"disable_resume"`
}

// New returns a Config populated with safe zero values.
func New() *Config {
	return &Config{}
}
