package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"runtime"
	"time"
)

type ShellExecutor struct {
	Env              func() []string
	TerminationGrace time.Duration
}

func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{Env: processEnv}
}

func (e *ShellExecutor) Execute(ctx context.Context, req RunRequest) (RunResult, error) {
	started := time.Now()
	result := RunResult{
		ID:        req.ID,
		Event:     req.Event,
		StartedAt: started,
		ExitCode:  -1,
	}

	runCtx := ctx
	cancel := func() {}
	if req.Exec.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Exec.Timeout)
	}
	defer cancel()

	cmd := shellCommand(req.Exec)
	cmd.Dir = req.Exec.Dir
	cmd.Env = BuildCommandEnv(e.env(), req.Exec, req)
	cmd.Stdin = req.Exec.Stdin
	cmd.Stdout = req.Exec.Stdout
	cmd.Stderr = req.Exec.Stderr
	configureCommandForCancel(cmd)

	err := e.runCommand(runCtx, cmd)
	result.FinishedAt = time.Now()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	switch {
	case err == nil:
		return result, nil
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		result.TimedOut = true
		result.Err = runCtx.Err()
		return result, fmt.Errorf("%w: command timed out after %s", ErrExecutionFailed, req.Exec.Timeout)
	case errors.Is(runCtx.Err(), context.Canceled), errors.Is(ctx.Err(), context.Canceled):
		result.Canceled = true
		result.Err = runCtx.Err()
		return result, fmt.Errorf("%w: command canceled", ErrExecutionFailed)
	default:
		result.Err = err
		if result.ExitCode >= 0 {
			return result, fmt.Errorf("%w: command exited with code %d", ErrExecutionFailed, result.ExitCode)
		}
		return result, fmt.Errorf("%w: %v", ErrExecutionFailed, err)
	}
}

func (e *ShellExecutor) runCommand(ctx context.Context, cmd *osexec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = terminateProcessTree(cmd.Process)
		select {
		case err := <-done:
			return err
		case <-time.After(e.terminationGrace()):
			_ = killProcessTree(cmd.Process)
			return <-done
		}
	}
}

func (e *ShellExecutor) env() []string {
	if e != nil && e.Env != nil {
		return e.Env()
	}
	return os.Environ()
}

func (e *ShellExecutor) terminationGrace() time.Duration {
	if e != nil && e.TerminationGrace > 0 {
		return e.TerminationGrace
	}
	return 2 * time.Second
}

func shellCommand(spec ExecSpec) *osexec.Cmd {
	if spec.Shell != "" {
		return osexec.Command(spec.Shell, "-c", spec.Command)
	}
	if runtime.GOOS == "windows" {
		return osexec.Command("cmd", "/C", spec.Command)
	}
	return osexec.Command("sh", "-c", spec.Command)
}
