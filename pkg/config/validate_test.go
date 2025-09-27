package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/pkg/config"
)

func TestValidate_NilConfig(t *testing.T) {
	err := config.Validate(nil)
	if err == nil {
		t.Fatal("expected validation error for nil config")
	}

	if !strings.Contains(err.Error(), "configuration cannot be nil") {
		t.Errorf("expected nil config error message, got: %v", err)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Path: "/tmp/cascade-test",
		},
		Executor: config.ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
		},
		Integration: config.IntegrationConfig{
			GitHub: config.GitHubConfig{
				Endpoint: "https://api.github.com",
			},
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		State: config.StateConfig{
			Dir:            "/tmp/cascade-state",
			RetentionCount: 10,
			Enabled:        true,
		},
	}

	if err := config.Validate(cfg); err != nil {
		t.Errorf("expected valid config to pass validation, got: %v", err)
	}
}

func TestValidateWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		workspace config.WorkspaceConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid absolute path",
			workspace: config.WorkspaceConfig{
				Path: "/tmp/cascade",
			},
			wantError: false,
		},
		{
			name: "empty path",
			workspace: config.WorkspaceConfig{
				Path: "",
			},
			wantError: true,
			errorMsg:  "workspace path is required",
		},
		{
			name: "relative path",
			workspace: config.WorkspaceConfig{
				Path: "relative/path",
			},
			wantError: true,
			errorMsg:  "workspace path must be absolute",
		},
		{
			name: "valid temp dir",
			workspace: config.WorkspaceConfig{
				Path:    "/tmp/cascade",
				TempDir: "/tmp/cascade-temp",
			},
			wantError: false,
		},
		{
			name: "relative temp dir",
			workspace: config.WorkspaceConfig{
				Path:    "/tmp/cascade",
				TempDir: "relative/temp",
			},
			wantError: true,
			errorMsg:  "temp directory path must be absolute",
		},
		{
			name: "relative manifest path",
			workspace: config.WorkspaceConfig{
				Path:         "/tmp/cascade",
				ManifestPath: ".cascade.yaml",
			},
			wantError: true,
			errorMsg:  "manifest path must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workspace: tt.workspace,
				Executor: config.ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
				},
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: config.StateConfig{
					Dir:            "/tmp/cascade-state",
					RetentionCount: 10,
				},
			}

			err := config.Validate(cfg)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message %q, got: %v", tt.errorMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidateExecutor(t *testing.T) {
	tests := []struct {
		name      string
		executor  config.ExecutorConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid configuration",
			executor: config.ExecutorConfig{
				Timeout:         5 * time.Minute,
				ConcurrentLimit: 4,
			},
			wantError: false,
		},
		{
			name: "zero timeout",
			executor: config.ExecutorConfig{
				Timeout:         0,
				ConcurrentLimit: 4,
			},
			wantError: true,
			errorMsg:  "timeout must be positive",
		},
		{
			name: "negative timeout",
			executor: config.ExecutorConfig{
				Timeout:         -1 * time.Minute,
				ConcurrentLimit: 4,
			},
			wantError: true,
			errorMsg:  "timeout must be positive",
		},
		{
			name: "excessive timeout",
			executor: config.ExecutorConfig{
				Timeout:         25 * time.Hour,
				ConcurrentLimit: 4,
			},
			wantError: true,
			errorMsg:  "timeout cannot exceed 24 hours",
		},
		{
			name: "zero concurrent limit",
			executor: config.ExecutorConfig{
				Timeout:         5 * time.Minute,
				ConcurrentLimit: 0,
			},
			wantError: true,
			errorMsg:  "concurrent limit must be positive",
		},
		{
			name: "excessive concurrent limit",
			executor: config.ExecutorConfig{
				Timeout:         5 * time.Minute,
				ConcurrentLimit: 1001,
			},
			wantError: true,
			errorMsg:  "concurrent limit cannot exceed 1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workspace: config.WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: tt.executor,
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: config.StateConfig{
					Dir:            "/tmp/cascade-state",
					RetentionCount: 10,
				},
			}

			err := config.Validate(cfg)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message %q, got: %v", tt.errorMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidateIntegration(t *testing.T) {
	tests := []struct {
		name        string
		integration config.IntegrationConfig
		wantError   bool
		errorMsg    string
	}{
		{
			name: "valid GitHub token",
			integration: config.IntegrationConfig{
				GitHub: config.GitHubConfig{
					Token:    "ghp_1234567890abcdefghijklmnopqrstuvwxyz123",
					Endpoint: "https://api.github.com",
				},
			},
			wantError: false,
		},
		{
			name: "valid classic GitHub token",
			integration: config.IntegrationConfig{
				GitHub: config.GitHubConfig{
					Token:    "1234567890abcdef1234567890abcdef12345678",
					Endpoint: "https://api.github.com",
				},
			},
			wantError: false,
		},
		{
			name: "invalid GitHub token",
			integration: config.IntegrationConfig{
				GitHub: config.GitHubConfig{
					Token: "invalid-token",
				},
			},
			wantError: true,
			errorMsg:  "invalid GitHub token format",
		},
		{
			name: "invalid GitHub endpoint",
			integration: config.IntegrationConfig{
				GitHub: config.GitHubConfig{
					Endpoint: ":",
				},
			},
			wantError: true,
			errorMsg:  "invalid GitHub endpoint URL",
		},
		{
			name: "valid Slack bot token",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					Token: "xoxb-1234567890-1234567890123-abcdefghijklmnopqrstuvwx",
				},
			},
			wantError: false,
		},
		{
			name: "invalid Slack token",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					Token: "invalid-slack-token",
				},
			},
			wantError: true,
			errorMsg:  "invalid Slack token format",
		},
		{
			name: "valid Slack webhook URL",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					WebhookURL: "https://hooks.slack.com/services/T12345678/B12345678/abcdefghijklmnopqrstuvwx",
				},
			},
			wantError: false,
		},
		{
			name: "insecure Slack webhook URL",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					WebhookURL: "http://hooks.slack.com/services/T12345678/B12345678/abcdefghijklmnopqrstuvwx",
				},
			},
			wantError: true,
			errorMsg:  "Slack webhook URL must use HTTPS",
		},
		{
			name: "valid Slack channel",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					Channel: "#general",
				},
			},
			wantError: false,
		},
		{
			name: "valid Slack user",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					Channel: "@username",
				},
			},
			wantError: false,
		},
		{
			name: "invalid Slack channel",
			integration: config.IntegrationConfig{
				Slack: config.SlackConfig{
					Channel: "invalid-channel",
				},
			},
			wantError: true,
			errorMsg:  "Slack channel must start with # (channel) or @ (user)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workspace: config.WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: config.ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
				},
				Integration: tt.integration,
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: config.StateConfig{
					Dir:            "/tmp/cascade-state",
					RetentionCount: 10,
				},
			}

			err := config.Validate(cfg)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message %q, got: %v", tt.errorMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidateLogging(t *testing.T) {
	tests := []struct {
		name      string
		logging   config.LoggingConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid logging configuration",
			logging: config.LoggingConfig{
				Level:  "info",
				Format: "text",
			},
			wantError: false,
		},
		{
			name: "invalid log level",
			logging: config.LoggingConfig{
				Level:  "invalid",
				Format: "text",
			},
			wantError: true,
			errorMsg:  "invalid log level",
		},
		{
			name: "invalid log format",
			logging: config.LoggingConfig{
				Level:  "info",
				Format: "invalid",
			},
			wantError: true,
			errorMsg:  "invalid log format",
		},
		{
			name: "verbose and quiet both enabled",
			logging: config.LoggingConfig{
				Level:   "info",
				Format:  "text",
				Verbose: true,
				Quiet:   true,
			},
			wantError: true,
			errorMsg:  "verbose and quiet modes are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workspace: config.WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: config.ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
				},
				Logging: tt.logging,
				State: config.StateConfig{
					Dir:            "/tmp/cascade-state",
					RetentionCount: 10,
				},
			}

			err := config.Validate(cfg)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message %q, got: %v", tt.errorMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidateState(t *testing.T) {
	tests := []struct {
		name      string
		state     config.StateConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid state configuration",
			state: config.StateConfig{
				Dir:            "/tmp/cascade-state",
				RetentionCount: 10,
				Enabled:        true,
			},
			wantError: false,
		},
		{
			name: "relative state directory",
			state: config.StateConfig{
				Dir:            "relative/state",
				RetentionCount: 10,
			},
			wantError: true,
			errorMsg:  "state directory path must be absolute",
		},
		{
			name: "zero retention count",
			state: config.StateConfig{
				Dir:            "/tmp/cascade-state",
				RetentionCount: 0,
			},
			wantError: true,
			errorMsg:  "retention count must be positive",
		},
		{
			name: "excessive retention count",
			state: config.StateConfig{
				Dir:            "/tmp/cascade-state",
				RetentionCount: 10001,
			},
			wantError: true,
			errorMsg:  "retention count cannot exceed 10000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Workspace: config.WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: config.ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
				},
				Logging: config.LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: tt.state,
			}

			err := config.Validate(cfg)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected validation error")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message %q, got: %v", tt.errorMsg, err)
				}
			} else if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestApplyDefaults_NilConfig(t *testing.T) {
	err := config.ApplyDefaults(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}

	if !strings.Contains(err.Error(), "configuration cannot be nil") {
		t.Errorf("expected nil config error message, got: %v", err)
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &config.Config{}

	err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("unexpected error applying defaults: %v", err)
	}

	// Verify workspace defaults
	if cfg.Workspace.Path == "" {
		t.Error("expected workspace path to be set by defaults")
	}

	// Verify executor defaults
	if cfg.Executor.Timeout != 5*time.Minute {
		t.Errorf("expected timeout default of 5m, got: %v", cfg.Executor.Timeout)
	}

	expectedConcurrency := runtime.NumCPU()
	if expectedConcurrency > 4 {
		expectedConcurrency = 4
	}
	if cfg.Executor.ConcurrentLimit != expectedConcurrency {
		t.Errorf("expected concurrent limit default of %d, got: %d", expectedConcurrency, cfg.Executor.ConcurrentLimit)
	}

	// Verify integration defaults
	if cfg.Integration.GitHub.Endpoint != "https://api.github.com" {
		t.Errorf("expected GitHub endpoint default, got: %s", cfg.Integration.GitHub.Endpoint)
	}

	// Verify logging defaults
	if cfg.Logging.Level != "info" {
		t.Errorf("expected log level default of 'info', got: %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected log format default of 'text', got: %s", cfg.Logging.Format)
	}

	// Verify state defaults
	if cfg.State.Dir == "" {
		t.Error("expected state dir to be set by defaults")
	}
	if cfg.State.RetentionCount != 10 {
		t.Errorf("expected retention count default of 10, got: %d", cfg.State.RetentionCount)
	}
	if !cfg.State.Enabled {
		t.Error("expected state to be enabled by default")
	}
}

