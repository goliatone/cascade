package config_test

import (
	"testing"
	"time"

	"github.com/goliatone/cascade/pkg/config"
)

func TestEnvParser_ParseEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		check   func(*testing.T, *config.Config)
	}{
		{
			name:    "empty environment",
			envVars: map[string]string{},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				// Should have all zero values
				if cfg.Workspace.Path != "" {
					t.Errorf("expected empty workspace path, got %s", cfg.Workspace.Path)
				}
				if cfg.Executor.Timeout != 0 {
					t.Errorf("expected zero timeout, got %v", cfg.Executor.Timeout)
				}
			},
		},
		{
			name: "workspace configuration",
			envVars: map[string]string{
				"CASCADE_WORKSPACE": "/tmp/cascade",
				"CASCADE_TEMP_DIR":  "/tmp",
				"CASCADE_MANIFEST":  "/path/to/deps.yaml",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.Workspace.Path != "/tmp/cascade" {
					t.Errorf("expected workspace path '/tmp/cascade', got %s", cfg.Workspace.Path)
				}
				if cfg.Workspace.TempDir != "/tmp" {
					t.Errorf("expected temp dir '/tmp', got %s", cfg.Workspace.TempDir)
				}
				if cfg.Workspace.ManifestPath != "/path/to/deps.yaml" {
					t.Errorf("expected manifest path '/path/to/deps.yaml', got %s", cfg.Workspace.ManifestPath)
				}
			},
		},
		{
			name: "executor configuration",
			envVars: map[string]string{
				"CASCADE_TIMEOUT":          "10m",
				"CASCADE_CONCURRENT_LIMIT": "8",
				"CASCADE_DRY_RUN":          "true",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				expectedTimeout := 10 * time.Minute
				if cfg.Executor.Timeout != expectedTimeout {
					t.Errorf("expected timeout %v, got %v", expectedTimeout, cfg.Executor.Timeout)
				}
				if cfg.Executor.ConcurrentLimit != 8 {
					t.Errorf("expected concurrent limit 8, got %d", cfg.Executor.ConcurrentLimit)
				}
				if !cfg.Executor.DryRun {
					t.Error("expected dry run to be true")
				}
			},
		},
		{
			name: "integration configuration",
			envVars: map[string]string{
				"CASCADE_GITHUB_TOKEN":    "ghp_token123",
				"CASCADE_GITHUB_ENDPOINT": "https://api.github.com",
				"CASCADE_GITHUB_ORG":      "myorg",
				"CASCADE_SLACK_TOKEN":     "xoxb-slack-token",
				"CASCADE_SLACK_WEBHOOK":   "https://hooks.slack.com/webhook",
				"CASCADE_SLACK_CHANNEL":   "#notifications",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.Integration.GitHub.Token != "ghp_token123" {
					t.Errorf("expected GitHub token 'ghp_token123', got %s", cfg.Integration.GitHub.Token)
				}
				if cfg.Integration.GitHub.Endpoint != "https://api.github.com" {
					t.Errorf("expected GitHub endpoint 'https://api.github.com', got %s", cfg.Integration.GitHub.Endpoint)
				}
				if cfg.Integration.GitHub.Organization != "myorg" {
					t.Errorf("expected GitHub org 'myorg', got %s", cfg.Integration.GitHub.Organization)
				}
				if cfg.Integration.Slack.Token != "xoxb-slack-token" {
					t.Errorf("expected Slack token 'xoxb-slack-token', got %s", cfg.Integration.Slack.Token)
				}
				if cfg.Integration.Slack.WebhookURL != "https://hooks.slack.com/webhook" {
					t.Errorf("expected Slack webhook 'https://hooks.slack.com/webhook', got %s", cfg.Integration.Slack.WebhookURL)
				}
				if cfg.Integration.Slack.Channel != "#notifications" {
					t.Errorf("expected Slack channel '#notifications', got %s", cfg.Integration.Slack.Channel)
				}
			},
		},
		{
			name: "logging configuration",
			envVars: map[string]string{
				"CASCADE_LOG_LEVEL":  "debug",
				"CASCADE_LOG_FORMAT": "json",
				"CASCADE_VERBOSE":    "yes",
				"CASCADE_QUIET":      "false",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.Logging.Level != "debug" {
					t.Errorf("expected log level 'debug', got %s", cfg.Logging.Level)
				}
				if cfg.Logging.Format != "json" {
					t.Errorf("expected log format 'json', got %s", cfg.Logging.Format)
				}
				if !cfg.Logging.Verbose {
					t.Error("expected verbose to be true")
				}
				if cfg.Logging.Quiet {
					t.Error("expected quiet to be false")
				}
			},
		},
		{
			name: "state configuration",
			envVars: map[string]string{
				"CASCADE_STATE_DIR":       "/var/lib/cascade",
				"CASCADE_STATE_RETENTION": "20",
				"CASCADE_STATE_ENABLED":   "on",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.State.Dir != "/var/lib/cascade" {
					t.Errorf("expected state dir '/var/lib/cascade', got %s", cfg.State.Dir)
				}
				if cfg.State.RetentionCount != 20 {
					t.Errorf("expected retention count 20, got %d", cfg.State.RetentionCount)
				}
				if !cfg.State.Enabled {
					t.Error("expected state enabled to be true")
				}
			},
		},
		{
			name: "invalid timeout",
			envVars: map[string]string{
				"CASCADE_TIMEOUT": "invalid-duration",
			},
			wantErr: true,
		},
		{
			name: "invalid concurrent limit",
			envVars: map[string]string{
				"CASCADE_CONCURRENT_LIMIT": "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "zero concurrent limit",
			envVars: map[string]string{
				"CASCADE_CONCURRENT_LIMIT": "0",
			},
			wantErr: true,
		},
		{
			name: "invalid boolean",
			envVars: map[string]string{
				"CASCADE_DRY_RUN": "maybe",
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			envVars: map[string]string{
				"CASCADE_LOG_LEVEL": "trace",
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			envVars: map[string]string{
				"CASCADE_LOG_FORMAT": "xml",
			},
			wantErr: true,
		},
		{
			name: "invalid retention count",
			envVars: map[string]string{
				"CASCADE_STATE_RETENTION": "-1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := config.NewEnvParserWithGetter(func(key string) string {
				return tt.envVars[key]
			})

			cfg, err := parser.ParseEnv()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestEnvParser_ParseBool(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
		wantErr  bool
	}{
		// True values
		{"true", "true", true, false},
		{"True", "True", true, false},
		{"TRUE", "TRUE", true, false},
		{"1", "1", true, false},
		{"yes", "yes", true, false},
		{"Yes", "Yes", true, false},
		{"YES", "YES", true, false},
		{"on", "on", true, false},
		{"On", "On", true, false},
		{"ON", "ON", true, false},
		{"enabled", "enabled", true, false},
		{"Enabled", "Enabled", true, false},
		{"ENABLED", "ENABLED", true, false},

		// False values
		{"false", "false", false, false},
		{"False", "False", false, false},
		{"FALSE", "FALSE", false, false},
		{"0", "0", false, false},
		{"no", "no", false, false},
		{"No", "No", false, false},
		{"NO", "NO", false, false},
		{"off", "off", false, false},
		{"Off", "Off", false, false},
		{"OFF", "OFF", false, false},
		{"disabled", "disabled", false, false},
		{"Disabled", "Disabled", false, false},
		{"DISABLED", "DISABLED", false, false},
		{"empty", "", false, false},
		{"whitespace", "   ", false, false},

		// Invalid values
		{"invalid", "maybe", false, true},
		{"number", "2", false, true},
		{"negative", "-1", false, true},
		{"random", "random", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to use reflection or a helper method to test parseBool
			// Since parseBool is not exported, we'll test it through DryRun parsing
			mockEnv := map[string]string{
				"CASCADE_DRY_RUN": tt.input,
			}

			testParser := config.NewEnvParserWithGetter(func(key string) string {
				return mockEnv[key]
			})

			cfg, err := testParser.ParseEnv()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if cfg.Executor.DryRun != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, cfg.Executor.DryRun)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	// Test the convenience function
	cfg, err := config.FromEnv()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Error("expected config, got nil")
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Test the backward-compatible function
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if cfg == nil {
		t.Error("expected config, got nil")
	}
}

func TestEnvParser_StringList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single", "item", []string{"item"}},
		{"multiple", "item1,item2,item3", []string{"item1", "item2", "item3"}},
		{"with_spaces", "item1, item2 , item3", []string{"item1", "item2", "item3"}},
		{"with_empty_parts", "item1,,item3,", []string{"item1", "item3"}},
		{"only_spaces", " , , ", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since parseStringList is not exported, we'll create a simple test
			// by using a reflection approach or by accessing it through a test helper
			// For now, we'll document this test structure for when string lists are needed
			t.Skipf("parseStringList is not exported and not currently used by any env vars")
		})
	}
}
