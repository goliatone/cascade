package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/pkg/config"
)

func TestLoadFromFile_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
workspace:
  path: "/tmp/workspace"
  manifest_path: ".cascade.yaml"

executor:
  timeout: "5m"
  concurrent_limit: 4
  dry_run: true

logging:
  level: "debug"
  format: "json"
  verbose: true

integration:
  github:
    token: "secret-token"
    organization: "myorg"

state:
  dir: "/tmp/state"
  retention_count: 5
  enabled: true
`

	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify workspace config
	if cfg.Workspace.Path != "/tmp/workspace" {
		t.Errorf("Expected workspace path '/tmp/workspace', got '%s'", cfg.Workspace.Path)
	}
	if cfg.Workspace.ManifestPath != ".cascade.yaml" {
		t.Errorf("Expected manifest path '.cascade.yaml', got '%s'", cfg.Workspace.ManifestPath)
	}

	// Verify executor config
	if cfg.Executor.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout 5m, got %v", cfg.Executor.Timeout)
	}
	if cfg.Executor.ConcurrentLimit != 4 {
		t.Errorf("Expected concurrent limit 4, got %d", cfg.Executor.ConcurrentLimit)
	}
	if !cfg.Executor.DryRun {
		t.Error("Expected dry run to be true")
	}

	// Verify logging config
	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Expected log format 'json', got '%s'", cfg.Logging.Format)
	}
	if !cfg.Logging.Verbose {
		t.Error("Expected verbose to be true")
	}

	// Verify integration config
	if cfg.Integration.GitHub.Token != "secret-token" {
		t.Errorf("Expected github token 'secret-token', got '%s'", cfg.Integration.GitHub.Token)
	}
	if cfg.Integration.GitHub.Organization != "myorg" {
		t.Errorf("Expected github org 'myorg', got '%s'", cfg.Integration.GitHub.Organization)
	}

	// Verify state config
	if cfg.State.Dir != "/tmp/state" {
		t.Errorf("Expected state dir '/tmp/state', got '%s'", cfg.State.Dir)
	}
	if cfg.State.RetentionCount != 5 {
		t.Errorf("Expected retention count 5, got %d", cfg.State.RetentionCount)
	}
	if !cfg.State.Enabled {
		t.Error("Expected state enabled to be true")
	}
}

func TestLoadFromFile_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Path:         "/tmp/workspace",
			ManifestPath: ".cascade.yaml",
		},
		Executor: config.ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}

	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configFile, jsonData, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	loadedCfg, err := config.LoadFromFile(configFile)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if loadedCfg.Workspace.Path != "/tmp/workspace" {
		t.Errorf("Expected workspace path '/tmp/workspace', got '%s'", loadedCfg.Workspace.Path)
	}
	if loadedCfg.Executor.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout 5m, got %v", loadedCfg.Executor.Timeout)
	}
	if loadedCfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got '%s'", loadedCfg.Logging.Level)
	}
}

func TestLoadFromFile_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.txt")

	if err := os.WriteFile(configFile, []byte("some content"), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_, err := config.LoadFromFile(configFile)
	if err == nil {
		t.Fatal("Expected error for unsupported format")
	}

	if !strings.Contains(err.Error(), "unsupported config file format") {
		t.Errorf("Expected unsupported format error, got: %v", err)
	}
}

func TestLoadFromFile_FileNotFound(t *testing.T) {
	_, err := config.LoadFromFile("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("Expected error for non-existent file")
	}

	if !strings.Contains(err.Error(), "failed to read config file") {
		t.Errorf("Expected read error, got: %v", err)
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	invalidYAML := `
workspace:
  path: "/tmp/workspace"
  - invalid yaml syntax
`

	if err := os.WriteFile(configFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_, err := config.LoadFromFile(configFile)
	if err == nil {
		t.Fatal("Expected error for invalid YAML")
	}

	if !strings.Contains(err.Error(), "failed to parse YAML") {
		t.Errorf("Expected YAML parse error, got: %v", err)
	}
}

func TestLoadFromFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")

	invalidJSON := `{
  "workspace": {
    "path": "/tmp/workspace",
    "invalid": json syntax
  }
}`

	if err := os.WriteFile(configFile, []byte(invalidJSON), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_, err := config.LoadFromFile(configFile)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON") {
		t.Errorf("Expected JSON parse error, got: %v", err)
	}
}

func TestLoadFromFile_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		errorMsg string
	}{
		{
			name: "invalid log level",
			config: `
logging:
  level: "invalid-level"
`,
			errorMsg: "invalid logging level",
		},
		{
			name: "invalid log format",
			config: `
logging:
  format: "invalid-format"
`,
			errorMsg: "invalid logging format",
		},
		{
			name: "negative concurrent limit",
			config: `
executor:
  concurrent_limit: -1
`,
			errorMsg: "concurrent_limit must be positive",
		},
		{
			name: "negative timeout",
			config: `
executor:
  timeout: "-5m"
