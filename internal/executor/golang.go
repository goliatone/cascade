package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// goOperations implements GoOperations using the system go tool.
type goOperations struct{}

// NewGoOperations creates a GoOperations implementation that shells out to go tool.
func NewGoOperations() GoOperations {
	return &goOperations{}
}

// Get updates a module to the specified version using go get.
func (g *goOperations) Get(ctx context.Context, repoPath, module, version string) error {
	// Construct go get command with module@version format
	var args []string
	if version == "" || version == "latest" {
		args = []string{"get", module}
	} else {
		args = []string{"get", fmt.Sprintf("%s@%s", module, version)}
	}

	// Execute go get command
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Include both stdout and stderr in error for diagnostics
		output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return &GoOperationError{
			Module:  module,
			Version: version,
			Err:     fmt.Errorf("go get failed: %w\nOutput: %s", err, output),
		}
	}

	return nil
}

// Tidy runs go mod tidy to clean up the module dependencies.
func (g *goOperations) Tidy(ctx context.Context, repoPath string) error {
	// Execute go mod tidy command
	cmd := exec.CommandContext(ctx, "go", "mod", "tidy")
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Include both stdout and stderr in error for diagnostics
		output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return &GoOperationError{
			Module:  "", // no specific module for tidy
			Version: "",
			Err:     fmt.Errorf("go mod tidy failed: %w\nOutput: %s", err, output),
		}
	}

	return nil
}
