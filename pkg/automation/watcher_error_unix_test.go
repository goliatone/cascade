//go:build unix

package automation

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
)

func TestIsInterruptedSystemCall(t *testing.T) {
	err := fmt.Errorf("watch backend: %w", os.NewSyscallError("kevent", syscall.EINTR))
	if !isInterruptedSystemCall(err) {
		t.Fatalf("isInterruptedSystemCall(%v) = false, want true", err)
	}
	if got := wrapWatchError(err); got != nil {
		t.Fatalf("wrapWatchError(%v) = %v, want nil", err, got)
	}
}

func TestWrapWatchErrorKeepsFatalErrors(t *testing.T) {
	err := errors.New("backend closed unexpectedly")
	got := wrapWatchError(err)
	if !errors.Is(got, ErrWatchFailed) {
		t.Fatalf("wrapWatchError() = %v, want ErrWatchFailed", got)
	}
}
