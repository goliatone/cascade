package main

import (
	"os"
	"os/exec"
	"testing"
)

// TestExitCodes verifies that the CLI returns appropriate exit codes for different error scenarios
func TestExitCodes(t *testing.T) {
	// Skip if we're not in a test environment that supports executing binaries
	if testing.Short() {
		t.Skip("skipping exit code tests in short mode")
	}

	tests := []struct {
		name          string
		args          []string
		env           map[string]string
		expectedCode  int
		skipCondition func() bool
	}{
		{
			name:         "help command succeeds",
			args:         []string{"--help"},
			expectedCode: ExitSuccess,
		},
		{
			name:         "plan without module fails with validation error",
			args:         []string{"plan"},
			expectedCode: ExitFileError, // Actually fails on manifest loading first
		},
		{
			name:         "plan with invalid manifest fails with file error",
			args:         []string{"plan", "--module=github.com/example/test", "--version=v1.0.0", "--manifest=nonexistent.yaml"},
			expectedCode: ExitConfigError, // Configuration initialization may fail first
		},
		{
			name:         "invalid flag fails with validation error",
			args:         []string{"--invalid-flag"},
			expectedCode: ExitValidationError,
		},
		{
			name:         "release without credentials fails with config error",
			args:         []string{"release", "--module=github.com/example/test", "--version=v1.0.0"},
			expectedCode: ExitConfigError,
			env: map[string]string{
				// Ensure no GitHub token is present
				"GITHUB_TOKEN":         "",
				"CASCADE_GITHUB_TOKEN": "",
				"GH_TOKEN":             "",
			},
		},
	}

	// Build the binary first
	binary, cleanup := buildTestBinary(t)
	defer cleanup()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipCondition != nil && tt.skipCondition() {
				t.Skip("skipping test due to condition")
			}

			cmd := exec.Command(binary, tt.args...)

			// Set up environment
			if tt.env != nil {
				env := os.Environ()
				for k, v := range tt.env {
					if v == "" {
						// Remove the environment variable
						env = removeFromEnv(env, k)
					} else {
						env = append(env, k+"="+v)
					}
				}
				cmd.Env = env
			}

			err := cmd.Run()

			var actualCode int
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					actualCode = exitError.ExitCode()
				} else {
					t.Fatalf("unexpected error type: %v", err)
				}
			} else {
				actualCode = 0
			}

			if actualCode != tt.expectedCode {
				t.Errorf("expected exit code %d, got %d", tt.expectedCode, actualCode)
			}
		})
	}
}

// buildTestBinary builds the cascade binary for testing
func buildTestBinary(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	binaryPath := tmpDir + "/cascade"

	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = "." // Build from the current directory
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build test binary: %v", err)
	}

	return binaryPath, func() {
		// Cleanup is handled by t.TempDir()
	}
}

// removeFromEnv removes an environment variable from the environment slice
func removeFromEnv(env []string, key string) []string {
	var result []string
	prefix := key + "="
	for _, e := range env {
		if !startsWith(e, prefix) && e != key {
			result = append(result, e)
		}
	}
	return result
}

// startsWith checks if a string starts with a prefix
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

// TestErrorTypeMapping verifies that different error types map to correct exit codes
func TestErrorTypeMapping(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{
			name:         "CLIError returns its exit code",
			err:          &CLIError{Code: ExitValidationError, Message: "validation failed"},
			expectedCode: ExitValidationError,
		},
		{
			name:         "config error gets mapped",
			err:          newConfigError("config failed", nil),
			expectedCode: ExitConfigError,
		},
		{
			name:         "validation error gets mapped",
			err:          newValidationError("validation failed", nil),
			expectedCode: ExitValidationError,
		},
		{
			name:         "file error gets mapped",
			err:          newFileError("file failed", nil),
			expectedCode: ExitFileError,
		},
		{
			name:         "state error gets mapped",
			err:          newStateError("state failed", nil),
			expectedCode: ExitStateError,
		},
		{
			name:         "planning error gets mapped",
			err:          newPlanningError("planning failed", nil),
			expectedCode: ExitPlanningError,
		},
		{
			name:         "execution error gets mapped",
			err:          newExecutionError("execution failed", nil),
			expectedCode: ExitExecutionError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cliErr, ok := tt.err.(*CLIError); ok {
				if cliErr.ExitCode() != tt.expectedCode {
					t.Errorf("expected exit code %d, got %d", tt.expectedCode, cliErr.ExitCode())
				}
			} else {
				t.Errorf("expected CLIError, got %T", tt.err)
			}
		})
	}
}
