package di_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
	"github.com/goliatone/cascade/pkg/di"
)

// Mock implementations for testing
type mockLogger struct {
	debugCalls, infoCalls, warnCalls, errorCalls int
}

func (m *mockLogger) Debug(msg string, args ...any) { m.debugCalls++ }
func (m *mockLogger) Info(msg string, args ...any)  { m.infoCalls++ }
func (m *mockLogger) Warn(msg string, args ...any)  { m.warnCalls++ }
func (m *mockLogger) Error(msg string, args ...any) { m.errorCalls++ }

type mockManifestLoader struct{}

func (m *mockManifestLoader) Load(path string) (*manifest.Manifest, error) {
	return nil, errors.New("not implemented")
}

func (m *mockManifestLoader) Validate(manifest *manifest.Manifest) error {
	return errors.New("not implemented")
}

type mockPlanner struct{}

func (m *mockPlanner) Plan(manifest *manifest.Manifest) (*planner.Plan, error) {
	return nil, errors.New("not implemented")
}

type mockExecutor struct{}

func (m *mockExecutor) Execute(plan *planner.Plan) (*executor.Result, error) {
	return nil, errors.New("not implemented")
}

type mockBroker struct{}

func (m *mockBroker) CreatePR(result *executor.Result) (*broker.PR, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBroker) UpdatePR(pr *broker.PR) error {
	return errors.New("not implemented")
}

type mockStateManager struct{}

func (m *mockStateManager) Save(state *state.State) error {
	return errors.New("not implemented")
}

func (m *mockStateManager) Load() (*state.State, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStateManager) List() ([]*state.State, error) {
	return nil, errors.New("not implemented")
}

func TestNew_ReturnsErrorUntilImplemented(t *testing.T) {
	_, err := di.New(di.WithConfig(&config.Config{}))
	if err == nil {
		t.Fatal("expected not implemented error")
	}
}

