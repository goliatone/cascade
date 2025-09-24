package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// commandRunner implements CommandRunner using os/exec.
type commandRunner struct{}

// NewCommandRunner creates a CommandRunner implementation.
func NewCommandRunner() CommandRunner {
	return &commandRunner{}
}

func (c *commandRunner) Run(ctx context.Context, repoPath string, cmd manifest.Command, env map[string]string, timeout time.Duration) (CommandResult, error) {
	result := CommandResult{
		Command: cmd,
	}

	// Handle empty command
	if len(cmd.Cmd) == 0 {
		return result, &CommandExecutionError{
			Command: cmd.Cmd,
			Dir:     repoPath,
			Err:     ErrEmptyCommand,
		}
	}

	// Set up timeout context
	if timeout <= 0 {
		timeout = 5 * time.Minute // default timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Set working directory
	workDir := repoPath
	if cmd.Dir != "" {
		workDir = filepath.Join(repoPath, cmd.Dir)
	}

	// Create command
	execCmd := exec.CommandContext(ctx, cmd.Cmd[0], cmd.Cmd[1:]...)
	execCmd.Dir = workDir

	// Set up environment
	execCmd.Env = prepareEnv(env)

	// Execute command and capture output
	output, err := execCmd.CombinedOutput()
	result.Output = string(output)

	if err != nil {
		cmdErr := &CommandExecutionError{
			Command:  cmd.Cmd,
			Dir:      workDir,
			Output:   string(output),
			ExitCode: getExitCode(err),
			Err:      err,
		}
		result.Err = cmdErr
		return result, cmdErr
	}

	return result, nil
}

// prepareEnv merges custom environment variables with the current environment
func prepareEnv(custom map[string]string) []string {
	env := os.Environ()

	for k, v := range custom {
		env = append(env, k+"="+v)
	}

	return env
}

// getExitCode extracts the exit code from an exec error
func getExitCode(err error) int {
	if exitError, ok := err.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}
	return -1
}
