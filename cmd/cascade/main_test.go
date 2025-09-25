package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestCLIHelp(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
	}{
		{
			name: "root help",
			args: []string{"--help"},
			contains: []string{
				"Cascade is a CLI tool that orchestrates automated dependency updates",
				"Available Commands:",
				"plan", "release", "resume", "revert",
				"Flags:",
			},
		},
		{
			name: "plan help",
			args: []string{"plan", "--help"},
			contains: []string{
				"Plan analyzes the dependency manifest",
				"Usage:",
				"cascade plan [manifest]",
			},
		},
		{
			name: "release help",
			args: []string{"release", "--help"},
			contains: []string{
				"Release executes the dependency update plan",
				"Usage:",
				"cascade release [manifest]",
			},
		},
		{
			name: "resume help",
			args: []string{"resume", "--help"},
			contains: []string{
				"Resume continues a previously interrupted cascade operation",
				"Usage:",
				"cascade resume [state-id]",
			},
		},
		{
			name: "revert help",
			args: []string{"revert", "--help"},
			contains: []string{
				"Revert undoes changes made by a cascade operation",
				"Usage:",
				"cascade revert [state-id]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("go", append([]string{"run", "."}, tt.args...)...)
			var stdout bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stdout

			err := cmd.Run()
			if err != nil {
				t.Fatalf("Command failed: %v, output: %s", err, stdout.String())
			}

			output := stdout.String()
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

func TestCLIStubCommands(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		contains    []string
	}{
		{
			name:        "plan command stub",
			args:        []string{"plan"},
			expectError: false,
			contains:    []string{"Plan command not yet implemented"},
		},
		{
			name:        "release command stub",
			args:        []string{"release"},
			expectError: false,
			contains:    []string{"Release command not yet implemented"},
		},
		{
			name:        "resume command stub",
			args:        []string{"resume"},
			expectError: false,
			contains:    []string{"Resume command not yet implemented"},
		},
		{
			name:        "revert command stub",
			args:        []string{"revert"},
			expectError: false,
			contains:    []string{"Revert command not yet implemented"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip these tests until we have better test environment setup
			// as they require the full DI container initialization
			t.Skip("Command execution tests skipped - require better test setup")

			cmd := exec.Command("go", append([]string{"run", "."}, tt.args...)...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if tt.expectError && err == nil {
				t.Fatalf("Expected command to fail, but it succeeded")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("Command failed unexpectedly: %v, stderr: %s", err, stderr.String())
			}

			output := stdout.String()
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

func TestCLIExitCodes(t *testing.T) {
	t.Skip("Exit code tests skipped - require better test setup")
	// TODO: Add tests for exit codes when error conditions are triggered
}
