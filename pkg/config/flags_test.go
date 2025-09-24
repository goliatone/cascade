package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestAddFlags(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "adds all required flags to command",
			test: func(t *testing.T) {
				cmd := &cobra.Command{Use: "test"}
				fc := AddFlags(cmd)

				// Verify FlagConfig struct is returned
				if fc == nil {
					t.Fatal("AddFlags should return a FlagConfig struct")
				}

				// Check that all expected flags are present
				expectedFlags := []string{
					"workspace", "manifest", "module", "version", "config",
					"dry-run", "timeout", "parallel",
					"verbose", "quiet", "log-level", "log-format",
					"github-token", "github-endpoint", "github-org",
					"slack-token", "slack-webhook", "slack-channel",
					"state-dir", "state",
				}

				for _, flagName := range expectedFlags {
					if cmd.Flags().Lookup(flagName) == nil {
						t.Errorf("Expected flag %q not found", flagName)
					}
				}
			},
		},
		{
			name: "sets up mutual exclusivity rules",
			test: func(t *testing.T) {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)

				// Add a run function to trigger validation
				cmd.RunE = func(cmd *cobra.Command, args []string) error {
					return nil
				}

				// Test that verbose and quiet are mutually exclusive
				cmd.SetArgs([]string{"--verbose", "--quiet"})
				err := cmd.Execute()
				if err == nil {
					t.Error("Expected error for mutually exclusive flags --verbose and --quiet")
				}
			},
		},
		{
			name: "provides proper help text",
			test: func(t *testing.T) {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)

				// Check that flags have help text
				workspaceFlag := cmd.Flags().Lookup("workspace")
				if workspaceFlag == nil || workspaceFlag.Usage == "" {
					t.Error("workspace flag should have help text")
				}

				manifestFlag := cmd.Flags().Lookup("manifest")
				if manifestFlag == nil || manifestFlag.Usage == "" {
					t.Error("manifest flag should have help text")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestFlagConfig_ValidateFlags(t *testing.T) {
	tests := []struct {
		name      string
		config    FlagConfig
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid configuration",
			config: FlagConfig{
				Timeout:   5 * time.Minute,
				Parallel:  4,
				LogLevel:  "info",
				LogFormat: "json",
			},
			wantError: false,
		},
		{
			name: "negative timeout",
			config: FlagConfig{
				Timeout: -1 * time.Minute,
			},
			wantError: true,
			errorMsg:  "timeout must be positive",
		},
		{
			name: "negative parallel count",
			config: FlagConfig{
				Parallel: -1,
			},
			wantError: true,
			errorMsg:  "parallel count must be non-negative",
		},
		{
			name: "invalid log level",
			config: FlagConfig{
				LogLevel: "invalid",
			},
			wantError: true,
			errorMsg:  "log-level must be one of: debug, info, warn, error",
		},
		{
			name: "invalid log format",
			config: FlagConfig{
				LogFormat: "invalid",
			},
			wantError: true,
			errorMsg:  "log-format must be one of: text, json",
		},
		{
			name: "multiple validation errors",
			config: FlagConfig{
				Timeout:   -1 * time.Minute,
				Parallel:  -1,
				LogLevel:  "invalid",
				LogFormat: "invalid",
			},
			wantError: true,
			errorMsg:  "timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateFlags()

			if tt.wantError {
				if err == nil {
					t.Error("Expected validation error, got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error to contain %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no validation error, got %v", err)
				}
			}
		})
	}
}

