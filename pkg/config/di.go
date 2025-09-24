package config

import (
	"fmt"
	"log/slog"
	"os"
)

// DIContainerConfig represents configuration needed for dependency injection setup.
// It provides configuration-driven service factory functions and feature toggles.
type DIContainerConfig struct {
	// Config is the complete configuration for the container
	Config *Config

	// Logger is the configured logger for the container
	Logger *slog.Logger

	// ServiceSelection allows configuration-driven service implementation selection
	ServiceSelection ServiceSelectionConfig

	// FeatureToggles enables configuration-based feature flags
	FeatureToggles FeatureToggleConfig

	// Debug enables dependency injection debugging and inspection
	Debug bool
}

// ServiceSelectionConfig controls which implementations are selected for services
// based on configuration settings.
type ServiceSelectionConfig struct {
	// GitImplementation selects the Git implementation to use
	// Valid values: "native", "go-git"
	GitImplementation string `json:"git_implementation" yaml:"git_implementation"`

	// StateBackend selects the state persistence backend
	// Valid values: "filesystem", "memory"
	StateBackend string `json:"state_backend" yaml:"state_backend"`

	// HTTPClient selects the HTTP client implementation
	// Valid values: "default", "custom"
	HTTPClient string `json:"http_client" yaml:"http_client"`
}

