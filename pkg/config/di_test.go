package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestConfiguredContainer(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "config cannot be nil",
		},
		{
			name: "missing workspace path",
			config: &Config{
				Workspace: WorkspaceConfig{
					Path: "", // Missing required workspace path
				},
				Executor: ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: StateConfig{
					Enabled: false,
				},
			},
			expectError: true,
			errorMsg:    "workspace path is required",
		},
		{
			name: "missing state dir when enabled",
			config: &Config{
				Workspace: WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: StateConfig{
					Enabled: true,
					Dir:     "", // Missing required state dir
				},
			},
			expectError: true,
			errorMsg:    "state directory is required when state persistence is enabled",
		},
		{
			name: "invalid executor timeout",
			config: &Config{
				Workspace: WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: ExecutorConfig{
					Timeout:         0, // Invalid timeout
					ConcurrentLimit: 4,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: StateConfig{
					Enabled: false,
				},
			},
			expectError: true,
			errorMsg:    "executor timeout must be positive",
		},
		{
			name: "invalid concurrent limit",
			config: &Config{
				Workspace: WorkspaceConfig{
					Path: "/tmp/cascade",
				},
				Executor: ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 0, // Invalid concurrent limit
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: StateConfig{
					Enabled: false,
				},
			},
			expectError: true,
			errorMsg:    "executor concurrent limit must be positive",
		},
		{
			name: "valid basic config",
			config: &Config{
				Workspace: WorkspaceConfig{
					Path:         "/tmp/cascade",
					ManifestPath: "/tmp/deps.yaml",
				},
				Executor: ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
					DryRun:          false,
				},
				Integration: IntegrationConfig{
					GitHub: GitHubConfig{
						Token: "ghp_test_token",
					},
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				State: StateConfig{
					Enabled:        true,
					Dir:            "/tmp/cascade/state",
					RetentionCount: 10,
				},
			},
			expectError: false,
		},
		{
			name: "valid debug config",
			config: &Config{
				Workspace: WorkspaceConfig{
					Path:         "/tmp/cascade",
					ManifestPath: "/tmp/deps.yaml",
				},
				Executor: ExecutorConfig{
					Timeout:         5 * time.Minute,
					ConcurrentLimit: 4,
					DryRun:          true,
				},
				Integration: IntegrationConfig{
					GitHub: GitHubConfig{
						Token: "ghp_test_token",
					},
				},
				Logging: LoggingConfig{
					Level:  "debug",
					Format: "json",
				},
				State: StateConfig{
					Enabled:        true,
					Dir:            "/tmp/cascade/state",
					RetentionCount: 10,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diConfig, err := ConfiguredContainer(tt.config)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				if tt.errorMsg != "" && err.Error() != tt.errorMsg && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diConfig == nil {
				t.Fatal("expected DI config but got nil")
			}

			// Verify basic structure
			if diConfig.Config != tt.config {
				t.Error("DI config should reference the original config")
			}

			if diConfig.Logger == nil {
				t.Error("logger should be configured")
			}

			// Verify logger configuration matches config
			expectedDebug := tt.config.Logging.Level == "debug"
			if diConfig.Debug != expectedDebug {
				t.Errorf("expected debug=%v, got %v", expectedDebug, diConfig.Debug)
			}
		})
	}
}

func TestServiceSelectionDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected ServiceSelectionConfig
	}{
		{
			name: "default selection",
			config: &Config{
				Executor: ExecutorConfig{
					DryRun: false,
				},
				Integration: IntegrationConfig{
					GitHub: GitHubConfig{},
				},
			},
			expected: ServiceSelectionConfig{
				GitImplementation: "native",
				StateBackend:      "filesystem",
				HTTPClient:        "default",
			},
		},
		{
			name: "dry-run selection",
			config: &Config{
				Executor: ExecutorConfig{
					DryRun: true,
				},
				Integration: IntegrationConfig{
					GitHub: GitHubConfig{},
				},
			},
			expected: ServiceSelectionConfig{
				GitImplementation: "native",
				StateBackend:      "memory", // Should use memory in dry-run
				HTTPClient:        "default",
			},
		},
		{
			name: "with github token",
			config: &Config{
				Executor: ExecutorConfig{
					DryRun: false,
				},
				Integration: IntegrationConfig{
					GitHub: GitHubConfig{
						Token: "ghp_test_token",
					},
				},
			},
			expected: ServiceSelectionConfig{
				GitImplementation: "native",
				StateBackend:      "filesystem",
				HTTPClient:        "default",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyServiceSelectionDefaults(tt.config)

			if result.GitImplementation != tt.expected.GitImplementation {
				t.Errorf("expected GitImplementation=%s, got %s", tt.expected.GitImplementation, result.GitImplementation)
			}
			if result.StateBackend != tt.expected.StateBackend {
				t.Errorf("expected StateBackend=%s, got %s", tt.expected.StateBackend, result.StateBackend)
			}
			if result.HTTPClient != tt.expected.HTTPClient {
				t.Errorf("expected HTTPClient=%s, got %s", tt.expected.HTTPClient, result.HTTPClient)
			}
		})
	}
}

func TestFeatureToggleDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected FeatureToggleConfig
	}{
		{
			name: "normal mode",
			config: &Config{
				Executor: ExecutorConfig{
					DryRun: false,
				},
				Logging: LoggingConfig{
					Level: "info",
				},
			},
			expected: FeatureToggleConfig{
				EnableMetrics:    false, // Not debug level
				EnableRetries:    true,
				EnableCaching:    true, // Not dry-run
				EnableValidation: true,
			},
		},
		{
			name: "debug mode",
			config: &Config{
				Executor: ExecutorConfig{
					DryRun: false,
				},
				Logging: LoggingConfig{
					Level: "debug",
				},
			},
			expected: FeatureToggleConfig{
				EnableMetrics:    true, // Debug level
				EnableRetries:    true,
				EnableCaching:    true,
				EnableValidation: true,
			},
		},
		{
			name: "dry-run mode",
			config: &Config{
				Executor: ExecutorConfig{
					DryRun: true,
				},
				Logging: LoggingConfig{
					Level: "debug",
				},
			},
			expected: FeatureToggleConfig{
				EnableMetrics:    false, // Disabled in dry-run
				EnableRetries:    true,
				EnableCaching:    false, // Disabled in dry-run
				EnableValidation: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyFeatureToggleDefaults(tt.config)

			if result.EnableMetrics != tt.expected.EnableMetrics {
				t.Errorf("expected EnableMetrics=%v, got %v", tt.expected.EnableMetrics, result.EnableMetrics)
			}
			if result.EnableRetries != tt.expected.EnableRetries {
				t.Errorf("expected EnableRetries=%v, got %v", tt.expected.EnableRetries, result.EnableRetries)
			}
			if result.EnableCaching != tt.expected.EnableCaching {
				t.Errorf("expected EnableCaching=%v, got %v", tt.expected.EnableCaching, result.EnableCaching)
			}
			if result.EnableValidation != tt.expected.EnableValidation {
				t.Errorf("expected EnableValidation=%v, got %v", tt.expected.EnableValidation, result.EnableValidation)
			}
		})
	}
}

func TestCreateConfiguredLogger(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		expectError    bool
		expectedLogger bool
	}{
		{
			name: "text format info level",
			config: &Config{
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
			},
			expectError:    false,
			expectedLogger: true,
		},
		{
			name: "json format debug level",
			config: &Config{
				Logging: LoggingConfig{
					Level:  "debug",
					Format: "json",
				},
			},
			expectError:    false,
			expectedLogger: true,
		},
		{
			name: "default handling for invalid level",
			config: &Config{
				Logging: LoggingConfig{
					Level:  "invalid",
					Format: "text",
				},
			},
			expectError:    false,
			expectedLogger: true,
		},
		{
			name: "default handling for invalid format",
			config: &Config{
				Logging: LoggingConfig{
					Level:  "info",
					Format: "invalid",
				},
			},
			expectError:    false,
			expectedLogger: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := createConfiguredLogger(tt.config)

			if tt.expectError && err == nil {
				t.Fatal("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectedLogger {
				if logger == nil {
					t.Fatal("expected logger but got nil")
				}
				// Verify it's a valid slog.Logger
				if _, ok := interface{}(logger).(*slog.Logger); !ok {
					t.Fatal("expected *slog.Logger")
				}
			}
		})
	}
}