func TestFlagConfig_ToConfig(t *testing.T) {
	tests := []struct {
		name       string
		flagConfig FlagConfig
		test       func(t *testing.T, config *Config, err error)
	}{
		{
			name: "converts basic flags to config",
			flagConfig: FlagConfig{
				Workspace: "/custom/workspace",
				Manifest:  "/path/to/deps.yaml",
				DryRun:    true,
				Timeout:   10 * time.Minute,
				Parallel:  8,
			},
			test: func(t *testing.T, config *Config, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if config.Workspace.Path != "/custom/workspace" {
					t.Errorf("Expected workspace path %q, got %q", "/custom/workspace", config.Workspace.Path)
				}

				if config.Workspace.ManifestPath != "/path/to/deps.yaml" {
					t.Errorf("Expected manifest path %q, got %q", "/path/to/deps.yaml", config.Workspace.ManifestPath)
				}

				if !config.Executor.DryRun {
					t.Error("Expected dry run to be true")
				}

				if config.Executor.Timeout != 10*time.Minute {
					t.Errorf("Expected timeout %v, got %v", 10*time.Minute, config.Executor.Timeout)
				}

				if config.Executor.ConcurrentLimit != 8 {
					t.Errorf("Expected concurrent limit %d, got %d", 8, config.Executor.ConcurrentLimit)
				}
			},
		},
		{
			name: "converts logging flags to config",
			flagConfig: FlagConfig{
				Verbose:   true,
				LogFormat: "json",
			},
			test: func(t *testing.T, config *Config, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if !config.Logging.Verbose {
					t.Error("Expected verbose to be true")
				}

				if config.Logging.Level != "debug" {
					t.Errorf("Expected log level %q, got %q", "debug", config.Logging.Level)
				}

				if config.Logging.Format != "json" {
					t.Errorf("Expected log format %q, got %q", "json", config.Logging.Format)
				}
			},
		},
		{
			name: "converts integration flags to config",
			flagConfig: FlagConfig{
				GitHubToken:    "ghp_token123",
				GitHubEndpoint: "https://api.github.com",
				GitHubOrg:      "myorg",
				SlackToken:     "slack_token123",
				SlackWebhook:   "https://hooks.slack.com/webhook",
				SlackChannel:   "#general",
			},
			test: func(t *testing.T, config *Config, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if config.Integration.GitHub.Token != "ghp_token123" {
					t.Errorf("Expected GitHub token %q, got %q", "ghp_token123", config.Integration.GitHub.Token)
				}

				if config.Integration.GitHub.Endpoint != "https://api.github.com" {
					t.Errorf("Expected GitHub endpoint %q, got %q", "https://api.github.com", config.Integration.GitHub.Endpoint)
				}

				if config.Integration.GitHub.Organization != "myorg" {
					t.Errorf("Expected GitHub org %q, got %q", "myorg", config.Integration.GitHub.Organization)
				}

				if config.Integration.Slack.Token != "slack_token123" {
					t.Errorf("Expected Slack token %q, got %q", "slack_token123", config.Integration.Slack.Token)
				}

				if config.Integration.Slack.WebhookURL != "https://hooks.slack.com/webhook" {
					t.Errorf("Expected Slack webhook %q, got %q", "https://hooks.slack.com/webhook", config.Integration.Slack.WebhookURL)
				}

				if config.Integration.Slack.Channel != "#general" {
					t.Errorf("Expected Slack channel %q, got %q", "#general", config.Integration.Slack.Channel)
				}
			},
		},
		{
			name: "handles quiet flag precedence",
			flagConfig: FlagConfig{
				Quiet: true,
			},
			test: func(t *testing.T, config *Config, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if !config.Logging.Quiet {
					t.Error("Expected quiet to be true")
				}

				if config.Logging.Level != "warn" {
					t.Errorf("Expected log level %q, got %q", "warn", config.Logging.Level)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := tt.flagConfig.ToConfig()
			tt.test(t, config, err)
		})
	}
}

func TestLoadFromFlags(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *cobra.Command
		test  func(t *testing.T, config *Config, err error)
	}{
		{
			name: "nil command returns error",
			setup: func() *cobra.Command {
				return nil
			},
			test: func(t *testing.T, config *Config, err error) {
				if err == nil {
					t.Error("Expected error for nil command")
				}
				if !strings.Contains(err.Error(), "command cannot be nil") {
					t.Errorf("Expected error about nil command, got %v", err)
				}
			},
		},
		{
			name: "valid flags load successfully",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)
				cmd.SetArgs([]string{"--workspace", "/test/workspace", "--timeout", "10m", "--verbose"})
				cmd.Execute() // This processes the flags
				return cmd
			},
			test: func(t *testing.T, config *Config, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				if config.Workspace.Path != "/test/workspace" {
					t.Errorf("Expected workspace %q, got %q", "/test/workspace", config.Workspace.Path)
				}

				if config.Executor.Timeout != 10*time.Minute {
					t.Errorf("Expected timeout %v, got %v", 10*time.Minute, config.Executor.Timeout)
				}

				if !config.Logging.Verbose {
					t.Error("Expected verbose to be true")
				}
			},
		},
		{
			name: "invalid flags return validation error",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)
				cmd.SetArgs([]string{"--timeout", "-5m"})
				cmd.Execute()
				return cmd
			},
			test: func(t *testing.T, config *Config, err error) {
				if err == nil {
					t.Error("Expected validation error for negative timeout")
				}
				if !strings.Contains(err.Error(), "timeout must be positive") {
					t.Errorf("Expected timeout validation error, got %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup()
			config, err := LoadFromFlags(cmd)
			tt.test(t, config, err)
		})
	}
}

