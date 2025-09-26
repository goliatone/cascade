package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvParser provides functionality to parse configuration from environment variables.
// It handles type conversions, validation, and error reporting for all supported
// environment variables defined in the CASCADE_* namespace.
type EnvParser struct {
	// getEnv allows injection of environment variable retrieval for testing
	getEnv func(string) string
}

// NewEnvParser creates a new environment variable parser.
func NewEnvParser() *EnvParser {
	return &EnvParser{
		getEnv: os.Getenv,
	}
}

// NewEnvParserWithGetter creates a new environment variable parser with custom getter.
// This is primarily used for testing with mock environment variables.
func NewEnvParserWithGetter(getter func(string) string) *EnvParser {
	return &EnvParser{
		getEnv: getter,
	}
}

// ParseEnv parses all CASCADE environment variables and returns a populated Config.
// It returns an error if any environment variables contain invalid values.
func (p *EnvParser) ParseEnv() (*Config, error) {
	var errs []string
	config := New()

	// Parse workspace configuration
	if err := p.parseWorkspace(config); err != nil {
		errs = append(errs, err.Error())
	}

	// Parse executor configuration
	if err := p.parseExecutor(config); err != nil {
		errs = append(errs, err.Error())
	}

	// Parse integration configuration
	if err := p.parseIntegration(config); err != nil {
		errs = append(errs, err.Error())
	}

	// Parse logging configuration
	if err := p.parseLogging(config); err != nil {
		errs = append(errs, err.Error())
	}

	// Parse state configuration
	if err := p.parseState(config); err != nil {
		errs = append(errs, err.Error())
	}

	// Parse manifest generator configuration
	if err := p.parseManifestGenerator(config); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("environment variable parsing errors: %s", strings.Join(errs, "; "))
	}

	return config, nil
}

// parseWorkspace parses workspace-related environment variables
func (p *EnvParser) parseWorkspace(config *Config) error {
	if path := p.getEnv(EnvWorkspacePath); path != "" {
		config.Workspace.Path = path
	}

	if tempDir := p.getEnv(EnvTempDir); tempDir != "" {
		config.Workspace.TempDir = tempDir
	}

	if manifestPath := p.getEnv(EnvManifestPath); manifestPath != "" {
		config.Workspace.ManifestPath = manifestPath
	}

	return nil
}