func TestDIContainerConfigAccessors(t *testing.T) {
	config := &Config{
		Workspace: WorkspaceConfig{
			Path:         "/tmp/cascade",
			ManifestPath: "/tmp/deps.yaml",
		},
		Executor: ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
		},
		Integration: IntegrationConfig{
			GitHub: GitHubConfig{
				Token: "test_token",
			},
		},
		Logging: LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
		State: StateConfig{
			Enabled: true,
			Dir:     "/tmp/state",
		},
	}

	diConfig, err := ConfiguredContainer(config)
	if err != nil {
		t.Fatalf("unexpected error creating DI config: %v", err)
	}

	// Test accessor methods
	if diConfig.GetWorkspaceConfig() != config.Workspace {
		t.Error("GetWorkspaceConfig should return config.Workspace")
	}

	if diConfig.GetExecutorConfig() != config.Executor {
		t.Error("GetExecutorConfig should return config.Executor")
	}

	if diConfig.GetIntegrationConfig() != config.Integration {
		t.Error("GetIntegrationConfig should return config.Integration")
	}

	if diConfig.GetLoggingConfig() != config.Logging {
		t.Error("GetLoggingConfig should return config.Logging")
	}

	if diConfig.GetStateConfig() != config.State {
		t.Error("GetStateConfig should return config.State")
	}

	if !diConfig.IsDebugEnabled() {
		t.Error("IsDebugEnabled should return true for debug level")
	}

	serviceSelection := diConfig.GetServiceSelection()
	if serviceSelection.GitImplementation == "" {
		t.Error("ServiceSelection should have GitImplementation set")
	}

	featureToggles := diConfig.GetFeatureToggles()
	if !featureToggles.EnableValidation {
		t.Error("FeatureToggles should have EnableValidation set to true")
	}
}

func TestDebugInfo(t *testing.T) {
	config := &Config{
		Workspace: WorkspaceConfig{
			Path:         "/tmp/cascade",
			ManifestPath: "/tmp/deps.yaml",
		},
		Executor: ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
			DryRun:          true,
		},
		Logging: LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
		State: StateConfig{
			Enabled: true,
			Dir:     "/tmp/state",
		},
	}

	diConfig, err := ConfiguredContainer(config)
	if err != nil {
		t.Fatalf("unexpected error creating DI config: %v", err)
	}

	debugInfo := diConfig.DebugInfo()
	if debugInfo == nil {
		t.Fatal("DebugInfo should not be nil")
	}

	// Verify some key fields are present
	if debugInfo["workspace_path"] != "/tmp/cascade" {
		t.Errorf("expected workspace_path=/tmp/cascade, got %v", debugInfo["workspace_path"])
	}

	if debugInfo["log_level"] != "debug" {
		t.Errorf("expected log_level=debug, got %v", debugInfo["log_level"])
	}

	if debugInfo["dry_run"] != true {
		t.Errorf("expected dry_run=true, got %v", debugInfo["dry_run"])
	}

	// Test disabled debug
	configNoDebug := &Config{
		Workspace: WorkspaceConfig{
			Path: "/tmp/cascade",
		},
		Executor: ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
		},
		Logging: LoggingConfig{
			Level:  "info", // Not debug
			Format: "text",
		},
		State: StateConfig{
			Enabled: false,
		},
	}

	diConfigNoDebug, err := ConfiguredContainer(configNoDebug)
	if err != nil {
		t.Fatalf("unexpected error creating DI config: %v", err)
	}

	debugInfoDisabled := diConfigNoDebug.DebugInfo()
	if debugInfoDisabled["debug"] != "disabled" {
		t.Errorf("expected debug=disabled when not in debug mode, got %v", debugInfoDisabled["debug"])
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
