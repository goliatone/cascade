package main

import (
	"context"
	"errors"
	"strings"
	"testing"

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
		name         string
		stateID      string
		config       *config.Config
		stateManager *mockStateManager
		expectError  bool
		errorType    string
	}{
		{
			name:    "successful resume",
			stateID: "github.com/example/lib@v1.2.3",
			config:  &config.Config{},
			stateManager: &mockStateManager{
				loadSummaryFunc: func(module, version string) (*state.Summary, error) {
					return &state.Summary{
						Module:  module,
						Version: version,
					}, nil
				},
			},
			expectError: false,
		},
		{
			name:        "invalid state ID format",
			stateID:     "invalid-format",
			config:      &config.Config{},
			expectError: true,
			errorType:   "validation",
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
			mockContainer, err := di.New(
				di.WithConfig(tt.config),
				di.WithLogger(logger),
				di.WithStateManager(tt.stateManager),
			)
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
					if !contains(errorMsg, "invalid state ID format") {
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
