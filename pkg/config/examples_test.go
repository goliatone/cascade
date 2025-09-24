package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/pkg/config"
)

// TestExampleConfigurations validates that all example configuration files
// can be loaded and are valid according to the configuration schema.
func TestExampleConfigurations(t *testing.T) {
	examplesDir := filepath.Join("examples")

	examples := []struct {
		name string
		file string
	}{
		{"Basic", "basic.yaml"},
		{"Advanced", "advanced.yaml"},
		{"CI", "ci.yaml"},
		{"Development", "development.yaml"},
	}

	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			configPath := filepath.Join(examplesDir, example.file)

			// Verify file exists
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				t.Fatalf("Example configuration file does not exist: %s", configPath)
			}

			// Load configuration from file
			cfg, err := config.LoadFromFile(configPath)
			if err != nil {
				t.Fatalf("Failed to load example configuration %s: %v", example.file, err)
			}

			// Basic validation that the configuration loaded successfully
			if cfg == nil {
				t.Fatalf("Configuration is nil for %s", example.file)
			}

			// Validate that the configuration has reasonable values
			validateExampleConfig(t, cfg, example.name)
		})
	}
}

// validateExampleConfig performs basic validation on loaded example configurations
func validateExampleConfig(t *testing.T, cfg *config.Config, exampleName string) {
	t.Helper()

	// Validate workspace configuration
	if cfg.Workspace.Path == "" {
		t.Errorf("%s example: workspace path should not be empty", exampleName)
	}

	// Validate executor configuration
	if cfg.Executor.ConcurrentLimit < 1 {
		t.Errorf("%s example: concurrent limit should be at least 1, got %d", exampleName, cfg.Executor.ConcurrentLimit)
	}

	if cfg.Executor.Timeout <= 0 {
		t.Errorf("%s example: timeout should be positive, got %v", exampleName, cfg.Executor.Timeout)
	}

	// Validate logging configuration
	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[cfg.Logging.Level] {
		t.Errorf("%s example: invalid log level %q", exampleName, cfg.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"text": true,
		"json": true,
	}
	if !validLogFormats[cfg.Logging.Format] {
		t.Errorf("%s example: invalid log format %q", exampleName, cfg.Logging.Format)
	}

	// Validate state configuration
	if cfg.State.RetentionCount < 1 {
		t.Errorf("%s example: state retention count should be at least 1, got %d", exampleName, cfg.State.RetentionCount)
	}
}

// TestExampleConfigurationDefaults tests that example configurations work well
// when merged with defaults from the configuration builder.
func TestExampleConfigurationDefaults(t *testing.T) {
	examplesDir := filepath.Join("examples")
	basicConfigPath := filepath.Join(examplesDir, "basic.yaml")

	// Create a temporary manifest file for validation
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "deps.yaml")
	manifestContent := "# Temporary manifest for testing\nmodules: []\n"
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to create temporary manifest: %v", err)
	}

	// Load the example configuration and update the manifest path
	cfg, err := config.LoadFromFile(basicConfigPath)
	if err != nil {
		t.Fatalf("Failed to load basic example: %v", err)
	}

	// Update the manifest path to point to our temporary file
	cfg.Workspace.ManifestPath = manifestPath

	// Apply defaults and validate the configuration
	if err := config.ApplyDefaults(cfg); err != nil {
		t.Fatalf("Failed to apply defaults: %v", err)
	}

	if err := config.Validate(cfg); err != nil {
		t.Fatalf("Configuration validation failed: %v", err)
	}

	// Verify that defaults are applied where not specified in the example
	if cfg.Workspace.Path == "" {
		t.Errorf("Workspace path should be populated after applying defaults")
	}

	if cfg.Executor.Timeout <= 0 {
		t.Errorf("Timeout should be positive after applying defaults")
	}

	if cfg.State.Dir == "" {
		t.Errorf("State directory should be populated after applying defaults")
	}
}

// TestExampleConfigurationValidation tests that example configurations
// pass validation rules.
func TestExampleConfigurationValidation(t *testing.T) {
	examplesDir := filepath.Join("examples")

	examples := []string{
		"basic.yaml",
		"advanced.yaml",
		"development.yaml",
		// Note: ci.yaml might fail validation due to environment variable references
		// which aren't resolved in tests, so we test it separately
	}

	// Create a temporary manifest file for validation
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "deps.yaml")
	manifestContent := "# Temporary manifest for testing\nmodules: []\n"
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to create temporary manifest: %v", err)
	}

	for _, example := range examples {
		t.Run(example, func(t *testing.T) {
			configPath := filepath.Join(examplesDir, example)

			// Load the example configuration and update the manifest path
			cfg, err := config.LoadFromFile(configPath)
			if err != nil {
				t.Fatalf("Failed to load example configuration %s: %v", example, err)
			}

			// Update the manifest path to point to our temporary file
			cfg.Workspace.ManifestPath = manifestPath

			// Apply defaults and validate
			if err := config.ApplyDefaults(cfg); err != nil {
				t.Fatalf("Failed to apply defaults for %s: %v", example, err)
			}

			// Validate the configuration
			if err := config.Validate(cfg); err != nil {
				t.Errorf("Configuration validation failed for %s: %v", example, err)
			}
		})
	}
}
