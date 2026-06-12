//go:build !windows

package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestShellExecutorTimeoutKillsChildProcess(t *testing.T) {
	dir := t.TempDir()
	childPath := filepath.Join(dir, "child.pid")

	result, err := (&ShellExecutor{
		Env:              processEnv,
		TerminationGrace: 50 * time.Millisecond,
	}).Execute(context.Background(), RunRequest{
		ID:    "run-1",
		Event: Event{Path: ".version"},
		Exec: ExecSpec{
			Command: fmt.Sprintf("sleep 10 & printf '%%s' \"$!\" > %s; wait", singleQuote(childPath)),
			Timeout: 50 * time.Millisecond,
		},
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout")
	}
	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}

	childPID := readPID(t, childPath)
	if childPID <= 0 {
		t.Fatalf("child pid = %d, want positive pid", childPID)
	}
	if processExists(childPID) {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatalf("child process %d is still alive after timeout", childPID)
	}
}

func readPID(t *testing.T, path string) int {
	t.Helper()

	var data []byte
	var err error
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		data, err = os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(data)) != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", string(data), err)
	}
	return pid
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
