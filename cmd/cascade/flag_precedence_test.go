package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/goliatone/cascade/pkg/config"
)

// TestFlagPrecedence verifies that flag precedence works correctly
func TestFlagPrecedence(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		envVars        map[string]string
		expectedValues map[string]interface{}
	}{
		{
			name: "flags override environment",
			args: []string{"--verbose", "--workspace=/tmp/test"},
			envVars: map[string]string{
				"CASCADE_VERBOSE":   "false",
				"CASCADE_WORKSPACE": "/tmp/other",
			},
			expectedValues: map[string]interface{}{
				"verbose":   true,
				"workspace": "/tmp/test",
			},
		},
		{
			name: "explicit false flag overrides environment true",
			args: []string{"--dry-run=false"},
			envVars: map[string]string{
				"CASCADE_DRY_RUN": "true",
			},
			expectedValues: map[string]interface{}{
				"dry_run": false,
			},
		},
		{
			name: "explicit true flag overrides environment false",
			args: []string{"--dry-run=true"},
			envVars: map[string]string{
				"CASCADE_DRY_RUN": "false",
			},
			expectedValues: map[string]interface{}{
				"dry_run": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Create a test command with flags
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Build configuration using the same pattern as the real CLI
					builder := config.NewBuilder().
						FromFile("").  // Skip file loading for test
						FromEnv().     // Load from environment
						FromFlags(cmd) // Load from command flags (highest precedence)

					cfg, err := builder.Build()
					if err != nil {
						return err
					}

					// Check expected values
					if expected, ok := tt.expectedValues["verbose"]; ok {
						if cfg.Logging.Verbose != expected.(bool) {
							t.Errorf("expected verbose=%v, got %v", expected, cfg.Logging.Verbose)
						}
					}

					if expected, ok := tt.expectedValues["workspace"]; ok {
						if cfg.Workspace.Path != expected.(string) {
							t.Errorf("expected workspace=%s, got %s", expected, cfg.Workspace.Path)
						}
					}

					if expected, ok := tt.expectedValues["dry_run"]; ok {
						if cfg.Executor.DryRun != expected.(bool) {
							t.Errorf("expected dry_run=%v, got %v", expected, cfg.Executor.DryRun)
						}
					}

					return nil
				},
			}

			// Add flags using the same pattern as the real CLI
			config.AddFlags(cmd)

			// Parse command line
			cmd.SetArgs(tt.args)

			// Execute the command
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command execution failed: %v", err)
			}
		})
	}
}

// TestMutuallyExclusiveFlags verifies that mutually exclusive flags are handled correctly
func TestMutuallyExclusiveFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "verbose and quiet are mutually exclusive",
			args:        []string{"--verbose", "--quiet"},
			expectError: true,
			errorMsg:    "verbose",
		},
		{
			name:        "verbose and log-level are mutually exclusive",
			args:        []string{"--verbose", "--log-level=info"},
			expectError: true,
			errorMsg:    "verbose",
		},
		{
			name:        "quiet and log-level are mutually exclusive",
			args:        []string{"--quiet", "--log-level=debug"},
			expectError: true,
			errorMsg:    "quiet",
		},
		{
			name:        "verbose alone is valid",
			args:        []string{"--verbose"},
			expectError: false,
		},
		{
			name:        "quiet alone is valid",
			args:        []string{"--quiet"},
			expectError: false,
		},
		{
			name:        "log-level alone is valid",
			args:        []string{"--log-level=warn"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command with flags
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}

			// Add flags using the same pattern as the real CLI
			config.AddFlags(cmd)

			// Parse command line
			cmd.SetArgs(tt.args)

			// Execute the command
			err := cmd.Execute()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorMsg != "" && !contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestBooleanFlagExtraction verifies that boolean flags are extracted correctly
func TestBooleanFlagExtraction(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedValues map[string]bool
		expectedSet    map[string]bool
	}{
		{
			name: "explicit true values",
			args: []string{"--dry-run=true", "--verbose=true", "--state=true"},
			expectedValues: map[string]bool{
				"dry-run": true,
				"verbose": true,
				"state":   true,
			},
			expectedSet: map[string]bool{
				"dry-run": true,
				"verbose": true,
				"state":   true,
			},
		},
		{
			name: "explicit false values",
			args: []string{"--dry-run=false", "--verbose=false", "--state=false"},
			expectedValues: map[string]bool{
				"dry-run": false,
				"verbose": false,
				"state":   false,
			},
			expectedSet: map[string]bool{
				"dry-run": true,
				"verbose": true,
				"state":   true,
			},
		},
		{
			name: "flag present without value defaults to true",
			args: []string{"--dry-run", "--verbose"},
			expectedValues: map[string]bool{
				"dry-run": true,
				"verbose": true,
			},
			expectedSet: map[string]bool{
				"dry-run": true,
				"verbose": true,
			},
		},
		{
			name: "flags not present",
			args: []string{},
			expectedValues: map[string]bool{
				"dry-run": false,
				"verbose": false,
				"state":   false,
			},
			expectedSet: map[string]bool{
				"dry-run": false,
				"verbose": false,
				"state":   false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command with flags
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Load configuration from flags
					cfg, err := config.LoadFromFlags(cmd)
					if err != nil {
						return err
					}

					// Check flag values
					if expected, ok := tt.expectedValues["dry-run"]; ok {
						if cfg.Executor.DryRun != expected {
							t.Errorf("expected dry-run=%v, got %v", expected, cfg.Executor.DryRun)
						}
					}

					if expected, ok := tt.expectedValues["verbose"]; ok {
						if cfg.Logging.Verbose != expected {
							t.Errorf("expected verbose=%v, got %v", expected, cfg.Logging.Verbose)
						}
					}

					if expected, ok := tt.expectedValues["state"]; ok {
						if cfg.State.Enabled != expected {
							t.Errorf("expected state=%v, got %v", expected, cfg.State.Enabled)
						}
					}

					// Check that the set flags are tracked properly
					// Note: This would require access to internal flag tracking,
					// which may not be directly available in the public API.
					// For now, we just verify the values are correct.

					return nil
				},
			}

			// Add flags using the same pattern as the real CLI
			config.AddFlags(cmd)

			// Parse command line
			cmd.SetArgs(tt.args)

			// Execute the command
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command execution failed: %v", err)
			}
		})
	}
}
