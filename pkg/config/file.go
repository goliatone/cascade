package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type rawFileConfig struct {
	Executor struct {
		DryRun *bool `json:"dry_run" yaml:"dry_run"`
	} `json:"executor" yaml:"executor"`
	Logging struct {
		Verbose *bool `json:"verbose" yaml:"verbose"`
		Quiet   *bool `json:"quiet" yaml:"quiet"`
	} `json:"logging" yaml:"logging"`
	State struct {
		Enabled *bool `json:"enabled" yaml:"enabled"`
	} `json:"state" yaml:"state"`
}

func applyRawBoolFlags(cfg *Config, raw *rawFileConfig) {
	if cfg == nil || raw == nil {
		return
	}

	if raw.Executor.DryRun != nil {
		cfg.setExecutorDryRun(*raw.Executor.DryRun)
	}

	if raw.Logging.Verbose != nil {
		cfg.setLoggingVerbose(*raw.Logging.Verbose)
	}

	if raw.Logging.Quiet != nil {
		cfg.setLoggingQuiet(*raw.Logging.Quiet)
	}

	if raw.State.Enabled != nil {
		cfg.setStateEnabled(*raw.State.Enabled)
	}
}

// ConfigFileLocations returns standard locations where configuration files are searched.
// Search order follows XDG Base Directory Specification with fallbacks.
func ConfigFileLocations() []string {
	home := os.Getenv("HOME")
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")

	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}

	locations := []string{
		".cascade.yaml",
		".cascade.yml",
		".cascade.json",
	}

	if xdgConfig != "" {
		locations = append(locations,
			filepath.Join(xdgConfig, "cascade", "config.yaml"),
			filepath.Join(xdgConfig, "cascade", "config.yml"),
			filepath.Join(xdgConfig, "cascade", "config.json"),
		)
	}

	if home != "" {
		locations = append(locations,
			filepath.Join(home, ".config", "cascade", "config.yaml"),
			filepath.Join(home, ".config", "cascade", "config.yml"),
			filepath.Join(home, ".config", "cascade", "config.json"),
		)
	}

	return locations
}

// DiscoverConfigFile searches for configuration files in standard locations.
// Returns the path to the first configuration file found, or empty string if none found.
func DiscoverConfigFile() (string, error) {
	locations := ConfigFileLocations()

	for _, path := range locations {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", nil // No config file found, but not an error
}

// LoadFromFile reads configuration from the provided path.
// Supports YAML, JSON, and TOML formats based on file extension.
func LoadFromFile(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config file path cannot be empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	config := New()
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		var raw rawFileConfig
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config file %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config file %s: %w", path, err)
		}
		applyRawBoolFlags(config, &raw)
	case ".json":
		var raw rawFileConfig
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config file %s: %w", path, err)
		}
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse JSON config file %s: %w", path, err)
		}
		applyRawBoolFlags(config, &raw)
	default:
		return nil, fmt.Errorf("unsupported config file format: %s (supported: .yaml, .yml, .json)", ext)
	}

	if err := validateConfigFile(config); err != nil {
		return nil, fmt.Errorf("invalid config file %s: %w", path, err)
	}

	return config, nil
}

// LoadFromFileOrDiscover loads configuration from the specified path,
// or discovers and loads from standard locations if path is empty.
func LoadFromFileOrDiscover(path string) (*Config, error) {
	if path != "" {
		return LoadFromFile(path)
	}

	discoveredPath, err := DiscoverConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to discover config file: %w", err)
	}

	if discoveredPath == "" {
		// No config file found, return empty config (will use defaults/env/flags)
		return New(), nil
	}

	return LoadFromFile(discoveredPath)
}

// MergeConfigs merges multiple configuration sources with precedence order.
// Later configs in the slice take precedence over earlier ones.
// Non-zero values from higher precedence configs override lower precedence values.
func MergeConfigs(configs ...*Config) *Config {
	if len(configs) == 0 {
		return New()
	}

	result := New()

	for _, config := range configs {
		if config == nil {
			continue
		}
		mergeConfig(result, config)
	}

	return result
}

