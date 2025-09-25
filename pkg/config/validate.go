package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ValidationError represents a configuration validation failure.
type ValidationError struct {
	Field   string
	Value   any
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation error: %s: %s", e.Field, e.Message)
}

// ValidationErrors aggregates multiple validation failures.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("config validation errors:\n  - %s", strings.Join(msgs, "\n  - "))
}

// Validate inspects the configuration for missing or invalid fields.
// It applies comprehensive validation rules and returns aggregated errors.
func Validate(cfg *Config) error {
	if cfg == nil {
		return &ValidationError{
			Field:   "config",
			Value:   nil,
			Message: "configuration cannot be nil",
		}
	}

	var errors ValidationErrors

	// Validate workspace configuration
	errors = append(errors, validateWorkspace(&cfg.Workspace)...)

	// Validate executor configuration
	errors = append(errors, validateExecutor(&cfg.Executor)...)

	// Validate integration configuration
	errors = append(errors, validateIntegration(&cfg.Integration)...)

	// Validate logging configuration
	errors = append(errors, validateLogging(&cfg.Logging)...)

	// Validate state configuration
	errors = append(errors, validateState(&cfg.State)...)

	if len(errors) > 0 {
		return errors
	}

	return nil
}

// ApplyDefaults applies sensible defaults to the configuration.
// It should be called after parsing but before validation.
func ApplyDefaults(cfg *Config) error {
	if cfg == nil {
		return &ValidationError{
			Field:   "config",
			Value:   nil,
			Message: "configuration cannot be nil",
		}
	}

	// Apply workspace defaults
	applyWorkspaceDefaults(&cfg.Workspace)

	// Apply executor defaults
	applyExecutorDefaults(&cfg.Executor)

	// Apply integration defaults
	applyIntegrationDefaults(&cfg.Integration)

	// Apply logging defaults
	applyLoggingDefaults(&cfg.Logging)

	// Apply state defaults
	applyStateDefaults(cfg)

	return nil
}

// validateWorkspace validates workspace configuration settings.
func validateWorkspace(ws *WorkspaceConfig) []ValidationError {
	var errors []ValidationError

	// Workspace path validation
	if ws.Path == "" {
		errors = append(errors, ValidationError{
			Field:   "workspace.path",
			Value:   ws.Path,
			Message: "workspace path is required",
		})
	} else {
		if !filepath.IsAbs(ws.Path) {
			errors = append(errors, ValidationError{
				Field:   "workspace.path",
				Value:   ws.Path,
				Message: "workspace path must be absolute",
			})
		}
	}

	// TempDir validation (if provided)
	if ws.TempDir != "" {
		if !filepath.IsAbs(ws.TempDir) {
			errors = append(errors, ValidationError{
				Field:   "workspace.temp_dir",
				Value:   ws.TempDir,
				Message: "temp directory path must be absolute",
			})
		}
	}

	// ManifestPath validation (if provided)
	if ws.ManifestPath != "" {
		if !filepath.IsAbs(ws.ManifestPath) {
			errors = append(errors, ValidationError{
				Field:   "workspace.manifest_path",
				Value:   ws.ManifestPath,
				Message: "manifest path must be absolute",
			})
		} else {
			// Check if manifest file exists and is readable
			if _, err := os.Stat(ws.ManifestPath); os.IsNotExist(err) {
				errors = append(errors, ValidationError{
					Field:   "workspace.manifest_path",
					Value:   ws.ManifestPath,
					Message: "manifest file does not exist",
				})
			} else if err != nil {
				errors = append(errors, ValidationError{
					Field:   "workspace.manifest_path",
					Value:   ws.ManifestPath,
					Message: fmt.Sprintf("cannot access manifest file: %v", err),
				})
			}
		}
	}

	return errors
}

// validateExecutor validates executor configuration settings.
func validateExecutor(exec *ExecutorConfig) []ValidationError {
	var errors []ValidationError

	// Timeout validation
	if exec.Timeout <= 0 {
		errors = append(errors, ValidationError{
			Field:   "executor.timeout",
			Value:   exec.Timeout,
			Message: "timeout must be positive",
		})
	} else if exec.Timeout > 24*time.Hour {
		errors = append(errors, ValidationError{
			Field:   "executor.timeout",
			Value:   exec.Timeout,
			Message: "timeout cannot exceed 24 hours",
		})
	}

	// Concurrency limit validation
	if exec.ConcurrentLimit <= 0 {
		errors = append(errors, ValidationError{
			Field:   "executor.concurrent_limit",
			Value:   exec.ConcurrentLimit,
			Message: "concurrent limit must be positive",
		})
	} else if exec.ConcurrentLimit > 1000 {
		errors = append(errors, ValidationError{
			Field:   "executor.concurrent_limit",
			Value:   exec.ConcurrentLimit,
			Message: "concurrent limit cannot exceed 1000",
		})
	}

	return errors
}

