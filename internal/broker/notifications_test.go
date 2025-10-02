package broker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
)

// mockHTTPClient is a test double for HTTP client.
type mockHTTPClient struct {
	responses []mockResponse
	requests  []*http.Request
	index     int
}

type mockResponse struct {
	statusCode int
	body       string
	err        error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Store the request for verification
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(body))
	}
	m.requests = append(m.requests, req)

	if m.index >= len(m.responses) {
		return nil, fmt.Errorf("unexpected request")
	}

	resp := m.responses[m.index]
	m.index++

	if resp.err != nil {
		return nil, resp.err
	}

	return &http.Response{
		StatusCode: resp.statusCode,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     make(http.Header),
	}, nil
}

func TestSlackNotifier_Send_Success(t *testing.T) {
	client := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 200, body: `{"ok": true}`},
		},
	}

	config := DefaultNotificationConfig()
	config.MaxRetries = 0 // No retries for success case

	notifier := NewSlackNotifier("bot-token", "#channel", client, config)

	item := planner.WorkItem{
		Module:     "example.com/module",
		Repo:       "owner/repo",
		BranchName: "update-module",
	}

	result := &executor.Result{
		Status:     executor.StatusCompleted,
		Reason:     "All tests passed",
		CommitHash: "abc123def456",
	}

	notification, err := notifier.Send(context.Background(), item, result)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if notification == nil {
		t.Fatal("Expected notification result, got nil")
	}

	if notification.Channel != "#channel" {
		t.Errorf("Expected channel '#channel', got '%s'", notification.Channel)
	}

	if len(client.requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(client.requests))
	}

	req := client.requests[0]
	if req.Method != "POST" {
		t.Errorf("Expected POST request, got %s", req.Method)
	}

	if req.Header.Get("Authorization") != "Bearer bot-token" {
		t.Errorf("Expected Bearer token, got %s", req.Header.Get("Authorization"))
	}

	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected application/json content type, got %s", req.Header.Get("Content-Type"))
	}
}

func TestSlackNotifier_Send_Retry_Success(t *testing.T) {
	client := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 500, body: `{"error": "server error"}`}, // First attempt fails
			{statusCode: 200, body: `{"ok": true}`},              // Second attempt succeeds
		},
	}

	config := DefaultNotificationConfig()
	config.MaxRetries = 2
	config.RetryDelay = time.Millisecond * 10 // Fast retry for testing

	notifier := NewSlackNotifier("bot-token", "#channel", client, config)

	item := planner.WorkItem{Module: "example.com/module"}
	result := &executor.Result{Status: executor.StatusCompleted}

	notification, err := notifier.Send(context.Background(), item, result)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if notification == nil {
		t.Fatal("Expected notification result, got nil")
	}

	if len(client.requests) != 2 {
		t.Fatalf("Expected 2 requests (1 failure + 1 success), got %d", len(client.requests))
	}
}

func TestSlackNotifier_Send_AllRetries_Fail(t *testing.T) {
	client := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 500, body: `{"error": "server error"}`},
			{statusCode: 500, body: `{"error": "server error"}`},
			{statusCode: 500, body: `{"error": "server error"}`},
		},
	}

	config := DefaultNotificationConfig()
	config.MaxRetries = 2
	config.RetryDelay = time.Millisecond * 10

	notifier := NewSlackNotifier("bot-token", "#channel", client, config)

	item := planner.WorkItem{Module: "example.com/module"}
	result := &executor.Result{Status: executor.StatusCompleted}

	notification, err := notifier.Send(context.Background(), item, result)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var notifErr *NotificationError
	if !ErrorAs(err, &notifErr) {
		t.Fatalf("Expected NotificationError, got %T", err)
	}

	if notifErr.Channel != "#channel" {
		t.Errorf("Expected channel '#channel', got '%s'", notifErr.Channel)
	}

	if notification != nil {
		t.Errorf("Expected nil notification on failure, got %+v", notification)
	}

	if len(client.requests) != 3 {
		t.Fatalf("Expected 3 requests (2 retries + 1 initial), got %d", len(client.requests))
	}
}