// FeatureToggleConfig enables configuration-based feature flags for service behavior.
type FeatureToggleConfig struct {
	// EnableMetrics enables metrics collection for services
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`

	// EnableRetries enables retry logic for external service calls
	EnableRetries bool `json:"enable_retries" yaml:"enable_retries"`

	// EnableCaching enables caching for service operations
	EnableCaching bool `json:"enable_caching" yaml:"enable_caching"`

	// EnableValidation enables additional validation in services
	EnableValidation bool `json:"enable_validation" yaml:"enable_validation"`
}

// ConfiguredContainer creates a DI container configuration from the given config.
// It applies configuration-driven service selection and feature toggles.
func ConfiguredContainer(config *Config) (*DIContainerConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Validate required configuration for DI
	if err := validateDIRequirements(config); err != nil {
		return nil, fmt.Errorf("configuration validation for DI failed: %w", err)
	}

	// Create logger based on configuration
	logger, err := createConfiguredLogger(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create configured logger: %w", err)
	}

	// Apply default service selections based on configuration
	serviceSelection := applyServiceSelectionDefaults(config)

	// Apply default feature toggles based on configuration
	featureToggles := applyFeatureToggleDefaults(config)

	return &DIContainerConfig{
		Config:           config,
		Logger:           logger,
		ServiceSelection: serviceSelection,
		FeatureToggles:   featureToggles,
		Debug:            config.Logging.Level == "debug",
	}, nil
}

// validateDIRequirements ensures the configuration has all required settings for DI.
func validateDIRequirements(config *Config) error {
	var errors []string

	// Validate workspace configuration
	if config.Workspace.Path == "" {
		errors = append(errors, "workspace path is required for DI container")
	}

	// Validate state configuration if enabled
	if config.State.Enabled && config.State.Dir == "" {
		errors = append(errors, "state directory is required when state persistence is enabled")
	}

	// Validate executor configuration
	if config.Executor.Timeout <= 0 {
		errors = append(errors, "executor timeout must be positive")
	}

	if config.Executor.ConcurrentLimit <= 0 {
		errors = append(errors, "executor concurrent limit must be positive")
	}

	if len(errors) > 0 {
		return fmt.Errorf("configuration validation errors: %v", errors)
	}

	return nil
}

// createConfiguredLogger creates a logger instance based on the logging configuration.
func createConfiguredLogger(config *Config) (*slog.Logger, error) {
	var level slog.Level
	var handler slog.Handler

	// Parse log level
	switch config.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Create handler based on format
	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	switch config.Logging.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, handlerOpts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, handlerOpts)
	default:
		handler = slog.NewTextHandler(os.Stdout, handlerOpts)
	}

	return slog.New(handler), nil
}

// applyServiceSelectionDefaults applies default service selections based on configuration.
func applyServiceSelectionDefaults(config *Config) ServiceSelectionConfig {
	selection := ServiceSelectionConfig{
		GitImplementation: "native",     // Default to native git
		StateBackend:      "filesystem", // Default to filesystem state
		HTTPClient:        "default",    // Default HTTP client
	}

	// Override based on configuration hints
	if config.Executor.DryRun {
		// In dry-run mode, prefer memory-based backends
		selection.StateBackend = "memory"
	}

	// If GitHub token is provided, use default HTTP client
	if config.Integration.GitHub.Token != "" {
		selection.HTTPClient = "default"
	}

	return selection
}

// applyFeatureToggleDefaults applies default feature toggles based on configuration.
func applyFeatureToggleDefaults(config *Config) FeatureToggleConfig {
	toggles := FeatureToggleConfig{
		EnableMetrics:    config.Logging.Level == "debug", // Enable metrics in debug mode
		EnableRetries:    true,                            // Enable retries by default
		EnableCaching:    !config.Executor.DryRun,         // Disable caching in dry-run mode
		EnableValidation: true,                            // Enable validation by default
	}

	// Disable features that don't make sense in dry-run mode
	if config.Executor.DryRun {
		toggles.EnableMetrics = false
		toggles.EnableCaching = false
	}

	return toggles
}

// GetWorkspaceConfig returns workspace-specific configuration for services.
func (d *DIContainerConfig) GetWorkspaceConfig() WorkspaceConfig {
	return d.Config.Workspace
}

// GetExecutorConfig returns executor-specific configuration for services.
func (d *DIContainerConfig) GetExecutorConfig() ExecutorConfig {
	return d.Config.Executor
}

// GetIntegrationConfig returns integration-specific configuration for services.
func (d *DIContainerConfig) GetIntegrationConfig() IntegrationConfig {
	return d.Config.Integration
}

// GetLoggingConfig returns logging-specific configuration for services.
func (d *DIContainerConfig) GetLoggingConfig() LoggingConfig {
	return d.Config.Logging
}

// GetStateConfig returns state-specific configuration for services.
func (d *DIContainerConfig) GetStateConfig() StateConfig {
	return d.Config.State
}

// IsDebugEnabled returns whether DI debugging is enabled.
func (d *DIContainerConfig) IsDebugEnabled() bool {
	return d.Debug
}

// GetServiceSelection returns the service selection configuration.
func (d *DIContainerConfig) GetServiceSelection() ServiceSelectionConfig {
	return d.ServiceSelection
}

// GetFeatureToggles returns the feature toggle configuration.
func (d *DIContainerConfig) GetFeatureToggles() FeatureToggleConfig {
	return d.FeatureToggles
}

// DebugInfo returns debugging information about the DI configuration.
func (d *DIContainerConfig) DebugInfo() map[string]interface{} {
	if !d.Debug {
		return map[string]interface{}{
			"debug": "disabled",
		}
	}

	return map[string]interface{}{
		"workspace_path":     d.Config.Workspace.Path,
		"manifest_path":      d.Config.Workspace.ManifestPath,
		"executor_timeout":   d.Config.Executor.Timeout.String(),
		"concurrent_limit":   d.Config.Executor.ConcurrentLimit,
		"dry_run":            d.Config.Executor.DryRun,
		"log_level":          d.Config.Logging.Level,
		"log_format":         d.Config.Logging.Format,
		"state_enabled":      d.Config.State.Enabled,
		"state_dir":          d.Config.State.Dir,
		"git_implementation": d.ServiceSelection.GitImplementation,
		"state_backend":      d.ServiceSelection.StateBackend,
		"http_client":        d.ServiceSelection.HTTPClient,
		"enable_metrics":     d.FeatureToggles.EnableMetrics,
		"enable_retries":     d.FeatureToggles.EnableRetries,
		"enable_caching":     d.FeatureToggles.EnableCaching,
		"enable_validation":  d.FeatureToggles.EnableValidation,
	}
}
