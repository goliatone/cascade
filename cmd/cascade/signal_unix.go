//go:build !windows

package main

import (
	"os"
	"syscall"
)

func watchSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