func TestWebhookNotifier_Send_Success(t *testing.T) {
	client := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 200, body: `{"status": "ok"}`},
		},
	}

	config := DefaultNotificationConfig()
	notifier := NewWebhookNotifier("https://example.com/webhook", client, config)

	item := planner.WorkItem{
		Module: "example.com/module",
		Repo:   "owner/repo",
	}

	result := &executor.Result{
		Status: executor.StatusFailed,
		Reason: "Tests failed",
	}

	notification, err := notifier.Send(context.Background(), item, result)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if notification == nil {
		t.Fatal("Expected notification result, got nil")
	}

	if notification.Channel != "https://example.com/webhook" {
		t.Errorf("Expected webhook URL as channel, got '%s'", notification.Channel)
	}

	if len(client.requests) != 1 {
		t.Fatalf("Expected 1 request, got %d", len(client.requests))
	}

	req := client.requests[0]
	if req.Method != "POST" {
		t.Errorf("Expected POST request, got %s", req.Method)
	}

	if req.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Expected application/json content type, got %s", req.Header.Get("Content-Type"))
	}
}

func TestWebhookNotifier_Send_NonTransientError(t *testing.T) {
	client := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 400, body: `{"error": "bad request"}`}, // Non-transient error
		},
	}

	config := DefaultNotificationConfig()
	config.MaxRetries = 2

	notifier := NewWebhookNotifier("https://example.com/webhook", client, config)

	item := planner.WorkItem{Module: "example.com/module"}
	result := &executor.Result{Status: executor.StatusCompleted}

	notification, err := notifier.Send(context.Background(), item, result)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if notification != nil {
		t.Errorf("Expected nil notification on failure, got %+v", notification)
	}

	// Should not retry non-transient errors
	if len(client.requests) != 1 {
		t.Fatalf("Expected 1 request (no retries for 400), got %d", len(client.requests))
	}
}

func TestMultiNotifier_Send_PartialSuccess(t *testing.T) {
	successClient := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 200, body: `{"ok": true}`},
		},
	}

	failClient := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 500, body: `{"error": "server error"}`},
		},
	}

	config := DefaultNotificationConfig()
	config.MaxRetries = 0 // No retries to keep test simple

	slackNotifier := NewSlackNotifier("bot-token", "#channel", successClient, config)
	webhookNotifier := NewWebhookNotifier("https://example.com/webhook", failClient, config)

	multiNotifier := NewMultiNotifier(slackNotifier, webhookNotifier)

	item := planner.WorkItem{Module: "example.com/module"}
	result := &executor.Result{Status: executor.StatusCompleted}

	notification, err := multiNotifier.Send(context.Background(), item, result)

	// Should succeed overall because one notifier succeeded
	if err != nil {
		t.Fatalf("Expected no error for partial success, got: %v", err)
	}

	if notification == nil {
		t.Fatal("Expected notification result, got nil")
	}

	// Should return result from the first successful notifier
	if notification.Channel != "#channel" {
		t.Errorf("Expected first successful result, got channel '%s'", notification.Channel)
	}
}

func TestMultiNotifier_Send_AllFail(t *testing.T) {
	failClient1 := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 500, body: `{"error": "server error 1"}`},
		},
	}

	failClient2 := &mockHTTPClient{
		responses: []mockResponse{
			{statusCode: 500, body: `{"error": "server error 2"}`},
		},
	}

	config := DefaultNotificationConfig()
	config.MaxRetries = 0

	slackNotifier := NewSlackNotifier("bot-token", "#channel", failClient1, config)
	webhookNotifier := NewWebhookNotifier("https://example.com/webhook", failClient2, config)

	multiNotifier := NewMultiNotifier(slackNotifier, webhookNotifier)

	item := planner.WorkItem{Module: "example.com/module"}
	result := &executor.Result{Status: executor.StatusCompleted}

	notification, err := multiNotifier.Send(context.Background(), item, result)

	if err == nil {
		t.Fatal("Expected error when all notifiers fail")
	}

	var notifErr *NotificationError
	if !ErrorAs(err, &notifErr) {
		t.Fatalf("Expected NotificationError, got %T", err)
	}

	if notifErr.Channel != "multi" {
		t.Errorf("Expected 'multi' channel for multi-notifier error, got '%s'", notifErr.Channel)
	}

	if notification != nil {
		t.Errorf("Expected nil notification when all fail, got %+v", notification)
	}
}

