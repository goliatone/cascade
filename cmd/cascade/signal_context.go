package main

import (
	"context"
	"os/signal"
)

func signalAwareContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return signal.NotifyContext(parent, watchSignals()...)
}
