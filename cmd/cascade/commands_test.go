package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/goliatone/cascade/internal/broker"
	execpkg "github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

// Helper function for string containment checks
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Mock implementations for testing

type mockManifestLoader struct {
	loadFunc func(path string) (*manifest.Manifest, error)
}

func (m *mockManifestLoader) Load(path string) (*manifest.Manifest, error) {
	if m.loadFunc != nil {
		return m.loadFunc(path)
	}
	return &manifest.Manifest{}, nil
}

func (m *mockManifestLoader) Generate(workdir string) (*manifest.Manifest, error) {
	return nil, errors.New("not implemented")
}

type mockPlanner struct {
	planFunc func(ctx context.Context, m *manifest.Manifest, target planner.Target) (*planner.Plan, error)
}

func (m *mockPlanner) Plan(ctx context.Context, manifest *manifest.Manifest, target planner.Target) (*planner.Plan, error) {
	if m.planFunc != nil {
		return m.planFunc(ctx, manifest, target)
	}
	return &planner.Plan{Target: target, Items: []planner.WorkItem{}}, nil
}

type mockStateManager struct {
	loadSummaryFunc func(module, version string) (*state.Summary, error)
}

func (m *mockStateManager) LoadSummary(module, version string) (*state.Summary, error) {
	if m.loadSummaryFunc != nil {
		return m.loadSummaryFunc(module, version)
	}
	return nil, state.ErrNotFound
}

func (m *mockStateManager) SaveSummary(summary *state.Summary) error {
	return nil
}

func (m *mockStateManager) SaveItemState(module, version string, item state.ItemState) error {
	return nil
}

func (m *mockStateManager) LoadItemStates(module, version string) ([]state.ItemState, error) {
	return []state.ItemState{}, nil
}

type mockExecutor struct {
	applyFunc func(ctx context.Context, input execpkg.WorkItemContext) (*execpkg.Result, error)
}

func (m *mockExecutor) Apply(ctx context.Context, input execpkg.WorkItemContext) (*execpkg.Result, error) {
	if m != nil && m.applyFunc != nil {
		return m.applyFunc(ctx, input)
	}
	return &execpkg.Result{Status: execpkg.StatusCompleted, Reason: "mock executor"}, nil
}

type mockBroker struct {
	ensurePRFunc func(ctx context.Context, item planner.WorkItem, result *execpkg.Result) (*broker.PullRequest, error)
	commentFunc  func(ctx context.Context, pr *broker.PullRequest, body string) error
	notifyFunc   func(ctx context.Context, item planner.WorkItem, result *execpkg.Result) (*broker.NotificationResult, error)
}

func (m *mockBroker) EnsurePR(ctx context.Context, item planner.WorkItem, result *execpkg.Result) (*broker.PullRequest, error) {
	if m != nil && m.ensurePRFunc != nil {
		return m.ensurePRFunc(ctx, item, result)
	}
	return &broker.PullRequest{Repo: item.Repo, URL: "https://example.com/pr/1"}, nil
}

func (m *mockBroker) Comment(ctx context.Context, pr *broker.PullRequest, body string) error {
	if m != nil && m.commentFunc != nil {
		return m.commentFunc(ctx, pr, body)
	}
	return nil
}

func (m *mockBroker) Notify(ctx context.Context, item planner.WorkItem, result *execpkg.Result) (*broker.NotificationResult, error) {
	if m != nil && m.notifyFunc != nil {
		return m.notifyFunc(ctx, item, result)
	}
	return nil, nil
}

type mockLogger struct {
	logs []string
}

func (m *mockLogger) Debug(msg string, args ...any) { m.logs = append(m.logs, msg) }
func (m *mockLogger) Info(msg string, args ...any)  { m.logs = append(m.logs, msg) }
func (m *mockLogger) Warn(msg string, args ...any)  { m.logs = append(m.logs, msg) }
func (m *mockLogger) Error(msg string, args ...any) { m.logs = append(m.logs, msg) }