// validateIntegration validates integration configuration settings.
func validateIntegration(integ *IntegrationConfig) []ValidationError {
	var errors []ValidationError

	// Validate GitHub configuration
	errors = append(errors, validateGitHub(&integ.GitHub)...)

	// Validate Slack configuration
	errors = append(errors, validateSlack(&integ.Slack)...)

	return errors
}

// validateGitHub validates GitHub integration settings.
func validateGitHub(gh *GitHubConfig) []ValidationError {
	var errors []ValidationError

	// GitHub token validation (if provided)
	if gh.Token != "" {
		// Basic token format validation - GitHub tokens start with specific prefixes
		if !isValidGitHubToken(gh.Token) {
			errors = append(errors, ValidationError{
				Field:   "integration.github.token",
				Value:   "[REDACTED]",
				Message: "invalid GitHub token format",
			})
		}
	}

	// GitHub endpoint validation (if provided)
	if gh.Endpoint != "" {
		if _, err := url.Parse(gh.Endpoint); err != nil {
			errors = append(errors, ValidationError{
				Field:   "integration.github.endpoint",
				Value:   gh.Endpoint,
				Message: fmt.Sprintf("invalid GitHub endpoint URL: %v", err),
			})
		}
	}

	return errors
}

// validateSlack validates Slack integration settings.
func validateSlack(slack *SlackConfig) []ValidationError {
	var errors []ValidationError

	// Slack token validation (if provided)
	if slack.Token != "" {
		if !isValidSlackToken(slack.Token) {
			errors = append(errors, ValidationError{
				Field:   "integration.slack.token",
				Value:   "[REDACTED]",
				Message: "invalid Slack token format",
			})
		}
	}

	// Slack webhook URL validation (if provided)
	if slack.WebhookURL != "" {
		if u, err := url.Parse(slack.WebhookURL); err != nil {
			errors = append(errors, ValidationError{
				Field:   "integration.slack.webhook_url",
				Value:   "[REDACTED]",
				Message: fmt.Sprintf("invalid Slack webhook URL: %v", err),
			})
		} else if u.Scheme != "https" {
			errors = append(errors, ValidationError{
				Field:   "integration.slack.webhook_url",
				Value:   "[REDACTED]",
				Message: "Slack webhook URL must use HTTPS",
			})
		}
	}

	// Slack channel validation (if provided)
	if slack.Channel != "" {
		if !strings.HasPrefix(slack.Channel, "#") && !strings.HasPrefix(slack.Channel, "@") {
			errors = append(errors, ValidationError{
				Field:   "integration.slack.channel",
				Value:   slack.Channel,
				Message: "Slack channel must start with # (channel) or @ (user)",
			})
		}
	}

	return errors
}

// validateLogging validates logging configuration settings.
func validateLogging(log *LoggingConfig) []ValidationError {
	var errors []ValidationError

	// Log level validation
	validLevels := []string{"debug", "info", "warn", "error"}
	if log.Level != "" && !contains(validLevels, log.Level) {
		errors = append(errors, ValidationError{
			Field:   "logging.level",
			Value:   log.Level,
			Message: fmt.Sprintf("invalid log level, must be one of: %s", strings.Join(validLevels, ", ")),
		})
	}

	// Log format validation
	validFormats := []string{"text", "json"}
	if log.Format != "" && !contains(validFormats, log.Format) {
		errors = append(errors, ValidationError{
			Field:   "logging.format",
			Value:   log.Format,
			Message: fmt.Sprintf("invalid log format, must be one of: %s", strings.Join(validFormats, ", ")),
		})
	}

	// Mutual exclusivity check for verbose and quiet
	if log.Verbose && log.Quiet {
		errors = append(errors, ValidationError{
			Field:   "logging.verbose,logging.quiet",
			Value:   fmt.Sprintf("verbose=%t, quiet=%t", log.Verbose, log.Quiet),
			Message: "verbose and quiet modes are mutually exclusive",
		})
	}

	return errors
}