func TestRenderNotification_Success(t *testing.T) {
	item := planner.WorkItem{
		Module:     "example.com/module",
		Repo:       "owner/repo",
		BranchName: "update-branch",
	}

	result := &executor.Result{
		Status:     executor.StatusCompleted,
		Reason:     "All tests passed successfully",
		CommitHash: "abc123def456ghi789",
	}

	message, err := RenderNotification("", item, result)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(message, "✅") {
		t.Error("Expected success emoji in notification")
	}

	if !strings.Contains(message, "example.com/module") {
		t.Error("Expected module name in notification")
	}

	if !strings.Contains(message, "owner/repo") {
		t.Error("Expected repository in notification")
	}

	if !strings.Contains(message, "update-branch") {
		t.Error("Expected branch name in notification")
	}

	if !strings.Contains(message, "abc12...") {
		t.Errorf("Expected truncated commit hash in notification. Message: %s", message)
	}
}

func TestRenderNotification_Failed(t *testing.T) {
	item := planner.WorkItem{
		Module: "example.com/module",
		Repo:   "owner/repo",
	}

	result := &executor.Result{
		Status: executor.StatusFailed,
		Reason: "Tests failed with multiple errors",
	}

	message, err := RenderNotification("", item, result)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(message, "❌") {
		t.Error("Expected failure emoji in notification")
	}

	if !strings.Contains(message, "failed") {
		t.Error("Expected status 'failed' in notification")
	}

	if !strings.Contains(message, "Tests failed with multiple") {
		t.Error("Expected truncated reason in notification")
	}
}

func TestRenderNotification_FailureDetails(t *testing.T) {
	item := planner.WorkItem{
		Module: "example.com/module",
		Repo:   "owner/repo",
	}

	failureOutput := `=== RUN   TestExample
--- FAIL: TestExample (0.00s)
    example_test.go:12: unexpected response code
FAIL
exit status 1
FAIL    example.com/app    0.123s
`

	cmd := manifest.Command{Cmd: []string{"go", "test", "./..."}}
	cmdErr := &executor.CommandExecutionError{
		Command:  cmd.Cmd,
		Dir:      "repo",
		Output:   failureOutput,
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	}

	result := &executor.Result{
		Status: executor.StatusFailed,
		Reason: "test execution failed",
		TestResults: []executor.CommandResult{
			{
				Command: cmd,
				Output:  failureOutput,
				Err:     cmdErr,
			},
		},
		DependencyImpact: &executor.DependencyImpact{
			Module:             "example.com/pkg",
			TargetVersion:      "v1.2.0",
			OldVersion:         "v1.1.0",
			OldVersionDetected: true,
			NewVersion:         "v1.2.0",
			NewVersionDetected: true,
			Applied:            true,
		},
	}

	message, err := RenderNotification("", item, result)
	if err != nil {
		t.Fatalf("RenderNotification error: %v", err)
	}

	if !strings.Contains(message, "**Failing Test**: "+escapeMarkdown("TestExample (example.com/app)")) {
		t.Fatalf("expected failing test summary in message, got:\n%s", message)
	}

	if !strings.Contains(message, "**Failure**: "+escapeMarkdown("example_test.go:12: unexpected response code")) {
		t.Fatalf("expected failure details in message, got:\n%s", message)
	}

	if !strings.Contains(message, "**Command**: go test ./...") {
		t.Fatalf("expected failing command in message, got:\n%s", message)
	}

	if !strings.Contains(message, "**Dependency**: "+escapeMarkdown("example.com/pkg -> v1.2.0 (was v1.1.0)")) {
		t.Fatalf("expected dependency summary in message, got:\n%s", message)
	}
}