// TestRunPlanWithMockDependencies tests the plan command logic with injected mocks
func TestRunPlanWithMockDependencies(t *testing.T) {
	tests := []struct {
		name           string
		manifestPath   string
		config         *config.Config
		manifestLoader *mockManifestLoader
		planner        *mockPlanner
		expectError    bool
		errorType      string
	}{
		{
			name:         "successful plan",
			manifestPath: "test.yaml",
			config: &config.Config{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
			},
			manifestLoader: &mockManifestLoader{
				loadFunc: func(path string) (*manifest.Manifest, error) {
					return &manifest.Manifest{}, nil
				},
			},
			planner: &mockPlanner{
				planFunc: func(ctx context.Context, m *manifest.Manifest, target planner.Target) (*planner.Plan, error) {
					return &planner.Plan{
						Target: target,
						Items:  []planner.WorkItem{},
					}, nil
				},
			},
			expectError: false,
		},
		{
			name:         "missing module",
			manifestPath: "test.yaml",
			config:       &config.Config{}, // No module specified
			manifestLoader: &mockManifestLoader{
				loadFunc: func(path string) (*manifest.Manifest, error) {
					return &manifest.Manifest{}, nil
				},
			},
			expectError: true,
			errorType:   "validation",
		},
		{
			name:         "manifest load error",
			manifestPath: "nonexistent.yaml",
			config: &config.Config{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
			},
			manifestLoader: &mockManifestLoader{
				loadFunc: func(path string) (*manifest.Manifest, error) {
					return nil, errors.New("file not found")
				},
			},
			expectError: true,
			errorType:   "file",
		},
		{
			name:         "planning error",
			manifestPath: "test.yaml",
			config: &config.Config{
				Module:  "github.com/example/lib",
				Version: "v1.2.3",
			},
			manifestLoader: &mockManifestLoader{
				loadFunc: func(path string) (*manifest.Manifest, error) {
					return &manifest.Manifest{}, nil
				},
			},
			planner: &mockPlanner{
				planFunc: func(ctx context.Context, m *manifest.Manifest, target planner.Target) (*planner.Plan, error) {
					return nil, errors.New("planning failed")
				},
			},
			expectError: true,
			errorType:   "planning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock container
			logger := &mockLogger{}
			mockContainer, err := di.New(
				di.WithConfig(tt.config),
				di.WithLogger(logger),
				di.WithManifestLoader(tt.manifestLoader),
				di.WithPlanner(tt.planner),
			)
			if err != nil {
				t.Fatalf("failed to create mock container: %v", err)
			}

			// Set the global container for the test
			originalContainer := container
			container = mockContainer
			defer func() { container = originalContainer }()

			// Call the function under test
			err = runPlan(tt.manifestPath)

			// Check results
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			// Check error type if specified
			if tt.expectError && tt.errorType != "" {
				// The error might be wrapped, so check the error message instead
				errorMsg := err.Error()
				switch tt.errorType {
				case "validation":
					if !contains(errorMsg, "target module must be specified") {
						t.Errorf("expected validation error message, got: %s", errorMsg)
					}
				case "file":
					if !contains(errorMsg, "failed to load manifest") {
						t.Errorf("expected file error message, got: %s", errorMsg)
					}
				case "planning":
					if !contains(errorMsg, "failed to generate plan") {
						t.Errorf("expected planning error message, got: %s", errorMsg)
					}
				}
			}

			// Verify logging occurred
			if len(logger.logs) == 0 {
				t.Error("expected some log messages but got none")
			}
		})
	}
}

