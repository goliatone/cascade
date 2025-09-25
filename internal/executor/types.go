package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

// Executor orchestrates the execution of work items.
type Executor interface {
	Apply(ctx context.Context, input WorkItemContext) (*Result, error)
}

// WorkItemContext provides all dependencies needed to execute a work item.
type WorkItemContext struct {
	Item      planner.WorkItem
	Workspace string
	Git       GitOperations
	Go        GoOperations
	Runner    CommandRunner
	Logger    Logger
}

// GitOperations defines the interface for git repository operations.
type GitOperations interface {
	EnsureClone(ctx context.Context, repo, workspace string) (string, error)
	EnsureWorktree(ctx context.Context, repoPath, branch string, base string) (string, error)
	Commit(ctx context.Context, repoPath, message string) (string, error)
	Push(ctx context.Context, repoPath, branch string) error
}

// GoOperations defines the interface for Go module operations.
type GoOperations interface {
	Get(ctx context.Context, repoPath, module, version string) error
	Tidy(ctx context.Context, repoPath string) error
}

// CommandRunner defines the interface for executing commands.
type CommandRunner interface {
	Run(ctx context.Context, repoPath string, cmd manifest.Command, env map[string]string, timeout time.Duration) (CommandResult, error)
}

// Logger defines the interface for logging.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
}

// Result represents the outcome of executing a work item.
type Result struct {
	Status       Status
	Reason       string
	CommitHash   string
	TestResults  []CommandResult
	ExtraResults []CommandResult
}

// CommandResult represents the outcome of executing a single command.
type CommandResult struct {
	Command manifest.Command `json:"command"`
	Output  string           `json:"output"`
	Err     error            `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for CommandResult
func (cr CommandResult) MarshalJSON() ([]byte, error) {
	type Alias CommandResult
	var errStr *string
	if cr.Err != nil {
		s := cr.Err.Error()
		errStr = &s
	}
	return json.Marshal(&struct {
		Alias
		Err *string `json:"err,omitempty"`
	}{
		Alias: Alias(cr),
		Err:   errStr,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for CommandResult
func (cr *CommandResult) UnmarshalJSON(data []byte) error {
	type Alias CommandResult
	aux := &struct {
		*Alias
		Err *string `json:"err,omitempty"`
	}{
		Alias: (*Alias)(cr),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Err != nil {
		cr.Err = errors.New(*aux.Err)
	}
	return nil
}

// Status represents the execution status of a work item.
type Status string

const (
	StatusCompleted    Status = "completed"
	StatusManualReview Status = "manual-review"
	StatusFailed       Status = "failed"
	StatusSkipped      Status = "skipped"
)

// NotImplementedError is returned by stub implementations.
type NotImplementedError struct {
	Operation string
}

func (e *NotImplementedError) Error() string {
	return "not implemented: " + e.Operation
}

// GitCommandRunner defines the interface for executing git commands.
type GitCommandRunner interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

// GitError represents errors from git operations.
type GitError struct {
	Operation string
	Args      []string
	Dir       string
	Err       error
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s failed in %s: %v", e.Operation, e.Dir, e.Err)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

// ErrNoChanges is returned when there are no changes to commit.
var ErrNoChanges = fmt.Errorf("no changes to commit")

// ErrInvalidRepo is returned when a repository is invalid or doesn't match expected origin.
type ErrInvalidRepo struct {
	Path     string
	Expected string
	Actual   string
}

func (e *ErrInvalidRepo) Error() string {
	return fmt.Sprintf("directory %s contains repository %s, expected %s", e.Path, e.Actual, e.Expected)
}
