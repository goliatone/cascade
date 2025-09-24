package executor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

func TestCommandRunner_Run(t *testing.T) {
	runner := NewCommandRunner()

	tests := []struct {
		name        string
		repoPath    string
		cmd         manifest.Command
		env         map[string]string
		timeout     time.Duration
		wantErr     bool
		wantErrType error
		checkOutput func(t *testing.T, result CommandResult)
	}{
		{
			name:     "successful command execution",
			repoPath: createTempDir(t),
			cmd: manifest.Command{
				Cmd: []string{"echo", "hello world"},
			},
			timeout: 10 * time.Second,
			wantErr: false,
			checkOutput: func(t *testing.T, result CommandResult) {
				if !strings.Contains(result.Output, "hello world") {
					t.Errorf("expected output to contain 'hello world', got: %s", result.Output)
				}
			},
		},
		{
			name:     "command with custom directory",
			repoPath: createTempDir(t),
			cmd: manifest.Command{
				Cmd: []string{"pwd"},
				Dir: "subdir",
			},
			timeout: 10 * time.Second,
			wantErr: false,
			checkOutput: func(t *testing.T, result CommandResult) {
				if !strings.Contains(result.Output, "subdir") {
					t.Errorf("expected output to contain 'subdir', got: %s", result.Output)
				}
			},
		},
		{
			name:     "command with environment variables",
			repoPath: createTempDir(t),
			cmd:      getEnvCommand(),
			env: map[string]string{
				"TEST_VAR": "test_value",
			},
			timeout: 10 * time.Second,
			wantErr: false,
			checkOutput: func(t *testing.T, result CommandResult) {
				if !strings.Contains(result.Output, "TEST_VAR=test_value") && !strings.Contains(result.Output, "test_value") {
					t.Errorf("expected output to contain TEST_VAR=test_value, got: %s", result.Output)
				}
			},
		},
		{
			name:     "command failure",
			repoPath: createTempDir(t),
			cmd: manifest.Command{
				Cmd: []string{"false"}, // command that always fails
			},
			timeout:     10 * time.Second,
			wantErr:     true,
			wantErrType: &CommandExecutionError{},
		},
		{
			name:     "command not found",
			repoPath: createTempDir(t),
			cmd: manifest.Command{
				Cmd: []string{"nonexistent_command_12345"},
			},
			timeout:     10 * time.Second,
			wantErr:     true,
			wantErrType: &CommandExecutionError{},
		},
		{
			name:     "empty command",
			repoPath: createTempDir(t),
			cmd: manifest.Command{
				Cmd: []string{},
			},
			timeout:     10 * time.Second,
			wantErr:     true,
			wantErrType: &CommandExecutionError{},
		},
		{
			name:        "command timeout",
			repoPath:    createTempDir(t),
			cmd:         getSleepCommand(2),     // sleep for 2 seconds
			timeout:     100 * time.Millisecond, // timeout after 100ms
			wantErr:     true,
			wantErrType: &CommandExecutionError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create subdirectory if needed
			if tt.cmd.Dir != "" {
				subdir := filepath.Join(tt.repoPath, tt.cmd.Dir)
				if err := os.MkdirAll(subdir, 0755); err != nil {
					t.Fatalf("failed to create subdir: %v", err)
				}
			}

			ctx := context.Background()
			result, err := runner.Run(ctx, tt.repoPath, tt.cmd, tt.env, tt.timeout)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.wantErrType != nil {
					if !isErrorType(err, tt.wantErrType) {
						t.Errorf("expected error type %T, got %T", tt.wantErrType, err)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if tt.checkOutput != nil {
					tt.checkOutput(t, result)
				}
			}

			// Verify result structure
			if result.Command.Cmd == nil && len(tt.cmd.Cmd) > 0 {
				t.Errorf("expected command to be set in result")
			}
		})
	}
}

func TestCommandRunner_ContextCancellation(t *testing.T) {
	runner := NewCommandRunner()
	repoPath := createTempDir(t)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context immediately
	cancel()

	result, err := runner.Run(ctx, repoPath, getSleepCommand(5), nil, 10*time.Second)

	if err == nil {
		t.Error("expected error due to context cancellation")
	}

	if !IsCommandError(err) {
		t.Errorf("expected CommandExecutionError, got %T", err)
	}

	// Result should still have command info
	if len(result.Command.Cmd) == 0 {
		t.Error("expected command to be set in result even on failure")
	}
}

func TestPrepareEnv(t *testing.T) {
	tests := []struct {
		name   string
		custom map[string]string
		check  func(t *testing.T, env []string)
	}{
		{
			name:   "nil custom env",
			custom: nil,
			check: func(t *testing.T, env []string) {
				// Should have at least PATH
				found := false
				for _, e := range env {
					if strings.HasPrefix(e, "PATH=") {
						found = true
						break
					}
				}
				if !found {
					t.Error("expected PATH to be in environment")
				}
			},
		},
		{
			name: "custom env vars",
			custom: map[string]string{
				"TEST_VAR1": "value1",
				"TEST_VAR2": "value2",
			},
			check: func(t *testing.T, env []string) {
				foundVar1, foundVar2 := false, false
				for _, e := range env {
					if e == "TEST_VAR1=value1" {
						foundVar1 = true
					}
					if e == "TEST_VAR2=value2" {
						foundVar2 = true
					}
				}
				if !foundVar1 || !foundVar2 {
					t.Error("expected custom env vars to be present")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := prepareEnv(tt.custom)
			tt.check(t, env)
		})
	}
}

func TestGetExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: -1,
		},
		{
			name:     "non-exec error",
			err:      context.DeadlineExceeded,
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getExitCode(tt.err)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// Helper functions

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "executor_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

func getEnvCommand() manifest.Command {
	if runtime.GOOS == "windows" {
		return manifest.Command{Cmd: []string{"cmd", "/c", "set"}}
	}
	return manifest.Command{Cmd: []string{"env"}}
}

func getSleepCommand(seconds int) manifest.Command {
	secondsStr := "2" // Use 2 seconds as default, could use fmt.Sprintf("%d", seconds) if needed
	if runtime.GOOS == "windows" {
		return manifest.Command{Cmd: []string{"timeout", "/t", secondsStr, "/nobreak"}}
	}
	return manifest.Command{Cmd: []string{"sleep", secondsStr}}
}

func isErrorType(err error, target error) bool {
	switch target.(type) {
	case *CommandExecutionError:
		return IsCommandError(err)
	default:
		return false
	}
}