// validateState validates state configuration settings.
func validateState(state *StateConfig) []ValidationError {
	var errors []ValidationError

	// State directory validation (if provided)
	if state.Dir != "" {
		if !filepath.IsAbs(state.Dir) {
			errors = append(errors, ValidationError{
				Field:   "state.dir",
				Value:   state.Dir,
				Message: "state directory path must be absolute",
			})
		}
	}

	// Retention count validation
	if state.RetentionCount <= 0 {
		errors = append(errors, ValidationError{
			Field:   "state.retention_count",
			Value:   state.RetentionCount,
			Message: "retention count must be positive",
		})
	} else if state.RetentionCount > 10000 {
		errors = append(errors, ValidationError{
			Field:   "state.retention_count",
			Value:   state.RetentionCount,
			Message: "retention count cannot exceed 10000",
		})
	}

	return errors
}

// applyWorkspaceDefaults applies default values to workspace configuration.
func applyWorkspaceDefaults(ws *WorkspaceConfig) {
	if ws.Path == "" {
		ws.Path = getDefaultWorkspacePath()
	}
}

// applyExecutorDefaults applies default values to executor configuration.
func applyExecutorDefaults(exec *ExecutorConfig) {
	if exec.Timeout == 0 {
		exec.Timeout = 5 * time.Minute // Default: 5 minutes
	}

	if exec.ConcurrentLimit == 0 {
		// Default: CPU count or 4, whichever is smaller
		cpuCount := runtime.NumCPU()
		if cpuCount > 4 {
			exec.ConcurrentLimit = 4
		} else {
			exec.ConcurrentLimit = cpuCount
		}
		if exec.ConcurrentLimit < 1 {
			exec.ConcurrentLimit = 1
		}
	}
}

// applyIntegrationDefaults applies default values to integration configuration.
func applyIntegrationDefaults(integ *IntegrationConfig) {
	if integ.GitHub.Endpoint == "" {
		integ.GitHub.Endpoint = "https://api.github.com" // Default GitHub endpoint
	}
}

// applyLoggingDefaults applies default values to logging configuration.
func applyLoggingDefaults(log *LoggingConfig) {
	if log.Level == "" {
		log.Level = "info" // Default log level
	}

	if log.Format == "" {
		log.Format = "text" // Default log format
	}

	// Handle verbose and quiet mode implications
	if log.Verbose {
		log.Level = "debug"
	}
	if log.Quiet {
		log.Level = "warn"
	}
}

// applyStateDefaults applies default values to state configuration.
func applyStateDefaults(cfg *Config) {
	if cfg == nil {
		return
	}
	state := &cfg.State

	if state.Dir == "" {
		state.Dir = getDefaultStatePath()
	}

	if state.RetentionCount == 0 {
		state.RetentionCount = 10 // Default: retain 10 state snapshots
	}

	// Default: state persistence is enabled unless explicitly disabled.
	if !cfg.stateEnabledSet() {
		state.Enabled = true
	}
}

// Helper functions

// getDefaultWorkspacePath returns the default workspace directory path.
func getDefaultWorkspacePath() string {
	// Follow XDG Base Directory specification
	if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
		return filepath.Join(xdgCache, "cascade")
	}

	// Fallback to ~/.cache/cascade
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".cache", "cascade")
	}

	// Last resort fallback
	return filepath.Join(os.TempDir(), "cascade")
}

// getDefaultStatePath returns the default state directory path.
func getDefaultStatePath() string {
	// Follow XDG Base Directory specification
	if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
		return filepath.Join(xdgState, "cascade")
	}

	// Fallback to ~/.local/state/cascade
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "cascade")
	}

	// Last resort fallback
	return filepath.Join(os.TempDir(), "cascade-state")
}

// isValidGitHubToken performs basic GitHub token format validation.
func isValidGitHubToken(token string) bool {
	// GitHub tokens have specific prefixes and lengths
	// Personal access tokens: ghp_
	// GitHub App tokens: ghs_
	// OAuth tokens: gho_
	// User-to-server tokens: ghu_
	// Server-to-server tokens: ghr_
	prefixes := []string{"ghp_", "ghs_", "gho_", "ghu_", "ghr_"}

	for _, prefix := range prefixes {
		if strings.HasPrefix(token, prefix) && len(token) >= 36 {
			return true
		}
	}

	// Also accept classic tokens (40 characters, hexadecimal)
	if len(token) == 40 {
		for _, char := range token {
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
		return true
	}

	return false
}

// isValidSlackToken performs basic Slack token format validation.
func isValidSlackToken(token string) bool {
	// Slack tokens have specific prefixes:
	// Bot tokens: xoxb-
	// User tokens: xoxp-
	// App tokens: xapp-
	prefixes := []string{"xoxb-", "xoxp-", "xapp-"}

	for _, prefix := range prefixes {
		if strings.HasPrefix(token, prefix) && len(token) >= 50 {
			return true
		}
	}

	return false
}

// contains checks if a slice contains a specific string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