`,
			errorMsg: "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.yaml")

			if err := os.WriteFile(configFile, []byte(tt.config), 0644); err != nil {
				t.Fatalf("Failed to write test config file: %v", err)
			}

			_, err := config.LoadFromFile(configFile)
			if err == nil {
				t.Fatal("Expected validation error")
			}

			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
			}
		})
	}
}

func TestConfigFileLocations(t *testing.T) {
	// Save original env vars
	originalHome := os.Getenv("HOME")
	originalXDGConfig := os.Getenv("XDG_CONFIG_HOME")

	// Set test environment
	testHome := "/home/testuser"
	testXDG := "/custom/config"

	os.Setenv("HOME", testHome)
	os.Setenv("XDG_CONFIG_HOME", testXDG)

	// Restore original env vars after test
	defer func() {
		if originalHome != "" {
			os.Setenv("HOME", originalHome)
		} else {
			os.Unsetenv("HOME")
		}
		if originalXDGConfig != "" {
			os.Setenv("XDG_CONFIG_HOME", originalXDGConfig)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	locations := config.ConfigFileLocations()

	expectedPrefixes := []string{
		".cascade.",
		"/custom/config/cascade/config.",
		"/home/testuser/.config/cascade/config.",
	}

	found := make(map[string]bool)
	for _, location := range locations {
		for _, prefix := range expectedPrefixes {
			if strings.HasPrefix(location, prefix) {
				found[prefix] = true
			}
		}
	}

	for _, prefix := range expectedPrefixes {
		if !found[prefix] {
			t.Errorf("Expected location with prefix '%s' not found in %v", prefix, locations)
		}
	}
}

func TestDiscoverConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp dir for test
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create a config file in current directory
	configFile := ".cascade.yaml"
	if err := os.WriteFile(configFile, []byte("workspace:\n  path: \"/tmp\""), 0644); err != nil {
		t.Fatal(err)
	}

	found, err := config.DiscoverConfigFile()
	if err != nil {
		t.Fatalf("DiscoverConfigFile failed: %v", err)
	}

	if found != configFile {
		t.Errorf("Expected to find '%s', got '%s'", configFile, found)
	}
}

func TestDiscoverConfigFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to empty temp dir
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	found, err := config.DiscoverConfigFile()
	if err != nil {
		t.Fatalf("DiscoverConfigFile failed: %v", err)
	}

	if found != "" {
		t.Errorf("Expected empty string for not found, got '%s'", found)
	}
}

func TestLoadFromFileOrDiscover_WithPath(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `workspace:
  path: "/tmp/workspace"`

	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg, err := config.LoadFromFileOrDiscover(configFile)
	if err != nil {
		t.Fatalf("LoadFromFileOrDiscover failed: %v", err)
	}

	if cfg.Workspace.Path != "/tmp/workspace" {
		t.Errorf("Expected workspace path '/tmp/workspace', got '%s'", cfg.Workspace.Path)
	}
}

func TestLoadFromFileOrDiscover_Discovery(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp dir for test
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create a discoverable config file
	configFile := ".cascade.yaml"
	yamlContent := `workspace:
  path: "/discovered/workspace"`

	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromFileOrDiscover("")
	if err != nil {
		t.Fatalf("LoadFromFileOrDiscover failed: %v", err)
	}

	if cfg.Workspace.Path != "/discovered/workspace" {
		t.Errorf("Expected workspace path '/discovered/workspace', got '%s'", cfg.Workspace.Path)
	}
}

func TestMergeConfigs(t *testing.T) {
	base := &config.Config{}
	base.Workspace.Path = "/base/workspace"
	base.Logging.Level = "info"
	base.Executor.ConcurrentLimit = 2

	override := &config.Config{}
	override.Workspace.Path = "/override/workspace" // Should override
	override.Logging.Format = "json"                // Should add
	// Note: ConcurrentLimit not set, should keep base value

	result := config.MergeConfigs(base, override)

	if result.Workspace.Path != "/override/workspace" {
		t.Errorf("Expected workspace path '/override/workspace', got '%s'", result.Workspace.Path)
	}
	if result.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got '%s'", result.Logging.Level)
	}
	if result.Logging.Format != "json" {
		t.Errorf("Expected log format 'json', got '%s'", result.Logging.Format)
	}
	if result.Executor.ConcurrentLimit != 2 {
		t.Errorf("Expected concurrent limit 2, got %d", result.Executor.ConcurrentLimit)
	}
}

func TestMergeConfigs_BooleanOverride(t *testing.T) {
	t.Setenv(config.EnvDryRun, "true")
	cTrue, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv true failed: %v", err)
	}

	t.Setenv(config.EnvDryRun, "false")
	cFalse, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv false failed: %v", err)
	}

	result := config.MergeConfigs(cTrue, cFalse)
	if result.Executor.DryRun {
		t.Errorf("expected dry run to be false after override")
	}
}

func TestMergeConfigs_BooleanOverrideLogging(t *testing.T) {
	t.Setenv(config.EnvVerbose, "true")
	cVerbose, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv verbose failed: %v", err)
	}

	t.Setenv(config.EnvVerbose, "false")
	cNotVerbose, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv not verbose failed: %v", err)
	}

	result := config.MergeConfigs(cVerbose, cNotVerbose)
	if result.Logging.Verbose {
		t.Errorf("expected verbose to be false after override")
	}
}

func TestMergeConfigs_BooleanOverrideState(t *testing.T) {
	t.Setenv(config.EnvStateEnabled, "true")
	cEnabled, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv state enabled failed: %v", err)
	}

	t.Setenv(config.EnvStateEnabled, "false")
	cDisabled, err := config.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv state disabled failed: %v", err)
	}

	result := config.MergeConfigs(cEnabled, cDisabled)
	if result.State.Enabled {
		t.Errorf("expected state enabled to be false after override")
	}
}

func TestMergeConfigs_NilConfigs(t *testing.T) {
	result := config.MergeConfigs()
	if result == nil {
		t.Error("Expected non-nil result from empty merge")
	}

	cfg := &config.Config{}
	cfg.Workspace.Path = "/test"

	result = config.MergeConfigs(nil, cfg, nil)
	if result.Workspace.Path != "/test" {
		t.Errorf("Expected workspace path '/test', got '%s'", result.Workspace.Path)
	}
}