// TestRunResumeWithMockDependencies tests the resume command logic with injected mocks
func TestRunResumeWithMockDependencies(t *testing.T) {
	tests := []struct {
		name           string
		stateID        string
		config         *config.Config
		stateManager   *mockStateManager
		manifestLoader *mockManifestLoader
		planner        *mockPlanner
		executor       execpkg.Executor
		broker         broker.Broker
		expectError    bool
		errorType      string
	}{
		{
			name:    "successful resume",
			stateID: "github.com/example/lib@v1.2.3",
			config: &config.Config{
				Executor: config.ExecutorConfig{DryRun: true},
			},
			stateManager: &mockStateManager{
				loadSummaryFunc: func(module, version string) (*state.Summary, error) {
					return &state.Summary{
						Module:  module,
						Version: version,
					}, nil
				},
			},
			manifestLoader: &mockManifestLoader{
				loadFunc: func(path string) (*manifest.Manifest, error) {
					return &manifest.Manifest{}, nil
				},
			},
			planner: &mockPlanner{
				planFunc: func(ctx context.Context, m *manifest.Manifest, target planner.Target) (*planner.Plan, error) {
					return &planner.Plan{Target: target, Items: []planner.WorkItem{}}, nil
				},
			},
			expectError: false,
		},
		{
			name:         "invalid state ID format",
			stateID:      "invalid-format",
			config:       &config.Config{},
			stateManager: &mockStateManager{},
			expectError:  true,
			errorType:    "validation",
		},
		{
			name:    "state not found",
			stateID: "github.com/example/lib@v1.2.3",
			config:  &config.Config{},
			stateManager: &mockStateManager{
				loadSummaryFunc: func(module, version string) (*state.Summary, error) {
					return nil, state.ErrNotFound
				},
			},
			expectError: true,
			errorType:   "state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock container
			logger := &mockLogger{}
			cfg := tt.config
			if cfg == nil {
				cfg = &config.Config{}
			}
			opts := []di.Option{
				di.WithConfig(cfg),
				di.WithLogger(logger),
			}
			if tt.stateManager != nil {
				opts = append(opts, di.WithStateManager(tt.stateManager))
			}
			if tt.manifestLoader != nil {
				opts = append(opts, di.WithManifestLoader(tt.manifestLoader))
			}
			if tt.planner != nil {
				opts = append(opts, di.WithPlanner(tt.planner))
			}
			if tt.executor != nil {
				opts = append(opts, di.WithExecutor(tt.executor))
			}
			if tt.broker != nil {
				opts = append(opts, di.WithBroker(tt.broker))
			}
			mockContainer, err := di.New(opts...)
			if err != nil {
				t.Fatalf("failed to create mock container: %v", err)
			}

			// Set the global container for the test
			originalContainer := container
			container = mockContainer
			defer func() { container = originalContainer }()

			// Call the function under test
			err = runResume(tt.stateID)

			// Check results
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}

			// Check error type if specified
			if tt.expectError && tt.errorType != "" {
				errorMsg := err.Error()
				switch tt.errorType {
				case "validation":
					if !contains(errorMsg, "module@version format") {
						t.Errorf("expected validation error message, got: %s", errorMsg)
					}
				case "state":
					if !contains(errorMsg, "no saved state found") {
						t.Errorf("expected state error message, got: %s", errorMsg)
					}
				}
			}
		})
	}
}

func TestIsProductionCommand(t *testing.T) {
	tests := []struct {
		name         string
		commandName  string
		isProduction bool
	}{
		{"plan command", "plan", false},
		{"release command", "release", true},
		{"resume command", "resume", true},
		{"revert command", "revert", true},
		{"unknown command", "unknown", false},
		{"nil command", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd *cobra.Command
			if tt.commandName != "" {
				root := newRootCommand()
				// Find the subcommand
				for _, subcmd := range root.Commands() {
					if subcmd.Name() == tt.commandName {
						cmd = subcmd
						break
					}
				}
			}

			result := isProductionCommand(cmd)
			if result != tt.isProduction {
				t.Errorf("isProductionCommand(%s) = %v, want %v", tt.commandName, result, tt.isProduction)
			}
		})
	}
}

