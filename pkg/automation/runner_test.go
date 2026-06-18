package automation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEventSource struct {
	events chan Event
	errs   chan error
}

func newFakeEventSource() *fakeEventSource {
	return &fakeEventSource{
		events: make(chan Event, 16),
		errs:   make(chan error, 1),
	}
}

func (s *fakeEventSource) Watch(ctx context.Context, targets []WatchTarget) (<-chan Event, <-chan error, error) {
	return s.events, s.errs, nil
}

type recordingExecutor struct {
	mu       sync.Mutex
	requests []RunRequest
	started  chan RunRequest
	release  chan struct{}
}

func newRecordingExecutor(block bool) *recordingExecutor {
	e := &recordingExecutor{
		started: make(chan RunRequest, 16),
	}
	if block {
		e.release = make(chan struct{})
	}
	return e
}

func (e *recordingExecutor) Execute(ctx context.Context, req RunRequest) (RunResult, error) {
	e.mu.Lock()
	e.requests = append(e.requests, req)
	e.mu.Unlock()

	select {
	case e.started <- req:
	default:
	}

	if e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
		}
	}
	return RunResult{ID: req.ID, Event: req.Event}, nil
}

func (e *recordingExecutor) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.requests)
}

func TestRunnerDebouncesBurstEvents(t *testing.T) {
	source := newFakeEventSource()
	executor := newRecordingExecutor(false)
	runner := newTestRunner(t, source, executor, Workflow{Debounce: 20 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := runInBackground(ctx, runner)

	source.events <- Event{Path: "one", Op: OperationWrite}
	source.events <- Event{Path: "two", Op: OperationWrite}
	source.events <- Event{Path: "three", Op: OperationWrite}

	req := waitStarted(t, executor)
	if req.Event.Path != "three" {
		t.Fatalf("run event path = %q, want latest event", req.Event.Path)
	}
	if executor.count() != 1 {
		t.Fatalf("executor count = %d, want 1", executor.count())
	}

	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerQueuesOneRerunWhenBusy(t *testing.T) {
	source := newFakeEventSource()
	executor := newRecordingExecutor(true)
	runner := newTestRunner(t, source, executor, Workflow{Debounce: time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := runInBackground(ctx, runner)

	source.events <- Event{Path: "first", Op: OperationWrite}
	first := waitStarted(t, executor)
	if first.Event.Path != "first" {
		t.Fatalf("first path = %q", first.Event.Path)
	}

	source.events <- Event{Path: "second", Op: OperationWrite}
	source.events <- Event{Path: "third", Op: OperationWrite}
	time.Sleep(25 * time.Millisecond)
	if executor.count() != 1 {
		t.Fatalf("executor count while blocked = %d, want 1", executor.count())
	}

	executor.release <- struct{}{}
	second := waitStarted(t, executor)
	if second.Event.Path != "third" {
		t.Fatalf("queued run path = %q, want latest pending event", second.Event.Path)
	}

	executor.release <- struct{}{}
	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerAllowsConcurrentRuns(t *testing.T) {
	source := newFakeEventSource()
	executor := newRecordingExecutor(true)
	runner := newTestRunner(t, source, executor, Workflow{
		Debounce:        time.Millisecond,
		AllowConcurrent: true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := runInBackground(ctx, runner)

	source.events <- Event{Path: "first", Op: OperationWrite}
	waitStarted(t, executor)
	source.events <- Event{Path: "second", Op: OperationWrite}
	waitStarted(t, executor)

	if executor.count() != 2 {
		t.Fatalf("executor count = %d, want 2 concurrent starts", executor.count())
	}

	executor.release <- struct{}{}
	executor.release <- struct{}{}
	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerReturnsWatcherErrors(t *testing.T) {
	source := newFakeEventSource()
	executor := newRecordingExecutor(false)
	runner := newTestRunner(t, source, executor, Workflow{Debounce: time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := runInBackground(ctx, runner)

	want := ErrWatchFailed
	source.errs <- want

	if err := <-errs; err != want {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

func TestRunnerWritesVerboseLifecycleLogs(t *testing.T) {
	var logs bytes.Buffer
	source := newFakeEventSource()
	executor := newRecordingExecutor(false)
	runner := newTestRunnerWithOptions(t, source, executor, Workflow{Debounce: time.Millisecond}, RunnerOptions{
		Logger: NewTextLogger(&logs),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := runInBackground(ctx, runner)

	source.events <- Event{Path: ".version", Op: OperationWrite}
	waitStarted(t, executor)

	cancel()
	if err := <-errs; err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := logs.String()
	for _, want := range []string{
		"automation: watching 1 target(s)",
		"automation: event write .version",
		"automation: starting run-1 for .version",
		"automation: finished run-1 with exit code 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("logs = %q, want %q", got, want)
		}
	}
}

func TestSchedulerLogsQueuedFollowUpRun(t *testing.T) {
	var logs bytes.Buffer
	executor := newRecordingExecutor(true)
	scheduler := NewScheduler(executor, ExecSpec{}, SchedulerOptions{
		Logger: NewTextLogger(&logs),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scheduler.Submit(ctx, Event{Path: "first", Op: OperationWrite})
	waitStarted(t, executor)
	scheduler.Submit(ctx, Event{Path: "second", Op: OperationWrite})

	executor.release <- struct{}{}
	executor.release <- struct{}{}
	scheduler.Wait()

	got := logs.String()
	if !strings.Contains(got, "automation: queued follow-up run for second") {
		t.Fatalf("logs = %q, want queued follow-up log", got)
	}
}

func TestSchedulerWritesExecutorErrorsToStderr(t *testing.T) {
	var stderr bytes.Buffer
	scheduler := NewScheduler(errorExecutor{}, ExecSpec{Stderr: &stderr}, SchedulerOptions{})
	scheduler.Submit(context.Background(), Event{Path: ".version"})
	scheduler.Wait()

	if !strings.Contains(stderr.String(), "automation:") {
		t.Fatalf("stderr = %q, want automation error", stderr.String())
	}
}

type errorExecutor struct{}

func (errorExecutor) Execute(ctx context.Context, req RunRequest) (RunResult, error) {
	return RunResult{ID: req.ID, Event: req.Event}, errors.New("boom")
}

func newTestRunner(t *testing.T, source *fakeEventSource, executor *recordingExecutor, workflow Workflow) *Runner {
	t.Helper()
	return newTestRunnerWithOptions(t, source, executor, workflow, RunnerOptions{})
}

func newTestRunnerWithOptions(t *testing.T, source *fakeEventSource, executor *recordingExecutor, workflow Workflow, opts RunnerOptions) *Runner {
	t.Helper()
	workflow.Targets = []WatchTarget{{Path: ".version"}}
	workflow.Exec = ExecSpec{Command: "echo changed"}
	opts.Source = source
	opts.Executor = executor
	runner, err := NewRunner(workflow, opts)
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	return runner
}

func runInBackground(ctx context.Context, runner *Runner) <-chan error {
	errs := make(chan error, 1)
	go func() {
		errs <- runner.Run(ctx)
	}()
	return errs
}

func waitStarted(t *testing.T, executor *recordingExecutor) RunRequest {
	t.Helper()
	select {
	case req := <-executor.started:
		return req
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for executor start")
		return RunRequest{}
	}
}
