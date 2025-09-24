package executor

import (
	"errors"
	"fmt"
)

// Common errors
var (
	ErrEmptyCommand = errors.New("command cannot be empty")
)

// GitOperationError wraps git failures with context.
type GitOperationError struct {
	Repo      string
	Operation string
	Err       error
}

func (e *GitOperationError) Error() string {
	return fmt.Sprintf("executor: git %s failed for %s: %v", e.Operation, e.Repo, e.Err)
}

func (e *GitOperationError) Unwrap() error {
	return e.Err
}

// GoOperationError wraps go tool failures.
type GoOperationError struct {
	Module  string
	Version string
	Err     error
}

func (e *GoOperationError) Error() string {
	return fmt.Sprintf("executor: go operation failed for %s@%s: %v", e.Module, e.Version, e.Err)
}

func (e *GoOperationError) Unwrap() error {
	return e.Err
}

// CommandExecutionError wraps command failures.
type CommandExecutionError struct {
	Command  []string
	Dir      string
	Output   string
	ExitCode int
	Err      error
}

func (e *CommandExecutionError) Error() string {
	return fmt.Sprintf("executor: command %v failed in %s (exit %d): %v", e.Command, e.Dir, e.ExitCode, e.Err)
}

func (e *CommandExecutionError) Unwrap() error {
	return e.Err
}

// WorkspaceError wraps workspace lifecycle failures.
type WorkspaceError struct {
	Path      string
	Operation string
	Err       error
}

func (e *WorkspaceError) Error() string {
	return fmt.Sprintf("executor: workspace %s %s failed: %v", e.Operation, e.Path, e.Err)
}

func (e *WorkspaceError) Unwrap() error {
	return e.Err
}

func IsGitError(err error) bool {
	var target *GitOperationError
	return errors.As(err, &target)
}

func IsGoError(err error) bool {
	var target *GoOperationError
	return errors.As(err, &target)
}

func IsCommandError(err error) bool {
	var target *CommandExecutionError
	return errors.As(err, &target)
}

func IsWorkspaceError(err error) bool {
	var target *WorkspaceError
	return errors.As(err, &target)
}
