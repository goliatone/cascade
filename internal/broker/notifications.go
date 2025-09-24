package broker

import (
	"context"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

// Notifier defines behaviour for sending notifications.
type Notifier interface {
	Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error)
}

// noopNotifier is a stub implementation used during bootstrapping.
type noopNotifier struct{}

func (n *noopNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*NotificationResult, error) {
	return nil, &NotImplementedError{Operation: "notifier.Send"}
}
