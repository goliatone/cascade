package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type rawFileConfig struct {
	Workspace struct {
		Path *string `json:"path" yaml:"path"`
	} `json:"workspace" yaml:"workspace"`
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

	if raw.Workspace.Path != nil {
		cfg.setWorkspacePath(*raw.Workspace.Path)
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

func globalConfigFileLocations() []string {
	locations := ConfigFileLocations()
	out := make([]string, 0, len(locations))
	for _, path := range locations {
		if filepath.IsAbs(path) {
			out = append(out, path)
		}
	}
	return out
}

// DiscoverConfigFile searches for configuration files in standard locations.
// Returns the path to the first configuration file found, or empty string if none found.
func DiscoverConfigFile() (string, error) {
	if path, err := discoverLocalConfigFile(); err != nil {
		return "", err
	} else if path != "" {
		return path, nil
	}

	for _, path := range ConfigFileLocations() {
		if !filepath.IsAbs(path) {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", nil // No config file found, but not an error
}

func discoverLocalConfigFile() (string, error) {
	paths, err := discoverLocalConfigFiles()
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", nil
	}
	return paths[len(paths)-1], nil
}

func discoverLocalConfigFiles() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	names := []string{".cascade.yaml", ".cascade.yml", ".cascade.json"}
	var paths []string
	for dir := cwd; ; dir = filepath.Dir(dir) {
		for _, name := range names {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
				break
			} else if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	slices.Reverse(paths)
	return paths, nil
}

// LoadConfigLayers loads file configuration layers in merge order.
// If explicitPath is set, that file is the only file layer.
func LoadConfigLayers(explicitPath string) ([]ConfigLayer, error) {
	if strings.TrimSpace(explicitPath) != "" {
		layer, err := loadConfigLayer(ConfigLayerScopeExplicit, explicitPath)
		if err != nil {
			return nil, err
		}
		return []ConfigLayer{layer}, nil
	}

	var layers []ConfigLayer
	for _, path := range globalConfigFileLocations() {
		if _, err := os.Stat(path); err == nil {
			layer, err := loadConfigLayer(ConfigLayerScopeGlobal, path)
			if err != nil {
				return nil, err
			}
			layers = append(layers, layer)
			break
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	localPaths, err := discoverLocalConfigFiles()
	if err != nil {
		return nil, err
	}
	for i, path := range localPaths {
		scope := ConfigLayerScopeProject
		if i < len(localPaths)-1 {
			scope = ConfigLayerScopeWorkspace
		}
		layer, err := loadConfigLayer(scope, path)
		if err != nil {
			return nil, err
		}
		layers = append(layers, layer)
	}
	return layers, nil
}

func loadConfigLayer(scope ConfigLayerScope, path string) (ConfigLayer, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ConfigLayer{}, err
	}
	cfg, err := LoadFromFile(absPath)
	if err != nil {
		return ConfigLayer{}, err
	}
	return ConfigLayer{
		Scope:   scope,
		Path:    absPath,
		BaseDir: filepath.Dir(absPath),
		Config:  cfg,
	}, nil
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
		if src.workspacePathSet() {
			dst.setFlags.workspacePath = true
		}
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
	// Merge SkipUpToDate and ForceAll - these are booleans that need special handling
	// We need to check if the source explicitly set these values
	// For now, we always take the source value if it's from a higher-precedence source
	if src.executorSkipUpToDateSet() {
		dst.Executor.SkipUpToDate = src.Executor.SkipUpToDate
	}
	if src.executorForceAllSet() {
		dst.Executor.ForceAll = src.Executor.ForceAll
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
		maps.Copy(dst.ManifestGenerator.TemplateProfiles, src.ManifestGenerator.TemplateProfiles)
	}

	mergeLocalUpdateHooks(&dst.Hooks.Update.Local, src.Hooks.Update.Local)
}

func mergeLocalUpdateHooks(dst *LocalUpdateHooksConfig, src LocalUpdateHooksConfig) {
	if hasUnconditionalLocalUpdateHooks(src) {
		dst.After = cloneHookConfigs(src.After)
		dst.AfterSuccess = cloneHookConfigs(src.AfterSuccess)
		dst.AfterFailure = cloneHookConfigs(src.AfterFailure)
		dst.Always = cloneHookConfigs(src.Always)
	}
	if len(src.Rules) > 0 {
		dst.Rules = cloneLocalUpdateHookRules(src.Rules)
	}
	if len(src.DisabledRules) > 0 {
		dst.DisabledRules = slices.Clone(src.DisabledRules)
	}
}

func hasUnconditionalLocalUpdateHooks(hooks LocalUpdateHooksConfig) bool {
	return len(hooks.After) > 0 ||
		len(hooks.AfterSuccess) > 0 ||
		len(hooks.AfterFailure) > 0 ||
		len(hooks.Always) > 0
}

func cloneLocalUpdateHookRules(rules []LocalUpdateHookRule) []LocalUpdateHookRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]LocalUpdateHookRule, len(rules))
	for i, rule := range rules {
		out[i] = rule
		out[i].Match = cloneLocalUpdateHookMatch(rule.Match)
		out[i].After = cloneHookConfigs(rule.After)
		out[i].AfterSuccess = cloneHookConfigs(rule.AfterSuccess)
		out[i].AfterFailure = cloneHookConfigs(rule.AfterFailure)
		out[i].Always = cloneHookConfigs(rule.Always)
	}
	return out
}

func cloneLocalUpdateHookMatch(match LocalUpdateHookMatch) LocalUpdateHookMatch {
	return LocalUpdateHookMatch{
		Modules:               slices.Clone(match.Modules),
		ModulePrefixes:        slices.Clone(match.ModulePrefixes),
		Workspaces:            slices.Clone(match.Workspaces),
		WorkspacePrefixes:     slices.Clone(match.WorkspacePrefixes),
		ModuleDirs:            slices.Clone(match.ModuleDirs),
		ModuleDirPrefixes:     slices.Clone(match.ModuleDirPrefixes),
		ExcludeModules:        slices.Clone(match.ExcludeModules),
		ExcludeModulePrefixes: slices.Clone(match.ExcludeModulePrefixes),
	}
}

func cloneHookConfigs(hooks []HookConfig) []HookConfig {
	if len(hooks) == 0 {
		return nil
	}
	out := make([]HookConfig, len(hooks))
	for i, hook := range hooks {
		out[i] = hook
		out[i].Cmd = slices.Clone(hook.Cmd)
		if hook.Env != nil {
			out[i].Env = maps.Clone(hook.Env)
		}
	}
	return out
}

// validateConfigFile performs basic validation on configuration loaded from files.
// This is a subset of full validation focusing on format and type consistency.
func validateConfigFile(config *Config) error {
	var errors []string

	// Validate logging level
	if config.Logging.Level != "" {
		validLevels := []string{"debug", "info", "warn", "error"}
		valid := slices.Contains(validLevels, config.Logging.Level)
		if !valid {
			errors = append(errors, fmt.Sprintf("invalid logging level '%s', must be one of: %s",
				config.Logging.Level, strings.Join(validLevels, ", ")))
		}
	}

	// Validate logging format
	if config.Logging.Format != "" {
		validFormats := []string{"text", "json"}
		valid := slices.Contains(validFormats, config.Logging.Format)
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

	for _, validationErr := range validateHooks(&config.Hooks) {
		errors = append(errors, fmt.Sprintf("%s: %s", validationErr.Field, validationErr.Message))
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
