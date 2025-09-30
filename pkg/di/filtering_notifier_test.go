package di

import (
	"context"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/planner"
)

type mockNotifierForFiltering struct {
	called bool
	result *broker.NotificationResult
	err    error
}

func (m *mockNotifierForFiltering) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
	m.called = true
	if m.result != nil {
		return m.result, m.err
	}
	return &broker.NotificationResult{
		Channel: "mock",
		Message: "sent",
	}, m.err
}

func TestFilteringNotifier_OnSuccessOnly(t *testing.T) {
	mock := &mockNotifierForFiltering{}
	logger := testLogger{}
	filtering := NewFilteringNotifier(mock, true, false, logger)

	item := planner.WorkItem{Repo: "test/repo"}

	// Test with success status
	successResult := &executor.Result{Status: executor.StatusCompleted}
	_, err := filtering.Send(context.Background(), item, successResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected notifier to be called for success when on_success=true")
	}

	// Test with failure status
	mock.called = false
	failureResult := &executor.Result{Status: executor.StatusFailed}
	_, err = filtering.Send(context.Background(), item, failureResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected notifier NOT to be called for failure when on_failure=false")
	}
}

func TestFilteringNotifier_OnFailureOnly(t *testing.T) {
	mock := &mockNotifierForFiltering{}
	logger := testLogger{}
	filtering := NewFilteringNotifier(mock, false, true, logger)

	item := planner.WorkItem{Repo: "test/repo"}

	// Test with failure status
	failureResult := &executor.Result{Status: executor.StatusFailed}
	_, err := filtering.Send(context.Background(), item, failureResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected notifier to be called for failure when on_failure=true")
	}

	// Test with success status
	mock.called = false
	successResult := &executor.Result{Status: executor.StatusCompleted}
	_, err = filtering.Send(context.Background(), item, successResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected notifier NOT to be called for success when on_success=false")
	}
}

func TestFilteringNotifier_BothEnabled(t *testing.T) {
	mock := &mockNotifierForFiltering{}
	logger := testLogger{}
	filtering := NewFilteringNotifier(mock, true, true, logger)

	item := planner.WorkItem{Repo: "test/repo"}

	// Test with success status
	successResult := &executor.Result{Status: executor.StatusCompleted}
	_, err := filtering.Send(context.Background(), item, successResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected notifier to be called for success when on_success=true")
	}

	// Test with failure status
	mock.called = false
	failureResult := &executor.Result{Status: executor.StatusFailed}
	_, err = filtering.Send(context.Background(), item, failureResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected notifier to be called for failure when on_failure=true")
	}
}

func TestFilteringNotifier_BothDisabled(t *testing.T) {
	mock := &mockNotifierForFiltering{}
	logger := testLogger{}
	filtering := NewFilteringNotifier(mock, false, false, logger)

	item := planner.WorkItem{Repo: "test/repo"}

	// Test with success status
	successResult := &executor.Result{Status: executor.StatusCompleted}
	_, err := filtering.Send(context.Background(), item, successResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected notifier NOT to be called for success when on_success=false")
	}

	// Test with failure status
	mock.called = false
	failureResult := &executor.Result{Status: executor.StatusFailed}
	_, err = filtering.Send(context.Background(), item, failureResult)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.called {
		t.Error("expected notifier NOT to be called for failure when on_failure=false")
	}
}

func TestFilteringNotifier_ManualReviewTreatedAsSuccess(t *testing.T) {
	mock := &mockNotifierForFiltering{}
	logger := testLogger{}
	filtering := NewFilteringNotifier(mock, true, false, logger)

	item := planner.WorkItem{Repo: "test/repo"}
	result := &executor.Result{Status: executor.StatusManualReview}

	_, err := filtering.Send(context.Background(), item, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected notifier to be called for manual review when on_success=true")
	}
}

func TestFilteringNotifier_SkippedTreatedAsFailure(t *testing.T) {
	mock := &mockNotifierForFiltering{}
	logger := testLogger{}
	filtering := NewFilteringNotifier(mock, false, true, logger)

	item := planner.WorkItem{Repo: "test/repo"}
	result := &executor.Result{Status: executor.StatusSkipped}

	_, err := filtering.Send(context.Background(), item, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.called {
		t.Error("expected notifier to be called for skipped when on_failure=true")
	}
}
