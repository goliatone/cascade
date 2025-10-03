package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
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
				"manifest", "plan", "release", "resume", "revert",
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
		{
			name: "manifest help",
			args: []string{"manifest", "--help"},
			contains: []string{
				"Manifest management commands",
				"Available Commands:",
				"generate",
				"Usage:",
				"cascade manifest [command]",
			},
		},
		{
			name: "manifest generate help",
			args: []string{"manifest", "generate", "--help"},
			contains: []string{
				"Generate creates a new dependency manifest file",
				"--module-path string",
				"--version string",
				"--dependents strings",
				"--output string",
				"--slack-channel string",
				"Examples:",
				"cascade manifest generate --version=v1.2.3",
			},
		},
		{
			name: "manifest gen alias help",
			args: []string{"manifest", "gen", "--help"},
			contains: []string{
				"Generate creates a new dependency manifest file",
				"--module-path string",
				"--version string",
				"Aliases:",
				"generate, gen",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("go", append([]string{"run", "."}, tt.args...)...)
			workspace := t.TempDir()
			env := append(os.Environ(),
				"CASCADE_WORKSPACE="+workspace,
				"CASCADE_STATE_DIR="+workspace,
				"CASCADE_DRY_RUN=true",
			)
			cmd.Env = env
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

func TestCLISmokeTests(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectError  bool
		expectedExit int
		contains     []string
		notContains  []string
		setupFiles   func(string) error
		description  string
	}{
		{
			name:         "plan command missing arguments",
			args:         []string{"plan"},
			expectError:  true,
			expectedExit: 1, // Falls back to generic error due to file loading before validation
			contains:     []string{"failed to load manifest", "no such file or directory"},
		},
		{
			name:         "plan command with auto-detected module",
			args:         []string{"plan", "testdata/minimal_manifest.yaml"},
			expectError:  true,
			expectedExit: 1, // Planning fails because module not found in manifest
			contains:     []string{"target module not found", "github.com/goliatone/cascade"},
		},
		{
			name:         "plan command with dry-run and manifest",
			args:         []string{"plan", "testdata/minimal_manifest.yaml", "--module", "github.com/example/lib", "--version", "v1.2.3", "--dry-run"},
			expectError:  false, // Should succeed with explicit flags
			expectedExit: 0,
			contains:     []string{"DRY RUN", "Planning updates", "github.com/example/lib@v1.2.3"},
		},
		{
			name:         "plan command uses dependent overrides",
			args:         []string{"plan", "testdata/dependent_overrides_manifest.yaml", "--module", "github.com/example/module-a", "--version", "v1.2.3", "--dry-run"},
			setupFiles:   setupDependentOverridesWorkspace,
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"task dependent:test",
				"task dependent:lint",
			},
			description: "Plan output should surface commands defined in dependent overrides",
		},
		{
			name:         "release command missing manifest",
			args:         []string{"release"},
			expectError:  true,
			expectedExit: 1, // Note: CLI correctly returns 5, but test framework sees 1
			contains:     []string{"failed to load manifest"},
		},
		{
			name:         "resume command invalid state format",
			args:         []string{"resume", "invalid-state"},
			expectError:  true,
			expectedExit: 1, // Current implementation shows exit code 1
			contains:     []string{"state identifier must be in module@version format"},
		},
		{
			name:         "resume command missing state",
			args:         []string{"resume", "github.com/example/lib@v1.2.3"},
			expectError:  true,
			expectedExit: 1, // Current implementation shows exit code 1
			contains:     []string{"no saved state found"},
		},
		{
			name:         "revert command invalid state format",
			args:         []string{"revert", "invalid-state"},
			expectError:  true,
			expectedExit: 1, // Current implementation shows exit code 1
			contains:     []string{"state identifier must be in module@version format"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()

			if tt.setupFiles != nil {
				if err := tt.setupFiles(workspace); err != nil {
					t.Fatalf("failed to setup workspace: %v", err)
				}
			}

			argsWithWorkspace := append(tt.args, "--workspace="+workspace)
			cmd := exec.Command("go", append([]string{"run", "."}, argsWithWorkspace...)...)
			env := append(os.Environ(),
				"CASCADE_WORKSPACE="+workspace,
				"CASCADE_STATE_DIR="+workspace,
				"CASCADE_DRY_RUN=true",
			)
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			// Check exit code
			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected command to fail, but it succeeded. Output: %s", stdout.String())
				}
				if exitError, ok := err.(*exec.ExitError); ok {
					if exitError.ExitCode() != tt.expectedExit {
						t.Errorf("Expected exit code %d, got %d. Stderr: %s", tt.expectedExit, exitError.ExitCode(), stderr.String())
					}
				}
			} else if err != nil {
				t.Fatalf("Command failed unexpectedly: %v, stderr: %s", err, stderr.String())
			}

			output := stdout.String() + stderr.String()

			// Check expected content
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, got:\n%s", expected, output)
				}
			}

			// Check content that should not be present
			for _, notExpected := range tt.notContains {
				if strings.Contains(output, notExpected) {
					t.Errorf("Expected output to NOT contain %q, got:\n%s", notExpected, output)
				}
			}
		})
	}
}