func TestExtractFlagConfig(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *cobra.Command
		test  func(t *testing.T, fc *FlagConfig)
	}{
		{
			name: "extracts workspace and manifest flags",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)
				cmd.SetArgs([]string{"--workspace", "/test/ws", "--manifest", "/test/deps.yaml"})
				cmd.Execute()
				return cmd
			},
			test: func(t *testing.T, fc *FlagConfig) {
				if fc.Workspace != "/test/ws" {
					t.Errorf("Expected workspace %q, got %q", "/test/ws", fc.Workspace)
				}
				if fc.Manifest != "/test/deps.yaml" {
					t.Errorf("Expected manifest %q, got %q", "/test/deps.yaml", fc.Manifest)
				}
			},
		},
		{
			name: "extracts execution flags",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)
				cmd.SetArgs([]string{"--dry-run", "--timeout", "15m", "--parallel", "6"})
				cmd.Execute()
				return cmd
			},
			test: func(t *testing.T, fc *FlagConfig) {
				if !fc.DryRun {
					t.Error("Expected DryRun to be true")
				}
				if fc.Timeout != 15*time.Minute {
					t.Errorf("Expected timeout %v, got %v", 15*time.Minute, fc.Timeout)
				}
				if fc.Parallel != 6 {
					t.Errorf("Expected parallel %d, got %d", 6, fc.Parallel)
				}
			},
		},
		{
			name: "extracts logging flags",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)
				cmd.SetArgs([]string{"--log-level", "debug", "--log-format", "json"})
				cmd.Execute()
				return cmd
			},
			test: func(t *testing.T, fc *FlagConfig) {
				if fc.LogLevel != "debug" {
					t.Errorf("Expected log level %q, got %q", "debug", fc.LogLevel)
				}
				if fc.LogFormat != "json" {
					t.Errorf("Expected log format %q, got %q", "json", fc.LogFormat)
				}
			},
		},
		{
			name: "only extracts changed flags",
			setup: func() *cobra.Command {
				cmd := &cobra.Command{Use: "test"}
				AddFlags(cmd)
				cmd.SetArgs([]string{"--workspace", "/test"})
				cmd.Execute()
				return cmd
			},
			test: func(t *testing.T, fc *FlagConfig) {
				if fc.Workspace != "/test" {
					t.Errorf("Expected workspace %q, got %q", "/test", fc.Workspace)
				}
				// These should be empty/zero since they weren't set
				if fc.Manifest != "" {
					t.Errorf("Expected empty manifest, got %q", fc.Manifest)
				}
				if fc.Timeout != 0 {
					t.Errorf("Expected zero timeout, got %v", fc.Timeout)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup()
			fc := extractFlagConfig(cmd.Flags())
			tt.test(t, fc)
		})
	}
}

// TestFlagPrecedence verifies that flags override environment variables
func TestFlagPrecedence(t *testing.T) {
	// Set an environment variable
	t.Setenv("CASCADE_WORKSPACE", "/env/workspace")
	t.Setenv("CASCADE_TIMEOUT", "2m")

	// Create command with flags that override environment
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)
	cmd.SetArgs([]string{"--workspace", "/flag/workspace", "--timeout", "10m"})
	cmd.Execute()

	config, err := LoadFromFlags(cmd)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Flags should take precedence over environment
	if config.Workspace.Path != "/flag/workspace" {
		t.Errorf("Expected flag workspace %q, got %q", "/flag/workspace", config.Workspace.Path)
	}

	if config.Executor.Timeout != 10*time.Minute {
		t.Errorf("Expected flag timeout %v, got %v", 10*time.Minute, config.Executor.Timeout)
	}
}