// mergeConfig merges src into dst, with src taking precedence for non-zero values
func mergeConfig(dst, src *Config) {
	// Workspace config
	if src.Workspace.Path != "" {
		dst.Workspace.Path = src.Workspace.Path
	}
	if src.Workspace.TempDir != "" {
		dst.Workspace.TempDir = src.Workspace.TempDir
	}
	if src.Workspace.ManifestPath != "" {
		dst.Workspace.ManifestPath = src.Workspace.ManifestPath
	}

	// Executor config
	if src.Executor.Timeout != 0 {
		dst.Executor.Timeout = src.Executor.Timeout
	}
	if src.Executor.ConcurrentLimit != 0 {
		dst.Executor.ConcurrentLimit = src.Executor.ConcurrentLimit
	}
	if src.executorDryRunSet() {
		dst.setExecutorDryRun(src.Executor.DryRun)
	}

	// Integration config - GitHub
	if src.Integration.GitHub.Token != "" {
		dst.Integration.GitHub.Token = src.Integration.GitHub.Token
	}
	if src.Integration.GitHub.Endpoint != "" {
		dst.Integration.GitHub.Endpoint = src.Integration.GitHub.Endpoint
	}
	if src.Integration.GitHub.Organization != "" {
		dst.Integration.GitHub.Organization = src.Integration.GitHub.Organization
	}

	// Integration config - Slack
	if src.Integration.Slack.Token != "" {
		dst.Integration.Slack.Token = src.Integration.Slack.Token
	}
	if src.Integration.Slack.WebhookURL != "" {
		dst.Integration.Slack.WebhookURL = src.Integration.Slack.WebhookURL
	}
	if src.Integration.Slack.Channel != "" {
		dst.Integration.Slack.Channel = src.Integration.Slack.Channel
	}

	// Logging config
	if src.Logging.Level != "" {
		dst.Logging.Level = src.Logging.Level
	}
	if src.Logging.Format != "" {
		dst.Logging.Format = src.Logging.Format
	}
	if src.loggingVerboseSet() {
		dst.setLoggingVerbose(src.Logging.Verbose)
	}
	if src.loggingQuietSet() {
		dst.setLoggingQuiet(src.Logging.Quiet)
	}

	// State config
	if src.State.Dir != "" {
		dst.State.Dir = src.State.Dir
	}
	if src.State.RetentionCount != 0 {
		dst.State.RetentionCount = src.State.RetentionCount
	}
	if src.stateEnabledSet() {
		dst.setStateEnabled(src.State.Enabled)
	}

	// ManifestGenerator config
	if src.ManifestGenerator.DefaultWorkspace != "" {
		dst.ManifestGenerator.DefaultWorkspace = src.ManifestGenerator.DefaultWorkspace
	}
	if src.ManifestGenerator.DefaultBranch != "" {
		dst.ManifestGenerator.DefaultBranch = src.ManifestGenerator.DefaultBranch
	}

	// ManifestGenerator tests config
	if src.ManifestGenerator.Tests.Command != "" {
		dst.ManifestGenerator.Tests.Command = src.ManifestGenerator.Tests.Command
	}
	if src.ManifestGenerator.Tests.Timeout != 0 {
		dst.ManifestGenerator.Tests.Timeout = src.ManifestGenerator.Tests.Timeout
	}
	if src.ManifestGenerator.Tests.WorkingDirectory != "" {
		dst.ManifestGenerator.Tests.WorkingDirectory = src.ManifestGenerator.Tests.WorkingDirectory
	}

	// ManifestGenerator notifications config
	if src.ManifestGenerator.Notifications.Enabled {
		dst.ManifestGenerator.Notifications.Enabled = src.ManifestGenerator.Notifications.Enabled
	}
	if len(src.ManifestGenerator.Notifications.Channels) > 0 {
		dst.ManifestGenerator.Notifications.Channels = src.ManifestGenerator.Notifications.Channels
	}
	if src.ManifestGenerator.Notifications.OnSuccess {
		dst.ManifestGenerator.Notifications.OnSuccess = src.ManifestGenerator.Notifications.OnSuccess
	}
	if src.ManifestGenerator.Notifications.OnFailure {
		dst.ManifestGenerator.Notifications.OnFailure = src.ManifestGenerator.Notifications.OnFailure
	}

	// ManifestGenerator discovery config
	if src.ManifestGenerator.Discovery.Enabled {
		dst.ManifestGenerator.Discovery.Enabled = src.ManifestGenerator.Discovery.Enabled
	}
	if src.ManifestGenerator.Discovery.MaxDepth != 0 {
		dst.ManifestGenerator.Discovery.MaxDepth = src.ManifestGenerator.Discovery.MaxDepth
	}
	if len(src.ManifestGenerator.Discovery.IncludePatterns) > 0 {
		dst.ManifestGenerator.Discovery.IncludePatterns = src.ManifestGenerator.Discovery.IncludePatterns
	}
	if len(src.ManifestGenerator.Discovery.ExcludePatterns) > 0 {
		dst.ManifestGenerator.Discovery.ExcludePatterns = src.ManifestGenerator.Discovery.ExcludePatterns
	}
	if src.ManifestGenerator.Discovery.Interactive {
		dst.ManifestGenerator.Discovery.Interactive = src.ManifestGenerator.Discovery.Interactive
	}

	// ManifestGenerator GitHub discovery config
	if src.ManifestGenerator.Discovery.GitHub.Enabled {
		dst.ManifestGenerator.Discovery.GitHub.Enabled = src.ManifestGenerator.Discovery.GitHub.Enabled
	}
	if src.ManifestGenerator.Discovery.GitHub.Organization != "" {
		dst.ManifestGenerator.Discovery.GitHub.Organization = src.ManifestGenerator.Discovery.GitHub.Organization
	}
	if len(src.ManifestGenerator.Discovery.GitHub.IncludePatterns) > 0 {
		dst.ManifestGenerator.Discovery.GitHub.IncludePatterns = src.ManifestGenerator.Discovery.GitHub.IncludePatterns
	}
	if len(src.ManifestGenerator.Discovery.GitHub.ExcludePatterns) > 0 {
		dst.ManifestGenerator.Discovery.GitHub.ExcludePatterns = src.ManifestGenerator.Discovery.GitHub.ExcludePatterns
	}

	// ManifestGenerator template profiles
	if len(src.ManifestGenerator.TemplateProfiles) > 0 {
		if dst.ManifestGenerator.TemplateProfiles == nil {
			dst.ManifestGenerator.TemplateProfiles = make(map[string]TemplateProfileConfig)
		}
		for name, profile := range src.ManifestGenerator.TemplateProfiles {
			dst.ManifestGenerator.TemplateProfiles[name] = profile
		}
	}
}

