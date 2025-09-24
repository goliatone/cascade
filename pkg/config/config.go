package config

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Builder orchestrates config assembly from various sources.
// It provides a fluent interface for loading configuration from files,
// environment variables, and command-line flags with proper precedence.
type Builder interface {
	FromEnv() Builder
	FromFlags(cmd *cobra.Command) Builder
	FromFile(path string) Builder
	Build() (*Config, error)
}

// NewBuilder returns a new configuration builder with sensible defaults.
func NewBuilder() Builder {
	return &builder{
		configs: make([]*Config, 0, 3), // Pre-allocate for typical use (file, env, flags)
	}
}

// builder implements the Builder interface with support for multiple configuration sources.
type builder struct {
	configs [](*Config)
	errors  []error
}

// FromEnv loads configuration from environment variables.
// Environment variables are parsed and added to the configuration pipeline.
func (b *builder) FromEnv() Builder {
	envConfig, err := FromEnv()
	if err != nil {
		b.errors = append(b.errors, fmt.Errorf("environment configuration error: %w", err))
		return b
	}

	if envConfig != nil {
		b.configs = append(b.configs, envConfig)
	}

	return b
}

// FromFlags loads configuration from command-line flags.
// Flags have the highest precedence and will override environment and file settings.
func (b *builder) FromFlags(cmd *cobra.Command) Builder {
	if cmd == nil {
		b.errors = append(b.errors, fmt.Errorf("command cannot be nil"))
		return b
	}

	flagConfig, err := LoadFromFlags(cmd)
	if err != nil {
		b.errors = append(b.errors, fmt.Errorf("flag configuration error: %w", err))
		return b
	}

	if flagConfig != nil {
		b.configs = append(b.configs, flagConfig)
	}

	return b
}

// FromFile loads configuration from the specified file path.
// If path is empty, attempts to discover configuration file in standard locations.
func (b *builder) FromFile(path string) Builder {
	var fileConfig *Config
	var err error

	if path == "" {
		// Attempt to discover configuration file
		discoveredPath, discoverErr := DiscoverConfigFile()
		if discoverErr != nil {
			b.errors = append(b.errors, fmt.Errorf("configuration file discovery error: %w", discoverErr))
			return b
		}

		if discoveredPath == "" {
			// No config file found, skip silently (this is normal)
			return b
		}

		fileConfig, err = LoadFromFile(discoveredPath)
	} else {
		fileConfig, err = LoadFromFile(path)
	}

	if err != nil {
		// Check if this is a file not found error for explicit paths
		if path != "" && os.IsNotExist(err) {
			b.errors = append(b.errors, fmt.Errorf("configuration file not found: %s", path))
		} else {
			b.errors = append(b.errors, fmt.Errorf("configuration file error: %w", err))
		}
		return b
	}

	if fileConfig != nil {
		b.configs = append(b.configs, fileConfig)
	}

	return b
}

// Build constructs the final configuration by merging all sources,
// applying defaults, and performing validation.
// Configuration sources are merged with precedence: flags > env > file > defaults
func (b *builder) Build() (*Config, error) {
	// Return any accumulated errors first
	if len(b.errors) > 0 {
		var errorMsg string
		if len(b.errors) == 1 {
			errorMsg = b.errors[0].Error()
		} else {
			errorMsg = fmt.Sprintf("multiple configuration errors: %v", b.errors)
		}
		return nil, fmt.Errorf("configuration build failed: %s", errorMsg)
	}

	// Start with base configuration (provides zero values)
	baseConfig := New()

	// Merge all configuration sources in precedence order
	configs := make([]*Config, 0, len(b.configs)+1)
	configs = append(configs, baseConfig)
	configs = append(configs, b.configs...)

	finalConfig := MergeConfigs(configs...)

	// Apply defaults to fill in any missing values
	if err := ApplyDefaults(finalConfig); err != nil {
		return nil, fmt.Errorf("failed to apply configuration defaults: %w", err)
	}

	// Validate the final configuration
	if err := Validate(finalConfig); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return finalConfig, nil
}

// Convenience functions for common configuration patterns

// New returns a Config populated with safe zero values and defaults applied.
func NewWithDefaults() (*Config, error) {
	config := New()
	if err := ApplyDefaults(config); err != nil {
		return nil, fmt.Errorf("failed to apply defaults: %w", err)
	}
	return config, nil
}

// FromFlags is a convenience function that creates a builder and loads from flags.
func FromFlags(cmd *cobra.Command) (*Config, error) {
	return NewBuilder().FromFlags(cmd).Build()
}

// FromFile is a convenience function that creates a builder and loads from a file.
func FromFile(path string) (*Config, error) {
	return NewBuilder().FromFile(path).Build()
}

// Merge is a convenience function for merging multiple configuration sources.
func Merge(configs ...*Config) (*Config, error) {
	merged := MergeConfigs(configs...)
	if err := ApplyDefaults(merged); err != nil {
		return nil, fmt.Errorf("failed to apply defaults: %w", err)
	}
	if err := Validate(merged); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	return merged, nil
}
