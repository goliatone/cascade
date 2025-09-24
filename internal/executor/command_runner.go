package executor

import (
	"context"
	"os/exec"
	"strings"
)

// defaultGitCommandRunner implements GitCommandRunner using os/exec.
type defaultGitCommandRunner struct{}

// NewDefaultGitCommandRunner creates a new GitCommandRunner that shells out to git.
func NewDefaultGitCommandRunner() GitCommandRunner {
	return &defaultGitCommandRunner{}
}

// Run executes a git command in the specified directory.
func (r *defaultGitCommandRunner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))

	if err != nil {
		return result, &GitError{
			Operation: strings.Join(args, " "),
			Args:      args,
			Dir:       dir,
			Err:       err,
		}
	}

	return result, nil
}