// parseExecutor parses executor-related environment variables
func (p *EnvParser) parseExecutor(config *Config) error {
	var errs []string

	// Parse timeout duration
	if timeoutStr := p.getEnv(EnvTimeout); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvTimeout, err))
		} else {
			config.Executor.Timeout = timeout
		}
	}

	// Parse concurrent limit
	if limitStr := p.getEnv(EnvConcurrentLimit); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: must be a positive integer", EnvConcurrentLimit))
		} else if limit <= 0 {
			errs = append(errs, fmt.Sprintf("invalid %s: must be greater than 0, got %d", EnvConcurrentLimit, limit))
		} else {
			config.Executor.ConcurrentLimit = limit
		}
	}

	// Parse dry run flag
	if dryRunStr := p.getEnv(EnvDryRun); dryRunStr != "" {
		dryRun, err := p.parseBool(dryRunStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvDryRun, err))
		} else {
			config.setExecutorDryRun(dryRun)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("executor configuration errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// parseIntegration parses integration-related environment variables
func (p *EnvParser) parseIntegration(config *Config) error {
	// Parse GitHub configuration
	if token := p.getEnv(EnvGitHubToken); token != "" {
		config.Integration.GitHub.Token = token
	}

	if endpoint := p.getEnv(EnvGitHubEndpoint); endpoint != "" {
		config.Integration.GitHub.Endpoint = endpoint
	}

	if org := p.getEnv(EnvGitHubOrg); org != "" {
		config.Integration.GitHub.Organization = org
	}

	// Parse Slack configuration
	if token := p.getEnv(EnvSlackToken); token != "" {
		config.Integration.Slack.Token = token
	}

	if webhook := p.getEnv(EnvSlackWebhook); webhook != "" {
		config.Integration.Slack.WebhookURL = webhook
	}

	if channel := p.getEnv(EnvSlackChannel); channel != "" {
		config.Integration.Slack.Channel = channel
	}

	return nil
}

// parseLogging parses logging-related environment variables
func (p *EnvParser) parseLogging(config *Config) error {
	var errs []string

	// Parse log level
	if level := p.getEnv(EnvLogLevel); level != "" {
		if !p.isValidLogLevel(level) {
			errs = append(errs, fmt.Sprintf("invalid %s: must be one of [debug, info, warn, error], got %q", EnvLogLevel, level))
		} else {
			config.Logging.Level = level
		}
	}

	// Parse log format
	if format := p.getEnv(EnvLogFormat); format != "" {
		if !p.isValidLogFormat(format) {
			errs = append(errs, fmt.Sprintf("invalid %s: must be one of [text, json], got %q", EnvLogFormat, format))
		} else {
			config.Logging.Format = format
		}
	}

	// Parse verbose flag
	if verboseStr := p.getEnv(EnvVerbose); verboseStr != "" {
		verbose, err := p.parseBool(verboseStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvVerbose, err))
		} else {
			config.setLoggingVerbose(verbose)
		}
	}

	// Parse quiet flag
	if quietStr := p.getEnv(EnvQuiet); quietStr != "" {
		quiet, err := p.parseBool(quietStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvQuiet, err))
		} else {
			config.setLoggingQuiet(quiet)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("logging configuration errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// parseState parses state-related environment variables
func (p *EnvParser) parseState(config *Config) error {
	var errs []string

	// Parse state directory
	if dir := p.getEnv(EnvStateDir); dir != "" {
		config.State.Dir = dir
	}

	// Parse retention count
	if retentionStr := p.getEnv(EnvStateRetention); retentionStr != "" {
		retention, err := strconv.Atoi(retentionStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: must be a positive integer", EnvStateRetention))
		} else if retention <= 0 {
			errs = append(errs, fmt.Sprintf("invalid %s: must be greater than 0, got %d", EnvStateRetention, retention))
		} else {
			config.State.RetentionCount = retention
		}
	}

	// Parse state enabled flag
	if enabledStr := p.getEnv(EnvStateEnabled); enabledStr != "" {
		enabled, err := p.parseBool(enabledStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvStateEnabled, err))
		} else {
			config.setStateEnabled(enabled)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("state configuration errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// parseManifestGenerator parses manifest generator-related environment variables
func (p *EnvParser) parseManifestGenerator(config *Config) error {
	var errs []string

	// Parse default workspace
	if workspace := p.getEnv(EnvManifestGeneratorWorkspace); workspace != "" {
		config.ManifestGenerator.DefaultWorkspace = workspace
	}

	// Parse test command
	if testCmd := p.getEnv(EnvManifestGeneratorTestCommand); testCmd != "" {
		config.ManifestGenerator.Tests.Command = testCmd
	}

	// Parse test timeout
	if timeoutStr := p.getEnv(EnvManifestGeneratorTestTimeout); timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvManifestGeneratorTestTimeout, err))
		} else {
			config.ManifestGenerator.Tests.Timeout = timeout
		}
	}

	// Parse notifications enabled
	if enabledStr := p.getEnv(EnvManifestGeneratorNotificationsEnable); enabledStr != "" {
		enabled, err := p.parseBool(enabledStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvManifestGeneratorNotificationsEnable, err))
		} else {
			config.ManifestGenerator.Notifications.Enabled = enabled
		}
	}

	// Parse default branch
	if branch := p.getEnv(EnvManifestGeneratorDefaultBranch); branch != "" {
		config.ManifestGenerator.DefaultBranch = branch
	}

	// Parse discovery enabled
	if enabledStr := p.getEnv(EnvManifestGeneratorDiscoveryEnabled); enabledStr != "" {
		enabled, err := p.parseBool(enabledStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvManifestGeneratorDiscoveryEnabled, err))
		} else {
			config.ManifestGenerator.Discovery.Enabled = enabled
		}
	}

	// Parse discovery max depth
	if depthStr := p.getEnv(EnvManifestGeneratorDiscoveryMaxDepth); depthStr != "" {
		depth, err := strconv.Atoi(depthStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: must be a positive integer", EnvManifestGeneratorDiscoveryMaxDepth))
		} else if depth <= 0 {
			errs = append(errs, fmt.Sprintf("invalid %s: must be greater than 0, got %d", EnvManifestGeneratorDiscoveryMaxDepth, depth))
		} else {
			config.ManifestGenerator.Discovery.MaxDepth = depth
		}
	}

	// Parse discovery interactive
	if interactiveStr := p.getEnv(EnvManifestGeneratorDiscoveryInteractive); interactiveStr != "" {
		interactive, err := p.parseBool(interactiveStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("invalid %s: %v", EnvManifestGeneratorDiscoveryInteractive, err))
		} else {
			config.ManifestGenerator.Discovery.Interactive = interactive
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("manifest generator configuration errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// parseBool parses a boolean value from a string, supporting multiple formats
func (p *EnvParser) parseBool(value string) (bool, error) {
	lower := strings.ToLower(strings.TrimSpace(value))

	switch lower {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled", "":
		return false, nil
	default:
		return false, fmt.Errorf("must be one of [true, false, 1, 0, yes, no, on, off, enabled, disabled], got %q", value)
	}
}

// parseStringList parses a comma-separated list of strings
func (p *EnvParser) parseStringList(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// isValidLogLevel checks if the given log level is valid
func (p *EnvParser) isValidLogLevel(level string) bool {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

// isValidLogFormat checks if the given log format is valid
func (p *EnvParser) isValidLogFormat(format string) bool {
	switch strings.ToLower(format) {
	case "text", "json":
		return true
	default:
		return false
	}
}

// FromEnv is a convenience function that creates a new parser and parses the environment.
func FromEnv() (*Config, error) {
	parser := NewEnvParser()
	return parser.ParseEnv()
}

// LoadFromEnv returns configuration derived from environment variables.
// This is a backward-compatible function that uses the new EnvParser.
func LoadFromEnv() (*Config, error) {
	return FromEnv()
}