func TestCLIManifestGenerateDiscovery(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		setupFiles   func(workspace string) error
		expectError  bool
		expectedExit int
		contains     []string
		notContains  []string
		description  string
	}{
		{
			name: "explicit dependents vs discovery - explicit wins",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/target/module",
				"--version=v1.2.3",
				"--dependents=owner/explicit-repo",
				"--dry-run",
				"--yes",
			},
			setupFiles: func(workspace string) error {
				// Create a workspace with discoverable modules that should be ignored
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"Generating manifest for github.com/target/module@v1.2.3",
				"Using 1 configured dependent repositories:",
				"owner/explicit-repo",
				"DRY RUN: Would write manifest to",
			},
			notContains: []string{
				"Discovery workspace:",
				"Discovered",
				"example/module-a",
				"example/module-b",
			},
			description: "When explicit dependents are provided, discovery should be skipped",
		},
		{
			name: "workspace discovery with findings",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/target/module",
				"--version=v1.2.3",
				"--dry-run",
				"--yes",
			},
			setupFiles: func(workspace string) error {
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"Generating manifest for github.com/target/module@v1.2.3",
				"Discovery workspace:",
				"Discovered",
				"dependent repositories:",
				"example/module-a",
				"example/module-b",
				"DRY RUN: Would write manifest to",
			},
			notContains: []string{
				"configured dependent repositories:",
			},
			description: "Discovery should find dependent modules in workspace",
		},
		{
			name: "workspace discovery with exclusions",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/target/module",
				"--version=v1.2.3",
				"--exclude=excluded",
				"--dry-run",
				"--yes",
			},
			setupFiles: func(workspace string) error {
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"Generating manifest for github.com/target/module@v1.2.3",
				"Discovery workspace:",
				"example/module-a",
				"example/module-b",
				"DRY RUN: Would write manifest to",
			},
			notContains: []string{
				"example/module-d", // Should be excluded
			},
			description: "Discovery should respect exclude patterns",
		},
		{
			name: "workspace discovery with inclusions",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/target/module",
				"--version=v1.2.3",
				"--include=module-a",
				"--dry-run",
				"--yes",
			},
			setupFiles: func(workspace string) error {
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"Generating manifest for github.com/target/module@v1.2.3",
				"Discovery workspace:",
				"example/module-a",
				"DRY RUN: Would write manifest to",
			},
			notContains: []string{
				"example/module-b", // Should be excluded by include pattern
				"example/module-d",
			},
			description: "Discovery should respect include patterns",
		},
		{
			name: "workspace discovery no findings",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/nonexistent/module",
				"--version=v1.2.3",
				"--dry-run",
				"--yes",
			},
			setupFiles: func(workspace string) error {
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"Generating manifest for github.com/nonexistent/module@v1.2.3",
				"Discovery workspace:",
				"No dependent repositories found or configured.",
				"DRY RUN: Would write manifest to",
			},
			notContains: []string{
				"Discovered",
				"example/module-a",
			},
			description: "Discovery should handle no findings gracefully",
		},
		{
			name: "dry-run shows summary without writing",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/target/module",
				"--version=v1.2.3",
				"--dry-run",
				"--yes",
			},
			setupFiles: func(workspace string) error {
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"DRY RUN: Would write manifest to",
				"--- Generated Manifest ---",
				"module: github.com/target/module",
				"dependents:",
			},
			notContains: []string{
				"Manifest generated successfully:",
				"File", "already exists",
			},
			description: "Dry-run should show manifest content without writing files",
		},
		{
			name: "non-interactive mode skips confirmation",
			args: []string{
				"manifest", "generate",
				"--module-path=github.com/target/module",
				"--version=v1.2.3",
				"--non-interactive",
				"--dry-run",
			},
			setupFiles: func(workspace string) error {
				return setupDiscoveryWorkspace(workspace)
			},
			expectError:  false,
			expectedExit: 0,
			contains: []string{
				"--- DRY RUN: Would proceed with manifest generation ---",
				"DRY RUN: Would write manifest to",
			},
			notContains: []string{
				"Proceed with manifest generation? [Y/n]:",
			},
			description: "Non-interactive mode should skip confirmation prompts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()

			// Set up test files if needed
			if tt.setupFiles != nil {
				if err := tt.setupFiles(workspace); err != nil {
					t.Fatalf("Failed to setup test files: %v", err)
				}
			}

			// Add workspace to args if not already specified
			argsWithWorkspace := append(tt.args, "--workspace="+workspace)

			cmd := exec.Command("go", append([]string{"run", "."}, argsWithWorkspace...)...)
			env := append(os.Environ(),
				"CASCADE_WORKSPACE="+workspace,
				"CASCADE_STATE_DIR="+workspace,
				"CASCADE_DRY_RUN=true",
			)
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			// Check exit code
			if tt.expectError {
				if err == nil {
					t.Fatalf("Expected command to fail, but it succeeded. Output: %s", stdout.String())
				}
				if exitError, ok := err.(*exec.ExitError); ok {
					if exitError.ExitCode() != tt.expectedExit {
						t.Errorf("Expected exit code %d, got %d. Stderr: %s", tt.expectedExit, exitError.ExitCode(), stderr.String())
					}
				}
			} else if err != nil {
				t.Fatalf("Command failed unexpectedly: %v, stderr: %s, stdout: %s", err, stderr.String(), stdout.String())
			}

			desc := tt.description
			if desc == "" {
				desc = tt.name
			}

			output := stdout.String() + stderr.String()

			// Check expected content
			for _, expected := range tt.contains {
				if !strings.Contains(output, expected) {
					t.Errorf("%s: expected output to contain %q, got:\n%s", desc, expected, output)
				}
			}

			// Check content that should not be present
			for _, notExpected := range tt.notContains {
				if strings.Contains(output, notExpected) {
					t.Errorf("%s: expected output to NOT contain %q, got:\n%s", desc, notExpected, output)
				}
			}
		})
	}
}

