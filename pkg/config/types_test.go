package config_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/goliatone/cascade/pkg/config"
	"gopkg.in/yaml.v3"
)

func TestNewReturnsConfig(t *testing.T) {
	cfg := config.New()
	if cfg == nil {
		t.Fatal("expected config.New to return non-nil")
	}
}

func TestConfigJSONMarshaling(t *testing.T) {
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Path:         "/tmp/workspace",
			TempDir:      "/tmp/cascade",
			ManifestPath: "/path/to/.cascade.yaml",
		},
		Executor: config.ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
			DryRun:          false,
		},
		Integration: config.IntegrationConfig{
			GitHub: config.GitHubConfig{
				Token:        "ghp_test_token",
				Endpoint:     "https://api.github.com",
				Organization: "test-org",
			},
			Slack: config.SlackConfig{
				Token:      "xoxb-test-token",
				WebhookURL: "https://hooks.slack.com/test",
				Channel:    "#general",
			},
		},
		Logging: config.LoggingConfig{
			Level:   "info",
			Format:  "text",
			Verbose: false,
			Quiet:   false,
		},
		State: config.StateConfig{
			Dir:            "/tmp/state",
			RetentionCount: 10,
			Enabled:        true,
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled config.Config
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal config from JSON: %v", err)
	}

	// Verify key fields
	if unmarshaled.Workspace.Path != cfg.Workspace.Path {
		t.Errorf("workspace path mismatch: got %s, want %s", unmarshaled.Workspace.Path, cfg.Workspace.Path)
	}
	if unmarshaled.Executor.Timeout != cfg.Executor.Timeout {
		t.Errorf("timeout mismatch: got %v, want %v", unmarshaled.Executor.Timeout, cfg.Executor.Timeout)
	}
	if unmarshaled.Integration.GitHub.Token != cfg.Integration.GitHub.Token {
		t.Errorf("github token mismatch: got %s, want %s", unmarshaled.Integration.GitHub.Token, cfg.Integration.GitHub.Token)
	}
}

func TestConfigYAMLMarshaling(t *testing.T) {
	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			Path:         "/tmp/workspace",
			TempDir:      "/tmp/cascade",
			ManifestPath: "/path/to/.cascade.yaml",
		},
		Executor: config.ExecutorConfig{
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 4,
			DryRun:          true,
		},
		Logging: config.LoggingConfig{
			Level:  "debug",
			Format: "json",
		},
	}

	// Test YAML marshaling
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config to YAML: %v", err)
	}

	// Test YAML unmarshaling
	var unmarshaled config.Config
	if err := yaml.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal config from YAML: %v", err)
	}

	// Verify key fields
	if unmarshaled.Workspace.Path != cfg.Workspace.Path {
		t.Errorf("workspace path mismatch: got %s, want %s", unmarshaled.Workspace.Path, cfg.Workspace.Path)
	}
	if unmarshaled.Executor.DryRun != cfg.Executor.DryRun {
		t.Errorf("dry run mismatch: got %v, want %v", unmarshaled.Executor.DryRun, cfg.Executor.DryRun)
	}
	if unmarshaled.Logging.Level != cfg.Logging.Level {
		t.Errorf("log level mismatch: got %s, want %s", unmarshaled.Logging.Level, cfg.Logging.Level)
	}
}

func TestEnvironmentConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"workspace path", config.EnvWorkspacePath, "CASCADE_WORKSPACE"},
		{"temp dir", config.EnvTempDir, "CASCADE_TEMP_DIR"},
		{"manifest path", config.EnvManifestPath, "CASCADE_MANIFEST"},
		{"timeout", config.EnvTimeout, "CASCADE_TIMEOUT"},
		{"concurrent limit", config.EnvConcurrentLimit, "CASCADE_CONCURRENT_LIMIT"},
		{"dry run", config.EnvDryRun, "CASCADE_DRY_RUN"},
		{"github token", config.EnvGitHubToken, "CASCADE_GITHUB_TOKEN"},
		{"github endpoint", config.EnvGitHubEndpoint, "CASCADE_GITHUB_ENDPOINT"},
		{"github org", config.EnvGitHubOrg, "CASCADE_GITHUB_ORG"},
		{"slack token", config.EnvSlackToken, "CASCADE_SLACK_TOKEN"},
		{"slack webhook", config.EnvSlackWebhook, "CASCADE_SLACK_WEBHOOK"},
		{"slack channel", config.EnvSlackChannel, "CASCADE_SLACK_CHANNEL"},
		{"log level", config.EnvLogLevel, "CASCADE_LOG_LEVEL"},
		{"log format", config.EnvLogFormat, "CASCADE_LOG_FORMAT"},
		{"verbose", config.EnvVerbose, "CASCADE_VERBOSE"},
		{"quiet", config.EnvQuiet, "CASCADE_QUIET"},
		{"state dir", config.EnvStateDir, "CASCADE_STATE_DIR"},
		{"state retention", config.EnvStateRetention, "CASCADE_STATE_RETENTION"},
		{"state enabled", config.EnvStateEnabled, "CASCADE_STATE_ENABLED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("environment constant %s mismatch: got %s, want %s", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestConfigStructValidation(t *testing.T) {
	// Test that all required fields are properly tagged
	cfg := config.New()
	if cfg == nil {
		t.Fatal("expected config.New to return non-nil")
	}

	// Verify zero values don't panic on marshaling
	_, err := json.Marshal(cfg)
	if err != nil {
		t.Errorf("failed to marshal zero config to JSON: %v", err)
	}

	_, err = yaml.Marshal(cfg)
	if err != nil {
		t.Errorf("failed to marshal zero config to YAML: %v", err)
	}
}
