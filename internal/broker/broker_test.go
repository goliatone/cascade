package broker_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/testsupport"
)

// mockLogger implements broker.Logger for testing.
type mockLogger struct {
	debugCalls []logCall
	infoCalls  []logCall
	warnCalls  []logCall
	errorCalls []logCall
}

type logCall struct {
	msg  string
	args []any
}

func (m *mockLogger) Debug(msg string, args ...any) {
	m.debugCalls = append(m.debugCalls, logCall{msg, args})
}
func (m *mockLogger) Info(msg string, args ...any) {
	m.infoCalls = append(m.infoCalls, logCall{msg, args})
}
func (m *mockLogger) Warn(msg string, args ...any) {
	m.warnCalls = append(m.warnCalls, logCall{msg, args})
}
func (m *mockLogger) Error(msg string, args ...any) {
	m.errorCalls = append(m.errorCalls, logCall{msg, args})
}

func TestBroker_EnsurePRProducesExpectedPayload(t *testing.T) {
	loader := manifest.NewLoader()
	m, err := loader.Load(filepath.Join("..", "manifest", "testdata", "basic.yaml"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	plannerSvc := planner.New()
	plan, err := plannerSvc.Plan(context.Background(), m, planner.Target{Module: "github.com/goliatone/go-errors", Version: "v1.2.3"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	execResult := &executor.Result{Status: executor.StatusCompleted}

	// Use stub implementation for consistent golden file output
	b := broker.NewStub()
	pr, err := b.EnsurePR(context.Background(), plan.Items[0], execResult)
	if err != nil {
		t.Fatalf("EnsurePR: %v", err)
	}

	got, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Add trailing newline to match golden file format
	got = append(got, '\n')

	wantBytes, err := testsupport.LoadFixture(filepath.Join("testdata", "basic_pr.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}

	if string(got) != string(wantBytes) {
		t.Fatalf("pull request mismatch\n got: %s\nwant: %s", got, string(wantBytes))
	}
}

// mockProvider implements the Provider interface for testing
type mockProvider struct {
	createOrUpdatePR func(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error)
	addLabels        func(ctx context.Context, repo string, number int, labels []string) error
	requestReviewers func(ctx context.Context, repo string, number int, reviewers []string, teamReviewers []string) error
	listPullRequests func(ctx context.Context, repo string, headBranch string) ([]*broker.PullRequest, error)
	addComment       func(ctx context.Context, repo string, number int, body string) error
}

func (m *mockProvider) CreateOrUpdatePullRequest(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error) {
	if m.createOrUpdatePR != nil {
		return m.createOrUpdatePR(ctx, input)
	}
	return &broker.PullRequest{
		URL:    "https://github.com/" + input.Repo + "/pull/1",
		Number: 1,
		Repo:   input.Repo,
		Labels: input.Labels,
	}, nil
}

func (m *mockProvider) AddLabels(ctx context.Context, repo string, number int, labels []string) error {
	if m.addLabels != nil {
		return m.addLabels(ctx, repo, number, labels)
	}
	return nil
}

func (m *mockProvider) RequestReviewers(ctx context.Context, repo string, number int, reviewers []string, teamReviewers []string) error {
	if m.requestReviewers != nil {
		return m.requestReviewers(ctx, repo, number, reviewers, teamReviewers)
	}
	return nil
}

func (m *mockProvider) ListPullRequests(ctx context.Context, repo string, headBranch string) ([]*broker.PullRequest, error) {
	if m.listPullRequests != nil {
		return m.listPullRequests(ctx, repo, headBranch)
	}
	return []*broker.PullRequest{}, nil
}

func (m *mockProvider) AddComment(ctx context.Context, repo string, number int, body string) error {
	if m.addComment != nil {
		return m.addComment(ctx, repo, number, body)
	}
	return nil
}

// mockNotifier implements the Notifier interface for testing
type mockNotifier struct {
	send func(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error)
}

func (m *mockNotifier) Send(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
	if m.send != nil {
		return m.send(ctx, item, result)
	}
	return &broker.NotificationResult{
		Channel: "test-channel",
		Message: "test notification",
	}, nil
}

func TestBroker_EnsurePR_TableDriven(t *testing.T) {
	testWorkItem := planner.WorkItem{
		Repo:          "owner/repo",
		Module:        "github.com/test/module",
		Branch:        "main",
		BranchName:    "cascade-update-test",
		SourceVersion: "v1.0.0",
		Labels:        []string{"automation", "dependency-update"},
		PR: manifest.PRConfig{
			Reviewers:     []string{"reviewer1"},
			TeamReviewers: []string{"team1"},
		},
	}

	testExecutorResult := &executor.Result{
		Status: executor.StatusCompleted,
		Reason: "Successfully updated dependency",
	}

	tests := []struct {
		name           string
		workItem       planner.WorkItem
		result         *executor.Result
		config         broker.Config
		mockProvider   *mockProvider
		mockNotifier   *mockNotifier
		wantErr        bool
		expectedPRRepo string
		expectedLabels []string
	}{
		{
			name:     "successful PR creation",
			workItem: testWorkItem,
			result:   testExecutorResult,
			config:   broker.DefaultConfig(),
			mockProvider: &mockProvider{
				createOrUpdatePR: func(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error) {
					return &broker.PullRequest{
						URL:    "https://github.com/" + input.Repo + "/pull/123",
						Number: 123,
						Repo:   input.Repo,
						Labels: input.Labels,
					}, nil
				},
			},
			mockNotifier:   &mockNotifier{},
			wantErr:        false,
			expectedPRRepo: "owner/repo",
			expectedLabels: []string{"automation:cascade", "automation", "dependency-update"},
		},
		{
			name:     "dry run mode",
			workItem: testWorkItem,
			result:   testExecutorResult,
			config:   func() broker.Config { c := broker.DefaultConfig(); c.DryRun = true; return c }(),
			mockProvider: &mockProvider{
				createOrUpdatePR: func(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error) {
					t.Error("CreateOrUpdatePR should not be called in dry-run mode")
					return nil, errors.New("should not be called")
				},
			},
			mockNotifier:   &mockNotifier{},
			wantErr:        false,
			expectedPRRepo: "owner/repo",
			expectedLabels: []string{"automation:cascade", "automation", "dependency-update"},
		},
		{
			name:     "failed execution - skip PR creation",
			workItem: testWorkItem,
			result: &executor.Result{
				Status: executor.StatusFailed,
				Reason: "Failed to update dependency",
			},
			config: broker.DefaultConfig(),
			mockProvider: &mockProvider{
				createOrUpdatePR: func(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error) {
					t.Error("CreateOrUpdatePR should not be called for failed executions")
					return nil, errors.New("should not be called")
				},
			},
			mockNotifier: &mockNotifier{},
			wantErr:      false,
		},
		{
			name:     "provider error",
			workItem: testWorkItem,
			result:   testExecutorResult,
			config:   broker.DefaultConfig(),
			mockProvider: &mockProvider{
				createOrUpdatePR: func(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error) {
					return nil, errors.New("GitHub API error")
				},
			},
			mockNotifier: &mockNotifier{},
			wantErr:      true,
		},
		{
			name: "with custom labels",
			workItem: func() planner.WorkItem {
				w := testWorkItem
				w.Labels = []string{"custom-label", "priority:high"}
				return w
			}(),
			result: testExecutorResult,
			config: func() broker.Config {
				c := broker.DefaultConfig()
				c.DefaultLabels = []string{"auto", "cascade"}
				return c
			}(),
			mockProvider:   &mockProvider{},
			mockNotifier:   &mockNotifier{},
			wantErr:        false,
			expectedPRRepo: "owner/repo",
			expectedLabels: []string{"auto", "cascade", "custom-label", "priority:high"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker.New(tt.mockProvider, tt.mockNotifier, tt.config, &mockLogger{})

			pr, err := b.EnsurePR(context.Background(), tt.workItem, tt.result)

			if tt.wantErr && err == nil {
				t.Errorf("EnsurePR() error = nil, wantErr %v", tt.wantErr)
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("EnsurePR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// For failed executions, PR should be nil (graceful skip)
			if tt.result != nil && tt.result.Status == executor.StatusFailed {
				if pr != nil {
					t.Errorf("EnsurePR() for failed execution should return nil PR, got %v", pr)
				}
				return
			}

			if !tt.wantErr && pr != nil {
				if pr.Repo != tt.expectedPRRepo {
					t.Errorf("EnsurePR() repo = %v, want %v", pr.Repo, tt.expectedPRRepo)
				}

				if tt.expectedLabels != nil {
					if len(pr.Labels) != len(tt.expectedLabels) {
						t.Errorf("EnsurePR() labels count = %v, want %v", len(pr.Labels), len(tt.expectedLabels))
					}

					labelMap := make(map[string]bool)
					for _, label := range pr.Labels {
						labelMap[label] = true
					}
					for _, expectedLabel := range tt.expectedLabels {
						if !labelMap[expectedLabel] {
							t.Errorf("EnsurePR() missing expected label %v in %v", expectedLabel, pr.Labels)
						}
					}
				}
			}
		})
	}
}

func TestBroker_Comment(t *testing.T) {
	tests := []struct {
		name         string
		pr           *broker.PullRequest
		body         string
		config       broker.Config
		mockProvider *mockProvider
		wantErr      bool
	}{
		{
			name: "successful comment",
			pr: &broker.PullRequest{
				URL:    "https://github.com/owner/repo/pull/123",
				Number: 123,
				Repo:   "owner/repo",
			},
			body:         "Test comment body",
			config:       broker.DefaultConfig(),
			mockProvider: &mockProvider{},
			wantErr:      false,
		},
		{
			name:         "nil PR",
			pr:           nil,
			body:         "Test comment",
			config:       broker.DefaultConfig(),
			mockProvider: &mockProvider{},
			wantErr:      true,
		},
		{
			name: "empty body",
			pr: &broker.PullRequest{
				URL:    "https://github.com/owner/repo/pull/123",
				Number: 123,
				Repo:   "owner/repo",
			},
			body:         "",
			config:       broker.DefaultConfig(),
			mockProvider: &mockProvider{},
			wantErr:      true,
		},
		{
			name: "dry run mode",
			pr: &broker.PullRequest{
				URL:    "https://github.com/owner/repo/pull/123",
				Number: 123,
				Repo:   "owner/repo",
			},
			body:   "Test comment",
			config: func() broker.Config { c := broker.DefaultConfig(); c.DryRun = true; return c }(),
			mockProvider: &mockProvider{
				addComment: func(ctx context.Context, repo string, number int, body string) error {
					t.Error("AddComment should not be called in dry-run mode")
					return errors.New("should not be called")
				},
			},
			wantErr: false,
		},
		{
			name: "provider error",
			pr: &broker.PullRequest{
				URL:    "https://github.com/owner/repo/pull/123",
				Number: 123,
				Repo:   "owner/repo",
			},
			body:   "Test comment",
			config: broker.DefaultConfig(),
			mockProvider: &mockProvider{
				addComment: func(ctx context.Context, repo string, number int, body string) error {
					return errors.New("GitHub API error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker.New(tt.mockProvider, &mockNotifier{}, tt.config, &mockLogger{})

			err := b.Comment(context.Background(), tt.pr, tt.body)

			if tt.wantErr && err == nil {
				t.Errorf("Comment() error = nil, wantErr %v", tt.wantErr)
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Comment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBroker_Notify(t *testing.T) {
	testWorkItem := planner.WorkItem{
		Repo:   "owner/repo",
		Module: "github.com/test/module",
	}

	testResult := &executor.Result{
		Status: executor.StatusCompleted,
		Reason: "Success",
	}

	tests := []struct {
		name         string
		workItem     planner.WorkItem
		result       *executor.Result
		config       broker.Config
		mockNotifier *mockNotifier
		wantErr      bool
		expectResult *broker.NotificationResult
	}{
		{
			name:     "successful notification",
			workItem: testWorkItem,
			result:   testResult,
			config:   broker.DefaultConfig(),
			mockNotifier: &mockNotifier{
				send: func(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
					return &broker.NotificationResult{
						Channel: "test-channel",
						Message: "Test notification sent",
					}, nil
				},
			},
			wantErr: false,
			expectResult: &broker.NotificationResult{
				Channel: "test-channel",
				Message: "Test notification sent",
			},
		},
		{
			name:     "dry run mode",
			workItem: testWorkItem,
			result:   testResult,
			config:   func() broker.Config { c := broker.DefaultConfig(); c.DryRun = true; return c }(),
			mockNotifier: &mockNotifier{
				send: func(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
					t.Error("Send should not be called in dry-run mode")
					return nil, errors.New("should not be called")
				},
			},
			wantErr:      false,
			expectResult: nil,
		},
		{
			name:     "notification failure - should not error",
			workItem: testWorkItem,
			result:   testResult,
			config:   broker.DefaultConfig(),
			mockNotifier: &mockNotifier{
				send: func(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
					return nil, errors.New("notification service unavailable")
				},
			},
			wantErr:      false, // Notification failures should not block PR creation
			expectResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := broker.New(&mockProvider{}, tt.mockNotifier, tt.config, &mockLogger{})

			result, err := b.Notify(context.Background(), tt.workItem, tt.result)

			if tt.wantErr && err == nil {
				t.Errorf("Notify() error = nil, wantErr %v", tt.wantErr)
				return
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Notify() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.expectResult == nil && result != nil {
				t.Errorf("Notify() result = %v, want nil", result)
			}
			if tt.expectResult != nil && result == nil {
				t.Errorf("Notify() result = nil, want %v", tt.expectResult)
			}
			if tt.expectResult != nil && result != nil {
				if result.Channel != tt.expectResult.Channel {
					t.Errorf("Notify() result.Channel = %v, want %v", result.Channel, tt.expectResult.Channel)
				}
				if result.Message != tt.expectResult.Message {
					t.Errorf("Notify() result.Message = %v, want %v", result.Message, tt.expectResult.Message)
				}
			}
		})
	}
}

func TestBroker_Notify_WithNoOpNotifier(t *testing.T) {
	// Create a broker with NoOpNotifier
	b := broker.New(&mockProvider{}, broker.NewNoOpNotifier(), broker.DefaultConfig(), &mockLogger{})

	workItem := planner.WorkItem{
		Module: "example.com/testmod",
		Repo:   "owner/repo",
	}

	result := &executor.Result{
		Status: executor.StatusCompleted,
		Reason: "Tests passed",
	}

	notificationResult, err := b.Notify(context.Background(), workItem, result)

	// Should not return an error
	if err != nil {
		t.Errorf("Notify() with NoOpNotifier returned unexpected error: %v", err)
	}

	// Should return a successful result
	if notificationResult == nil {
		t.Fatal("Notify() with NoOpNotifier returned nil result")
	}

	if notificationResult.Channel != "noop" {
		t.Errorf("Expected channel 'noop', got '%s'", notificationResult.Channel)
	}

	expectedMessage := "Notification skipped (no integrations configured)"
	if notificationResult.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, notificationResult.Message)
	}
}

func TestBroker_StructuredLogging(t *testing.T) {
	tests := []struct {
		name       string
		scenario   string
		workItem   planner.WorkItem
		result     *executor.Result
		expectLogs func(t *testing.T, logger *mockLogger)
	}{
		{
			name:     "logs reviewer request failure",
			scenario: "reviewer_failure",
			workItem: planner.WorkItem{
				Module: "example.com/testmod",
				Repo:   "owner/repo",
				PR: manifest.PRConfig{
					Reviewers: []string{"reviewer1"},
				},
			},
			result: &executor.Result{Status: executor.StatusCompleted},
			expectLogs: func(t *testing.T, logger *mockLogger) {
				if len(logger.warnCalls) != 1 {
					t.Errorf("expected 1 warn call, got %d", len(logger.warnCalls))
					return
				}
				call := logger.warnCalls[0]
				if call.msg != "Failed to request reviewers" {
					t.Errorf("expected 'Failed to request reviewers', got %q", call.msg)
				}
				// Check that structured fields are present
				hasModule := false
				hasRepo := false
				for i := 0; i < len(call.args); i += 2 {
					if i+1 >= len(call.args) {
						break
					}
					key := call.args[i].(string)
					if key == "module" && call.args[i+1].(string) == "example.com/testmod" {
						hasModule = true
					}
					if key == "repo" && call.args[i+1].(string) == "owner/repo" {
						hasRepo = true
					}
				}
				if !hasModule {
					t.Error("expected module field in log args")
				}
				if !hasRepo {
					t.Error("expected repo field in log args")
				}
			},
		},
		{
			name:     "logs failed execution skip",
			scenario: "failed_execution",
			workItem: planner.WorkItem{
				Module: "example.com/testmod",
				Repo:   "owner/repo",
			},
			result: &executor.Result{
				Status: executor.StatusFailed,
				Reason: "build failed",
			},
			expectLogs: func(t *testing.T, logger *mockLogger) {
				if len(logger.infoCalls) != 1 {
					t.Errorf("expected 1 info call, got %d", len(logger.infoCalls))
					return
				}
				call := logger.infoCalls[0]
				if call.msg != "Skipping PR creation for failed execution" {
					t.Errorf("expected 'Skipping PR creation for failed execution', got %q", call.msg)
				}
			},
		},
		{
			name:     "logs noop notification",
			scenario: "noop_notification",
			workItem: planner.WorkItem{
				Module: "example.com/testmod",
				Repo:   "owner/repo",
			},
			result: &executor.Result{Status: executor.StatusCompleted},
			expectLogs: func(t *testing.T, logger *mockLogger) {
				if len(logger.infoCalls) != 1 {
					t.Errorf("expected 1 info call, got %d", len(logger.infoCalls))
					return
				}
				call := logger.infoCalls[0]
				if call.msg != "Notifications disabled" {
					t.Errorf("expected 'Notifications disabled', got %q", call.msg)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &mockLogger{}
			var mockProv *mockProvider
			var mockNotif broker.Notifier

			switch tt.scenario {
			case "reviewer_failure":
				mockProv = &mockProvider{
					requestReviewers: func(ctx context.Context, repo string, number int, reviewers []string, teamReviewers []string) error {
						return errors.New("reviewer request failed")
					},
					createOrUpdatePR: func(ctx context.Context, input broker.PRInput) (*broker.PullRequest, error) {
						return &broker.PullRequest{URL: "test", Number: 1, Repo: input.Repo}, nil
					},
				}
				mockNotif = &mockNotifier{
					send: func(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
						return nil, nil
					},
				}
			case "failed_execution":
				mockProv = &mockProvider{}
				mockNotif = &mockNotifier{}
			case "noop_notification":
				mockProv = &mockProvider{}
				mockNotif = broker.NewNoOpNotifier()
			}

			b := broker.New(mockProv, mockNotif, broker.DefaultConfig(), logger)

			switch tt.scenario {
			case "reviewer_failure", "failed_execution":
				b.EnsurePR(context.Background(), tt.workItem, tt.result)
			case "noop_notification":
				b.Notify(context.Background(), tt.workItem, tt.result)
			}

			tt.expectLogs(t, logger)
		})
	}
}
