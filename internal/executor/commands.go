package executor

import (
	"context"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// commandRunner implements CommandRunner using os/exec.
type commandRunner struct{}

// NewCommandRunner creates a stub CommandRunner implementation.
func NewCommandRunner() CommandRunner {
	return &commandRunner{}
}

func (c *commandRunner) Run(ctx context.Context, repoPath string, cmd manifest.Command, env map[string]string, timeout time.Duration) (CommandResult, error) {
	return CommandResult{}, &NotImplementedError{Operation: "commandRunner.Run"}
}