func TestApplyDefaults_VerboseAndQuiet(t *testing.T) {
	tests := []struct {
		name          string
		verbose       bool
		quiet         bool
		expectedLevel string
		initialLevel  string
	}{
		{
			name:          "verbose mode sets debug level",
			verbose:       true,
			expectedLevel: "debug",
		},
		{
			name:          "quiet mode sets warn level",
			quiet:         true,
			expectedLevel: "warn",
		},
		{
			name:          "neither verbose nor quiet uses default",
			expectedLevel: "info",
		},
		{
			name:          "verbose overrides existing level",
			verbose:       true,
			initialLevel:  "error",
			expectedLevel: "debug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Logging: config.LoggingConfig{
					Level:   tt.initialLevel,
					Verbose: tt.verbose,
					Quiet:   tt.quiet,
				},
			}

			err := config.ApplyDefaults(cfg)
			if err != nil {
				t.Fatalf("unexpected error applying defaults: %v", err)
			}

			if cfg.Logging.Level != tt.expectedLevel {
				t.Errorf("expected log level %s, got: %s", tt.expectedLevel, cfg.Logging.Level)
			}
		})
	}
}

func TestDefaultPaths(t *testing.T) {
	// Test workspace path generation
	cfg := &config.Config{}
	config.ApplyDefaults(cfg)

	if !filepath.IsAbs(cfg.Workspace.Path) {
		t.Errorf("default workspace path should be absolute, got: %s", cfg.Workspace.Path)
	}

	if !filepath.IsAbs(cfg.State.Dir) {
		t.Errorf("default state directory should be absolute, got: %s", cfg.State.Dir)
	}

	// Test with custom environment
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	cfg2 := &config.Config{}
	config.ApplyDefaults(cfg2)

	expectedWorkspace := "/custom/cache/cascade"
	if cfg2.Workspace.Path != expectedWorkspace {
		t.Errorf("expected workspace path %s, got: %s", expectedWorkspace, cfg2.Workspace.Path)
	}

	expectedState := "/custom/state/cascade"
	if cfg2.State.Dir != expectedState {
		t.Errorf("expected state dir %s, got: %s", expectedState, cfg2.State.Dir)
	}
}

