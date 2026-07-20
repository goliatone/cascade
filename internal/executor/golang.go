package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// goOperations implements GoOperations using the system go tool.
type goOperations struct {
	stdout   io.Writer
	stderr   io.Writer
	streamMu sync.Mutex
}

// NewGoOperations creates a GoOperations implementation that shells out to go tool.
func NewGoOperations() GoOperations {
	return &goOperations{}
}

// NewGoOperationsWithOutput creates Go operations that stream command output
// while retaining it for error diagnostics.
func NewGoOperationsWithOutput(stdout, stderr io.Writer) GoOperations {
	return &goOperations{stdout: stdout, stderr: stderr}
}

// Get updates a module to the specified version using go get.
func (g *goOperations) Get(ctx context.Context, repoPath, module, version string) error {
	return g.runGet(ctx, repoPath, []ModuleVersion{{Module: module, Version: version}})
}

// GetBatch updates multiple modules in one go get invocation.
func (g *goOperations) GetBatch(ctx context.Context, repoPath string, targets []ModuleVersion) error {
	if len(targets) == 0 {
		return nil
	}
	return g.runGet(ctx, repoPath, targets)
}

func (g *goOperations) runGet(ctx context.Context, repoPath string, targets []ModuleVersion) error {
	args := []string{"get"}
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		label := target.Module
		if target.Version != "" && target.Version != "latest" {
			label = fmt.Sprintf("%s@%s", target.Module, target.Version)
		}
		args = append(args, label)
		labels = append(labels, label)
	}

	// Execute go get command
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = bufferedWriter(&stdout, g.stdout, &g.streamMu)
	cmd.Stderr = bufferedWriter(&stderr, g.stderr, &g.streamMu)

	err := cmd.Run()
	if err != nil {
		// Include both stdout and stderr in error for diagnostics
		output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		moduleLabel := strings.Join(labels, " ")
		versionLabel := ""
		if len(targets) == 1 {
			moduleLabel = targets[0].Module
			versionLabel = targets[0].Version
		}
		return &GoOperationError{
			Module:  moduleLabel,
			Version: versionLabel,
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
	cmd.Stdout = bufferedWriter(&stdout, g.stdout, &g.streamMu)
	cmd.Stderr = bufferedWriter(&stderr, g.stderr, &g.streamMu)

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

func bufferedWriter(buffer *bytes.Buffer, stream io.Writer, mu *sync.Mutex) io.Writer {
	if stream == nil {
		return buffer
	}
	return io.MultiWriter(buffer, lockedWriter{writer: stream, mu: mu})
}

type lockedWriter struct {
	writer io.Writer
	mu     *sync.Mutex
}

func (w lockedWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writer.Write(data)
}
