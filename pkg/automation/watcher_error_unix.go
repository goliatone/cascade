//go:build unix

package automation

import (
	"errors"
	"syscall"
)

func isInterruptedSystemCall(err error) bool {
	return errors.Is(err, syscall.EINTR)
}