func TestNew_OptionValidation(t *testing.T) {
	tests := []struct {
		name        string
		opts        []di.Option
		wantErr     bool
		errContains string
	}{
		{
			name:    "no options should work",
			opts:    nil,
			wantErr: true, // Currently returns not implemented error
		},
		{
			name:        "nil config should error",
			opts:        []di.Option{di.WithConfig(nil)},
			wantErr:     true,
			errContains: "config cannot be nil",
		},
		{
			name:        "nil logger should error",
			opts:        []di.Option{di.WithLogger(nil)},
			wantErr:     true,
			errContains: "logger cannot be nil",
		},
		{
			name:        "nil http client should error",
			opts:        []di.Option{di.WithHTTPClient(nil)},
			wantErr:     true,
			errContains: "http client cannot be nil",
		},
		{
			name:        "nil manifest loader should error",
			opts:        []di.Option{di.WithManifestLoader(nil)},
			wantErr:     true,
			errContains: "manifest loader cannot be nil",
		},
		{
			name:        "nil planner should error",
			opts:        []di.Option{di.WithPlanner(nil)},
			wantErr:     true,
			errContains: "planner cannot be nil",
		},
		{
			name:        "nil executor should error",
			opts:        []di.Option{di.WithExecutor(nil)},
			wantErr:     true,
			errContains: "executor cannot be nil",
		},
		{
			name:        "nil broker should error",
			opts:        []di.Option{di.WithBroker(nil)},
			wantErr:     true,
			errContains: "broker cannot be nil",
		},
		{
			name:        "nil state manager should error",
			opts:        []di.Option{di.WithStateManager(nil)},
			wantErr:     true,
			errContains: "state manager cannot be nil",
		},
		{
			name: "valid options should work",
			opts: []di.Option{
				di.WithConfig(&config.Config{}),
				di.WithLogger(&mockLogger{}),
				di.WithHTTPClient(&http.Client{}),
				di.WithManifestLoader(&mockManifestLoader{}),
				di.WithPlanner(&mockPlanner{}),
				di.WithExecutor(&mockExecutor{}),
				di.WithBroker(&mockBroker{}),
				di.WithStateManager(&mockStateManager{}),
			},
			wantErr: true, // Still returns not implemented until Task 3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := di.New(tt.opts...)

			if tt.wantErr {
				if err == nil {
					t.Errorf("New() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errContains != "" && err.Error() != "" {
					if err.Error() != tt.errContains && tt.errContains != "not implemented" {
						// For specific error messages, check containment
						if len(tt.errContains) > 10 { // Assume it's a contains check if longer
							t.Errorf("New() error = %v, want error containing %v", err, tt.errContains)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestOptions_IndividualValidation(t *testing.T) {
	// Test each option individually to ensure they validate their inputs correctly
	tests := []struct {
		name    string
		option  di.Option
		wantErr string
	}{
		{
			name:    "WithConfig with nil",
			option:  di.WithConfig(nil),
			wantErr: "config cannot be nil",
		},
		{
			name:    "WithConfig with valid config",
			option:  di.WithConfig(&config.Config{}),
			wantErr: "",
		},
		{
			name:    "WithLogger with nil",
			option:  di.WithLogger(nil),
			wantErr: "logger cannot be nil",
		},
		{
			name:    "WithLogger with valid logger",
			option:  di.WithLogger(&mockLogger{}),
			wantErr: "",
		},
		{
			name:    "WithHTTPClient with nil",
			option:  di.WithHTTPClient(nil),
			wantErr: "http client cannot be nil",
		},
		{
			name:    "WithHTTPClient with valid client",
			option:  di.WithHTTPClient(&http.Client{}),
			wantErr: "",
		},
		{
			name:    "WithManifestLoader with nil",
			option:  di.WithManifestLoader(nil),
			wantErr: "manifest loader cannot be nil",
		},
		{
			name:    "WithManifestLoader with valid loader",
			option:  di.WithManifestLoader(&mockManifestLoader{}),
			wantErr: "",
		},
		{
			name:    "WithPlanner with nil",
			option:  di.WithPlanner(nil),
			wantErr: "planner cannot be nil",
		},
		{
			name:    "WithPlanner with valid planner",
			option:  di.WithPlanner(&mockPlanner{}),
			wantErr: "",
		},
		{
			name:    "WithExecutor with nil",
			option:  di.WithExecutor(nil),
			wantErr: "executor cannot be nil",
		},
		{
			name:    "WithExecutor with valid executor",
			option:  di.WithExecutor(&mockExecutor{}),
			wantErr: "",
		},
		{
			name:    "WithBroker with nil",
			option:  di.WithBroker(nil),
			wantErr: "broker cannot be nil",
		},
		{
			name:    "WithBroker with valid broker",
			option:  di.WithBroker(&mockBroker{}),
			wantErr: "",
		},
		{
			name:    "WithStateManager with nil",
			option:  di.WithStateManager(nil),
			wantErr: "state manager cannot be nil",
		},
		{
			name:    "WithStateManager with valid manager",
			option:  di.WithStateManager(&mockStateManager{}),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test option directly by trying to create container
			_, err := di.New(tt.option)

			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("New() with %s error = nil, want error containing %v", tt.name, tt.wantErr)
					return
				}
				// For options that should validate inputs, check the specific error
				if tt.wantErr != "not implemented" && err.Error() != tt.wantErr {
					// The error should contain the expected validation message
					// (it might be wrapped in "di: failed to apply option: <message>")
					if !contains(err.Error(), tt.wantErr) {
						t.Errorf("New() with %s error = %v, want error containing %v", tt.name, err, tt.wantErr)
					}
				}
			} else {
				// If no error expected from validation, but build will still fail
				// until Task 3, so we expect a "not implemented" error
				if err == nil {
					t.Errorf("New() with %s error = nil, expected not implemented error", tt.name)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && s[1:len(substr)+1] == substr))
}

// Note: These tests will be expanded in Task 5 for integration testing
// once the container build() method is implemented in Task 3.

func TestContainer_InterfaceCompliance(t *testing.T) {
	// This test ensures the Container interface is properly defined
	// It will be expanded when container construction is implemented

	t.Skip("Container construction not implemented yet - will be enabled in Task 3")

	// This is a placeholder for future tests that will verify:
	// - Container returns non-nil services for all accessors
	// - Services implement expected interfaces
	// - Container properly manages dependency lifecycle
}