// setupDiscoveryWorkspace creates a test workspace with discoverable modules
func setupDiscoveryWorkspace(workspace string) error {
	modules := map[string]string{
		"module-a/go.mod": `module github.com/example/module-a

go 1.21

require github.com/target/module v1.0.0
`,
		"module-b/go.mod": `module github.com/example/module-b

go 1.21

require (
	github.com/target/module v1.0.0
	github.com/other/dep v1.5.0
)
`,
		"module-c/go.mod": `module github.com/example/module-c

go 1.21

require github.com/other/dependency v1.0.0
`,
		"excluded/module-d/go.mod": `module github.com/example/module-d

go 1.21

require github.com/target/module v1.0.0
`,
	}

	for path, content := range modules {
		fullPath := workspace + "/" + path
		if err := os.MkdirAll(strings.TrimSuffix(fullPath, "/go.mod"), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}

		// Also create go.sum files to make modules valid
		sumPath := strings.TrimSuffix(fullPath, ".mod") + ".sum"
		if err := os.WriteFile(sumPath, []byte(""), 0644); err != nil {
			return err
		}
	}

	return nil
}

func setupDependentOverridesWorkspace(workspace string) error {
	depPath := filepath.Join(workspace, "module-b")
	if err := os.MkdirAll(depPath, 0o755); err != nil {
		return err
	}

	goMod := `module github.com/example/module-b

 go 1.21

 require github.com/example/module-a v1.0.0
`
	if err := os.WriteFile(filepath.Join(depPath, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}

	dependentManifest := `manifest_version: 1
module:
  module: github.com/example/module-b
  tests:
    - cmd: [task, module:test]
  extra_commands:
    - cmd: [task, module:lint]
dependents:
  github.com/example/module-a:
    tests:
      - cmd: [task, dependent:test]
    extra_commands:
      - cmd: [task, dependent:lint]
    env:
      MODULE_B_ENV: "override"
`
	if err := os.WriteFile(filepath.Join(depPath, ".cascade.yaml"), []byte(dependentManifest), 0o644); err != nil {
		return err
	}

	return nil
}

func TestCLIExitCodes(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedExit int
		description  string
	}{
		{
			name:         "success help",
			args:         []string{"--help"},
			expectedExit: 0,
			description:  "Help should exit with code 0",
		},
		{
			name:         "validation error missing module",
			args:         []string{"plan", "testdata/minimal_manifest.yaml"},
			expectedExit: 1, // TODO: Implement proper exit code propagation through Cobra
			description:  "Missing required arguments should exit with validation error code",
		},
		{
			name:         "file error missing manifest",
			args:         []string{"plan", "nonexistent.yaml", "--module", "test", "--version", "v1.0.0"},
			expectedExit: 1, // TODO: Implement proper exit code propagation through Cobra
			description:  "Missing manifest file should exit with file error code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("go", append([]string{"run", "."}, tt.args...)...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			actualExit := 0
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					actualExit = exitError.ExitCode()
				} else {
					t.Fatalf("Unexpected error type: %v", err)
				}
			}

			if actualExit != tt.expectedExit {
				t.Errorf("%s: expected exit code %d, got %d. Output: %s",
					tt.description, tt.expectedExit, actualExit, stdout.String()+stderr.String())
			}
		})
	}
}
