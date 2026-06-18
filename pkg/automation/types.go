package automation

import (
	"context"
	"io"
	"time"
)

const DefaultDebounce = 750 * time.Millisecond

type Operation string

const (
	OperationUnknown Operation = "unknown"
	OperationCreate  Operation = "create"
	OperationWrite   Operation = "write"
	OperationRemove  Operation = "remove"
	OperationRename  Operation = "rename"
	OperationChmod   Operation = "chmod"
)

type WatchTarget struct {
	Path string
}

type Event struct {
	Path      string
	Op        Operation
	WatchRoot string
	Time      time.Time
}

type ExecSpec struct {
	Command string
	Dir     string
	Timeout time.Duration
	Env     map[string]string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Shell   string
}

type Workflow struct {
	Targets         []WatchTarget
	Exec            ExecSpec
	Debounce        time.Duration
	AllowConcurrent bool
	Recursive       bool
}

type RunnerOptions struct {
	Source   EventSource
	Executor Executor
	Logger   Logger
}

type Logger interface {
	Printf(format string, args ...any)
}

type EventSource interface {
	Watch(ctx context.Context, targets []WatchTarget) (<-chan Event, <-chan error, error)
}

type Executor interface {
	Execute(ctx context.Context, req RunRequest) (RunResult, error)
}

type RunRequest struct {
	ID    string
	Event Event
	Exec  ExecSpec
}

type RunResult struct {
	ID         string
	Event      Event
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	TimedOut   bool
	Canceled   bool
	Err        error
}