// validateConfigFile performs basic validation on configuration loaded from files.
// This is a subset of full validation focusing on format and type consistency.
func validateConfigFile(config *Config) error {
	var errors []string

	// Validate logging level
	if config.Logging.Level != "" {
		validLevels := []string{"debug", "info", "warn", "error"}
		valid := false
		for _, level := range validLevels {
			if config.Logging.Level == level {
				valid = true
				break
			}
		}
		if !valid {
			errors = append(errors, fmt.Sprintf("invalid logging level '%s', must be one of: %s",
				config.Logging.Level, strings.Join(validLevels, ", ")))
		}
	}

	// Validate logging format
	if config.Logging.Format != "" {
		validFormats := []string{"text", "json"}
		valid := false
		for _, format := range validFormats {
			if config.Logging.Format == format {
				valid = true
				break
			}
		}
		if !valid {
			errors = append(errors, fmt.Sprintf("invalid logging format '%s', must be one of: %s",
				config.Logging.Format, strings.Join(validFormats, ", ")))
		}
	}

	// Validate executor settings
	if config.Executor.ConcurrentLimit < 0 {
		errors = append(errors, "concurrent_limit must be positive")
	}

	if config.Executor.Timeout < 0 {
		errors = append(errors, "timeout must be positive")
	}

	// Validate state settings
	if config.State.RetentionCount < 0 {
		errors = append(errors, "state retention_count must be positive")
	}

	// Validate paths exist if specified (basic check)
	if config.Workspace.Path != "" {
		if !filepath.IsAbs(config.Workspace.Path) && !strings.HasPrefix(config.Workspace.Path, "~") {
			// Allow relative paths for now, but warn about absolute paths being preferred
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}

	return nil
}