func TestValidationErrors_Multiple(t *testing.T) {
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Path: "", // Missing required path
		},
		Executor: config.ExecutorConfig{
			Timeout:         -1 * time.Minute, // Invalid timeout
			ConcurrentLimit: 0,                // Invalid concurrency
		},
		Logging: config.LoggingConfig{
			Level:  "invalid", // Invalid log level
			Format: "invalid", // Invalid log format
		},
		State: config.StateConfig{
			RetentionCount: -1, // Invalid retention count
		},
	}

	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected validation errors")
	}

	errMsg := err.Error()
	expectedErrors := []string{
		"workspace path is required",
		"timeout must be positive",
		"concurrent limit must be positive",
		"invalid log level",
		"invalid log format",
		"retention count must be positive",
	}

	for _, expectedErr := range expectedErrors {
		if !strings.Contains(errMsg, expectedErr) {
			t.Errorf("expected error message to contain %q, got: %s", expectedErr, errMsg)
		}
	}
}

func TestManifestFileValidation(t *testing.T) {
	// Create a temporary manifest file
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, ".cascade.yaml")

	if err := os.WriteFile(manifestPath, []byte("modules: []"), 0644); err != nil {
		t.Fatalf("failed to create test manifest file: %v", err)
	}

	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Path:         "/tmp/cascade",
			ManifestPath: manifestPath,
		},
		Executor: config.ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		State: config.StateConfig{
			Dir:            "/tmp/cascade-state",
			RetentionCount: 10,
		},
	}

	// Should pass validation with existing file
	if err := config.Validate(cfg); err != nil {
		t.Errorf("expected validation to pass with existing manifest file, got: %v", err)
	}

	// Should fail validation with non-existent file
	cfg.Workspace.ManifestPath = "/non/existent/manifest.yaml"
	err := config.Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for non-existent manifest file")
	}
	if !strings.Contains(err.Error(), "manifest file does not exist") {
		t.Errorf("expected manifest file error, got: %v", err)
	}
}