func TestProductionCommandsFailWithoutCredentials(t *testing.T) {
	// Clear any GitHub environment variables that might interfere
	withClearedGitHubEnv(t, func() {
		productionCommands := []string{"release", "resume", "revert"}

		for _, commandName := range productionCommands {
			t.Run(commandName+" without credentials", func(t *testing.T) {
				// Create a root command and find the specific subcommand
				root := newRootCommand()
				var cmd *cobra.Command
				for _, subcmd := range root.Commands() {
					if subcmd.Name() == commandName {
						cmd = subcmd
						break
					}
				}

				if cmd == nil {
					t.Fatalf("command %s not found", commandName)
				}

				// Test container creation directly since CLI flag parsing is complex
				cfg := &config.Config{
					Module:  "github.com/example/test",
					Version: "v1.0.0",
				}

				// This should fail for production commands without GitHub credentials
				_, err := di.New(
					di.WithConfig(cfg),
					di.WithProductionCredentials(),
				)

				// Should get a configuration error about missing GitHub credentials
				if err == nil {
					t.Fatalf("expected error for %s command without GitHub credentials", commandName)
				}

				errorMsg := err.Error()
				if !contains(errorMsg, "production commands require GitHub credentials") {
					t.Errorf("expected error about production credentials requirement, got: %s", errorMsg)
				}

				// Verify it mentions the mitigation steps
				if !contains(errorMsg, "CASCADE_GITHUB_TOKEN") || !contains(errorMsg, "--dry-run") {
					t.Errorf("expected error to mention mitigation options (env var or dry-run), got: %s", errorMsg)
				}
			})
		}
	})
}

func TestPlanCommandWorksWithoutCredentials(t *testing.T) {
	// Clear any GitHub environment variables that might interfere
	withClearedGitHubEnv(t, func() {
		// Test container creation for plan command (should not require production credentials)
		cfg := &config.Config{
			Module:  "github.com/example/test",
			Version: "v1.0.0",
		}

		// Plan command should not use WithProductionCredentials, so this should succeed
		_, err := di.New(di.WithConfig(cfg))

		// Should succeed or fail for reasons other than missing credentials
		if err != nil {
			errorMsg := err.Error()
			// Should not fail due to missing production credentials
			if contains(errorMsg, "production commands require GitHub credentials") {
				t.Errorf("plan command should not require GitHub credentials, got: %s", errorMsg)
			}
			// Other errors (like config defaults loading issues) are acceptable for this test
		}
	})
}

func TestDryRunModeWorksWithoutCredentials(t *testing.T) {
	// Clear any GitHub environment variables that might interfere
	withClearedGitHubEnv(t, func() {
		// Test container creation with dry-run enabled (should work without GitHub credentials)
		cfg := &config.Config{
			Module:  "github.com/example/test",
			Version: "v1.0.0",
			Executor: config.ExecutorConfig{
				DryRun: true, // Enable dry-run mode
			},
		}

		// Even with production credentials requirement, dry-run should work without GitHub token
		_, err := di.New(
			di.WithConfig(cfg),
			di.WithProductionCredentials(),
		)

		// Should succeed or fail for reasons other than missing credentials
		if err != nil {
			errorMsg := err.Error()
			// Should not fail due to missing production credentials in dry-run mode
			if contains(errorMsg, "production commands require GitHub credentials") {
				t.Errorf("production command in dry-run mode should not require GitHub credentials, got: %s", errorMsg)
			}
			// Other errors (like config defaults loading issues) are acceptable for this test
		}
	})
}

// Helper function to clear GitHub environment variables for testing
func withClearedGitHubEnv(t *testing.T, fn func()) {
	t.Helper()
	vars := []string{"GITHUB_TOKEN", "GITHUB_ACCESS_TOKEN", "GH_TOKEN", "CASCADE_GITHUB_TOKEN"}
	original := make(map[string]string, len(vars))
	for _, v := range vars {
		original[v] = os.Getenv(v)
		os.Unsetenv(v)
	}
	defer func() {
		for _, v := range vars {
			if val, ok := original[v]; ok {
				os.Setenv(v, val)
			}
		}
	}()

	fn()
}