func TestRenderNotification_CustomTemplate(t *testing.T) {
	item := planner.WorkItem{
		Module: "example.com/module",
	}

	result := &executor.Result{
		Status: executor.StatusCompleted,
	}

	customTemplate := "Module {{.Module}} has status {{.Status}}"
	message, err := RenderNotification(customTemplate, item, result)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "Module example.com/module has status completed"
	if message != expected {
		t.Errorf("Expected '%s', got '%s'", expected, message)
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"nil error", nil, false},
		{"timeout error", fmt.Errorf("connection timeout"), true},
		{"connection error", fmt.Errorf("connection refused"), true},
		{"temporary error", fmt.Errorf("temporary failure"), true},
		{"5xx error", fmt.Errorf("status 500"), true},
		{"4xx error", fmt.Errorf("status 400"), false},
		{"other error", fmt.Errorf("not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientError(tt.err); got != tt.transient {
				t.Errorf("isTransientError() = %v, want %v", got, tt.transient)
			}
		})
	}
}

func TestNotificationConfig_Defaults(t *testing.T) {
	config := DefaultNotificationConfig()

	if config.Template != defaultNotificationTemplate {
		t.Error("Expected default notification template")
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected 3 max retries, got %d", config.MaxRetries)
	}

	if config.RetryDelay != time.Second*2 {
		t.Errorf("Expected 2 second retry delay, got %v", config.RetryDelay)
	}

	if config.Timeout != time.Second*30 {
		t.Errorf("Expected 30 second timeout, got %v", config.Timeout)
	}
}

// ErrorAs is a helper function that mimics errors.As for testing
func ErrorAs(err error, target any) bool {
	switch e := err.(type) {
	case *NotificationError:
		if ptr, ok := target.(**NotificationError); ok {
			*ptr = e
			return true
		}
	}
	return false
}

func TestNoOpNotifier_Send(t *testing.T) {
	ctx := context.Background()
	notifier := NewNoOpNotifier()

	workItem := planner.WorkItem{
		Module: "example.com/testmod",
		Repo:   "owner/repo",
	}

	result := &executor.Result{
		Status: executor.StatusCompleted,
		Reason: "Tests passed",
	}

	notificationResult, err := notifier.Send(ctx, workItem, result)

	// Should not return an error
	if err != nil {
		t.Errorf("NoOpNotifier.Send() returned unexpected error: %v", err)
	}

	// Should return a successful result indicating notification was skipped
	if notificationResult == nil {
		t.Fatal("NoOpNotifier.Send() returned nil result")
	}

	if notificationResult.Channel != "noop" {
		t.Errorf("Expected channel 'noop', got '%s'", notificationResult.Channel)
	}

	expectedMessage := "Notification skipped (no integrations configured)"
	if notificationResult.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, notificationResult.Message)
	}
}

func TestNoOpNotifier_Send_WithNilResult(t *testing.T) {
	ctx := context.Background()
	notifier := NewNoOpNotifier()

	workItem := planner.WorkItem{
		Module: "example.com/testmod",
		Repo:   "owner/repo",
	}

	notificationResult, err := notifier.Send(ctx, workItem, nil)

	// Should still succeed even with nil result
	if err != nil {
		t.Errorf("NoOpNotifier.Send() returned unexpected error with nil result: %v", err)
	}

	if notificationResult == nil {
		t.Fatal("NoOpNotifier.Send() returned nil result")
	}

	if notificationResult.Channel != "noop" {
		t.Errorf("Expected channel 'noop', got '%s'", notificationResult.Channel)
	}
}
