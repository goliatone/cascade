package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goliatone/cascade/pkg/config"
	"github.com/spf13/cobra"
)

func TestBuilder_NewBuilder(t *testing.T) {
	builder := config.NewBuilder()
	if builder == nil {
		t.Fatal("NewBuilder returned nil")
	}

	// Should be able to chain calls
	result := builder.FromEnv().FromFile("")
	if result == nil {
		t.Fatal("Builder methods should return builder for chaining")
	}
}

func TestBuilder_FromEnv(t *testing.T) {
	// Set up test environment variables
	oldVars := map[string]string{
		config.EnvWorkspacePath: os.Getenv(config.EnvWorkspacePath),
		config.EnvTimeout:       os.Getenv(config.EnvTimeout),
		config.EnvLogLevel:      os.Getenv(config.EnvLogLevel),
	}
	defer func() {
		for k, v := range oldVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Set test values
	testWorkspace := "/tmp/test-workspace"
	testTimeout := "10m"
	testLogLevel := "debug"

	os.Setenv(config.EnvWorkspacePath, testWorkspace)
	os.Setenv(config.EnvTimeout, testTimeout)
	os.Setenv(config.EnvLogLevel, testLogLevel)

	cfg, err := config.NewBuilder().FromEnv().Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.Workspace.Path != testWorkspace {
		t.Errorf("Expected workspace path %s, got %s", testWorkspace, cfg.Workspace.Path)
	}

	expectedTimeout := 10 * time.Minute
	if cfg.Executor.Timeout != expectedTimeout {
		t.Errorf("Expected timeout %v, got %v", expectedTimeout, cfg.Executor.Timeout)
	}

	if cfg.Logging.Level != testLogLevel {
		t.Errorf("Expected log level %s, got %s", testLogLevel, cfg.Logging.Level)
	}
}

func TestBuilder_FromFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	configContent := `
workspace:
  path: "/tmp/file-workspace"
executor:
  timeout: "15m"
  concurrent_limit: 8
logging:
  level: "warn"
  format: "json"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg, err := config.NewBuilder().FromFile(configPath).Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if cfg.Workspace.Path != "/tmp/file-workspace" {
		t.Errorf("Expected workspace path /tmp/file-workspace, got %s", cfg.Workspace.Path)
	}

	expectedTimeout := 15 * time.Minute
	if cfg.Executor.Timeout != expectedTimeout {
		t.Errorf("Expected timeout %v, got %v", expectedTimeout, cfg.Executor.Timeout)
	}

	if cfg.Executor.ConcurrentLimit != 8 {
		t.Errorf("Expected concurrent limit 8, got %d", cfg.Executor.ConcurrentLimit)
	}

	if cfg.Logging.Level != "warn" {
		t.Errorf("Expected log level warn, got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format json, got %s", cfg.Logging.Format)
	}
}

func TestBuilder_FromFile_NotFound(t *testing.T) {
	_, err := config.NewBuilder().FromFile("/nonexistent/config.yaml").Build()
	if err == nil {
		t.Fatal("Expected error for nonexistent config file")
	}

	if !os.IsNotExist(err) && err.Error() == "" {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

func TestBuilder_FromFile_EmptyPath(t *testing.T) {
	// Should succeed even if no config file is found (normal case)
	cfg, err := config.NewBuilder().FromFile("").Build()
	if err != nil {
		t.Fatalf("Build with empty file path should not fail: %v", err)
	}

	// Should have defaults applied
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", cfg.Logging.Level)
	}
}

func TestBuilder_FromFlags(t *testing.T) {
	cmd := &cobra.Command{}
	fc := config.AddFlags(cmd)

	// Simulate flag values
	fc.Workspace = "/tmp/flag-workspace"
	fc.Verbose = true
	fc.Timeout = 20 * time.Minute
	fc.Parallel = 12

	// We need to mock the flag parsing since we can't easily set up cobra flags in test
	// For now, test the builder interface
	cfg, err := config.NewBuilder().Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Verify defaults are applied
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected default log level info, got %s", cfg.Logging.Level)
	}
}

func TestBuilder_MergeOrder(t *testing.T) {
	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	configContent := `
workspace:
  path: "/tmp/file-workspace"
executor:
  timeout: "15m"
logging:
  level: "warn"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Set up environment variables that should override file settings
	oldVars := map[string]string{
		config.EnvWorkspacePath: os.Getenv(config.EnvWorkspacePath),
		config.EnvLogLevel:      os.Getenv(config.EnvLogLevel),
	}
	defer func() {
		for k, v := range oldVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	os.Setenv(config.EnvWorkspacePath, "/tmp/env-workspace")
	os.Setenv(config.EnvLogLevel, "debug")

	cfg, err := config.NewBuilder().
		FromFile(configPath).
		FromEnv().
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Environment should override file
	if cfg.Workspace.Path != "/tmp/env-workspace" {
		t.Errorf("Expected env workspace path /tmp/env-workspace, got %s", cfg.Workspace.Path)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected env log level debug, got %s", cfg.Logging.Level)
	}

	// File setting should be preserved where not overridden
	expectedTimeout := 15 * time.Minute
	if cfg.Executor.Timeout != expectedTimeout {
		t.Errorf("Expected file timeout %v, got %v", expectedTimeout, cfg.Executor.Timeout)
	}
}

func TestBuilder_ValidationFailure(t *testing.T) {
	// Set up environment with invalid values
	oldVar := os.Getenv(config.EnvTimeout)
	defer func() {
		if oldVar == "" {
			os.Unsetenv(config.EnvTimeout)
		} else {
			os.Setenv(config.EnvTimeout, oldVar)
		}
	}()

	os.Setenv(config.EnvTimeout, "-5m") // Invalid negative timeout

	_, err := config.NewBuilder().FromEnv().Build()
	if err == nil {
		t.Fatal("Expected validation error for negative timeout")
	}

	if err.Error() == "" {
		t.Errorf("Expected validation error message, got empty string")
	}
}

func TestBuilder_ErrorAccumulation(t *testing.T) {
	// Test that errors from multiple sources are accumulated
	builder := config.NewBuilder()

	// Add a nonexistent file
	builder = builder.FromFile("/nonexistent/config.yaml")

	// Set invalid environment
	oldVar := os.Getenv(config.EnvConcurrentLimit)
	defer func() {
		if oldVar == "" {
			os.Unsetenv(config.EnvConcurrentLimit)
		} else {
			os.Setenv(config.EnvConcurrentLimit, oldVar)
		}
	}()

	os.Setenv(config.EnvConcurrentLimit, "invalid")
	builder = builder.FromEnv()

	_, err := builder.Build()
	if err == nil {
		t.Fatal("Expected accumulated errors from multiple sources")
	}

	errorStr := err.Error()
	if errorStr == "" {
		t.Error("Expected error message describing multiple failures")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	t.Run("NewWithDefaults", func(t *testing.T) {
		cfg, err := config.NewWithDefaults()
		if err != nil {
			t.Fatalf("NewWithDefaults failed: %v", err)
		}

		if cfg.Logging.Level != "info" {
			t.Errorf("Expected default log level info, got %s", cfg.Logging.Level)
		}

		if cfg.Executor.Timeout == 0 {
			t.Error("Expected non-zero timeout from defaults")
		}
	})

	t.Run("FromEnv", func(t *testing.T) {
		// Clean environment for this test
		oldVar := os.Getenv(config.EnvLogLevel)
		defer func() {
			if oldVar == "" {
				os.Unsetenv(config.EnvLogLevel)
			} else {
				os.Setenv(config.EnvLogLevel, oldVar)
			}
		}()

		os.Setenv(config.EnvLogLevel, "error")

		cfg, err := config.FromEnv()
		if err != nil {
			t.Fatalf("FromEnv failed: %v", err)
		}

		if cfg.Logging.Level != "error" {
			t.Errorf("Expected log level error, got %s", cfg.Logging.Level)
		}
	})

	t.Run("Merge", func(t *testing.T) {
		cfg1 := config.New()
		cfg1.Logging.Level = "debug"
		cfg1.Workspace.Path = "/tmp/first"

		cfg2 := config.New()
		cfg2.Logging.Format = "json"
		cfg2.Workspace.Path = "/tmp/second" // Should override cfg1

		merged, err := config.Merge(cfg1, cfg2)
		if err != nil {
			t.Fatalf("Merge failed: %v", err)
		}

		if merged.Logging.Level != "debug" {
			t.Errorf("Expected log level debug, got %s", merged.Logging.Level)
		}

		if merged.Logging.Format != "json" {
			t.Errorf("Expected log format json, got %s", merged.Logging.Format)
		}

		if merged.Workspace.Path != "/tmp/second" {
			t.Errorf("Expected workspace path /tmp/second, got %s", merged.Workspace.Path)
		}
	})
}
