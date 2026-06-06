//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
)

func TestWatchSignalsIncludeInterruptAndTERM(t *testing.T) {
	signals := watchSignals()
	if !containsSignal(signals, os.Interrupt) {
		t.Fatal("watchSignals() missing os.Interrupt")
	}
	if !containsSignal(signals, syscall.SIGTERM) {
		t.Fatal("watchSignals() missing syscall.SIGTERM")
	}
}

func containsSignal(signals []os.Signal, want os.Signal) bool {
	for _, signal := range signals {
		if signal == want {
			return true
		}
	}
	return false
}
