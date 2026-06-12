package automation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShellExecutorSuccessStreamsOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	result, err := NewShellExecutor().Execute(context.Background(), RunRequest{
		ID:    "run-1",
		Event: Event{Path: ".version", Op: OperationWrite, WatchRoot: ".version"},
		Exec: ExecSpec{
			Command: "printf 'out'; printf 'err' >&2",
			Stdout:  &stdout,
			Stderr:  &stderr,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if stdout.String() != "out" {
		t.Fatalf("stdout = %q, want out", stdout.String())
	}
	if stderr.String() != "err" {
		t.Fatalf("stderr = %q, want err", stderr.String())
	}
}

func TestShellExecutorNonZeroExit(t *testing.T) {
	result, err := NewShellExecutor().Execute(context.Background(), RunRequest{
		ID:    "run-1",
		Event: Event{Path: ".version"},
		Exec:  ExecSpec{Command: "exit 7"},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want non-zero exit error")
	}
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("Execute() error = %v, want ErrExecutionFailed", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestShellExecutorTimeout(t *testing.T) {
	result, err := NewShellExecutor().Execute(context.Background(), RunRequest{
		ID:    "run-1",
		Event: Event{Path: ".version"},
		Exec: ExecSpec{
			Command: "sleep 2",
			Timeout: 20 * time.Millisecond,
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout")
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
}

func TestShellExecutorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := NewShellExecutor().Execute(ctx, RunRequest{
		ID:    "run-1",
		Event: Event{Path: ".version"},
		Exec:  ExecSpec{Command: "sleep 2"},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want cancellation")
	}
	if !result.Canceled {
		t.Fatal("Canceled = false, want true")
	}
}

func TestShellExecutorWorkingDirectoryAndEnv(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer

	_, err := NewShellExecutor().Execute(context.Background(), RunRequest{
		ID: "run-42",
		Event: Event{
			Path:      filepath.Join(dir, ".version"),
			Op:        OperationWrite,
			WatchRoot: filepath.Join(dir, ".version"),
			Time:      time.Unix(1, 2),
		},
		Exec: ExecSpec{
			Command: "printf '%s|%s|%s|%s' \"$PWD\" \"$CUSTOM\" \"$AUTOMATION_RUN_ID\" \"$AUTOMATION_EVENT_OP\"",
			Dir:     dir,
			Env:     map[string]string{"CUSTOM": "value"},
			Stdout:  &stdout,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	parts := strings.Split(stdout.String(), "|")
	if len(parts) != 4 {
		t.Fatalf("stdout parts = %q", stdout.String())
	}
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := parts[0], wantDir; got != want {
		t.Fatalf("working dir = %q, want %q", got, want)
	}
	if parts[1] != "value" {
		t.Fatalf("custom env = %q, want value", parts[1])
	}
	if parts[2] != "run-42" {
		t.Fatalf("run id = %q, want run-42", parts[2])
	}
	if parts[3] != string(OperationWrite) {
		t.Fatalf("event op = %q, want %q", parts[3], OperationWrite)
	}
}

func TestBuildCommandEnvOverridesProcessEnv(t *testing.T) {
	env := BuildCommandEnv([]string{"A=one", "B=two"}, ExecSpec{
		Env: map[string]string{"B": "override"},
	}, RunRequest{
		ID:    "run-1",
		Event: Event{Path: "p", Op: OperationCreate, WatchRoot: "r"},
	})

	got := envMap(env)
	if got["A"] != "one" {
		t.Fatalf("A = %q, want one", got["A"])
	}
	if got["B"] != "override" {
		t.Fatalf("B = %q, want override", got["B"])
	}
	if got["AUTOMATION_EVENT_PATH"] != "p" {
		t.Fatalf("event path = %q, want p", got["AUTOMATION_EVENT_PATH"])
	}
}

func TestShellExecutorInvalidWorkingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := NewShellExecutor().Execute(context.Background(), RunRequest{
		ID:    "run-1",
		Event: Event{Path: ".version"},
		Exec:  ExecSpec{Command: "pwd", Dir: missing, Stdout: os.Stdout},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want missing directory error")
	}
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("Execute() error = %v, want ErrExecutionFailed", err)
	}
}
