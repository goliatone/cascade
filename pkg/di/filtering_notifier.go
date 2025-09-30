package di

import (
	"context"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

// FilteringNotifier wraps a notifier and filters notifications based on success/failure flags.
type FilteringNotifier struct {
	notifier  broker.Notifier
	onSuccess bool
	onFailure bool
	logger    Logger
}

// NewFilteringNotifier creates a new filtering notifier that respects on_success and on_failure flags.
func NewFilteringNotifier(notifier broker.Notifier, onSuccess, onFailure bool, logger Logger) broker.Notifier {
	return &FilteringNotifier{
		notifier:  notifier,
		onSuccess: onSuccess,
		onFailure: onFailure,
		logger:    logger,
	}
}

// Send sends a notification only if the result status matches the configured flags.
func (f *FilteringNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
	if result == nil {
		return nil, nil
	}

	// Determine if this is a success or failure
	isSuccess := result.Status == executor.StatusCompleted || result.Status == executor.StatusManualReview
	isFailure := result.Status == executor.StatusFailed || result.Status == executor.StatusSkipped

	// Check if we should send based on the flags
	shouldSend := (isSuccess && f.onSuccess) || (isFailure && f.onFailure)

	if !shouldSend {
		f.logger.Debug("Skipping notification based on manifest flags",
			"repo", item.Repo,
			"status", result.Status,
			"on_success", f.onSuccess,
			"on_failure", f.onFailure)
		return &broker.NotificationResult{
			Channel: "filtered",
			Message: "notification skipped based on on_success/on_failure flags",
		}, nil
	}

	// Send the notification
	return f.notifier.Send(ctx, item, result)
}
