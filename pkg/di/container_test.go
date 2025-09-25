package di_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
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

func (m *mockManifestLoader) Generate(workdir string) (*manifest.Manifest, error) {
	return nil, errors.New("not implemented")
}

type mockPlanner struct{}

func (m *mockPlanner) Plan(ctx context.Context, manifest *manifest.Manifest, target planner.Target) (*planner.Plan, error) {
	return nil, errors.New("not implemented")
}

type mockExecutor struct{}

func (m *mockExecutor) Apply(ctx context.Context, input executor.WorkItemContext) (*executor.Result, error) {
	return nil, errors.New("not implemented")
}

type mockBroker struct{}

func (m *mockBroker) EnsurePR(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.PullRequest, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBroker) Comment(ctx context.Context, pr *broker.PullRequest, body string) error {
	return errors.New("not implemented")
}

func (m *mockBroker) Notify(ctx context.Context, item planner.WorkItem, result *executor.Result) (*broker.NotificationResult, error) {
	return nil, errors.New("not implemented")
}

type mockStateManager struct{}

func (m *mockStateManager) LoadSummary(module, version string) (*state.Summary, error) {
	return nil, errors.New("not implemented")
}

func (m *mockStateManager) SaveSummary(summary *state.Summary) error {
	return errors.New("not implemented")
}

func (m *mockStateManager) SaveItemState(module, version string, item state.ItemState) error {
	return errors.New("not implemented")
}

func (m *mockStateManager) LoadItemStates(module, version string) ([]state.ItemState, error) {
	return nil, errors.New("not implemented")
}

func TestNew_WithDefaults(t *testing.T) {
	container, err := di.New()
	if err != nil {
		t.Fatalf("expected successful container creation with defaults, got error: %v", err)
	}

	if container == nil {
		t.Fatal("expected non-nil container")
	}

	// Verify all services are available
	if container.Config() == nil {
		t.Error("expected non-nil Config")
	}
	if container.Logger() == nil {
		t.Error("expected non-nil Logger")
	}
	if container.HTTPClient() == nil {
		t.Error("expected non-nil HTTPClient")
	}
	if container.Manifest() == nil {
		t.Error("expected non-nil Manifest")
	}
	if container.Planner() == nil {
		t.Error("expected non-nil Planner")
	}
	if container.Executor() == nil {
		t.Error("expected non-nil Executor")
	}
	if container.Broker() == nil {
		t.Error("expected non-nil Broker")
	}
	if container.State() == nil {
		t.Error("expected non-nil State")
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
			wantErr: false, // Now works with default providers
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
			wantErr: false, // Now works with implemented container build
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
					if !strings.Contains(err.Error(), tt.errContains) {
						t.Errorf("New() error = %v, want error containing %v", err, tt.errContains)
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
					if !strings.Contains(err.Error(), tt.wantErr) {
						t.Errorf("New() with %s error = %v, want error containing %v", tt.name, err, tt.wantErr)
					}
				}
			} else {
				// If no error expected, container creation should succeed
				if err != nil {
					t.Errorf("New() with %s error = %v, expected success", tt.name, err)
				}
			}
		})
	}
}

// Test configuration-driven providers
func TestContainer_ConfigurationIntegration(t *testing.T) {
	tests := []struct {
		name   string
		config *config.Config
	}{
		{
			name:   "default config",
			config: nil, // Will use defaults
		},
		{
			name: "custom logging config",
			config: &config.Config{
				Logging: config.LoggingConfig{
					Level:   "debug",
					Format:  "json",
					Verbose: true,
				},
			},
		},
		{
			name: "state disabled",
			config: &config.Config{
				State: config.StateConfig{
					Enabled: false,
				},
			},
		},
		{
			name: "dry-run mode",
			config: &config.Config{
				Executor: config.ExecutorConfig{
					DryRun: true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []di.Option
			if tt.config != nil {
				opts = append(opts, di.WithConfig(tt.config))
			}

			container, err := di.New(opts...)
			if err != nil {
				t.Fatalf("Failed to create container with config: %v", err)
			}

			// Verify all services are properly wired
			if container.Config() == nil {
				t.Error("Config() returned nil")
			}
			if container.Logger() == nil {
				t.Error("Logger() returned nil")
			}
			if container.HTTPClient() == nil {
				t.Error("HTTPClient() returned nil")
			}
			if container.State() == nil {
				t.Error("State() returned nil")
			}
			if container.Broker() == nil {
				t.Error("Broker() returned nil")
			}
			if container.Executor() == nil {
				t.Error("Executor() returned nil")
			}

			// Test that config values are accessible
			cfg := container.Config()
			if tt.config != nil {
				if tt.config.Logging.Level != "" && cfg.Logging.Level != tt.config.Logging.Level {
					t.Errorf("Expected logging level %s, got %s", tt.config.Logging.Level, cfg.Logging.Level)
				}
				if cfg.State.Enabled != tt.config.State.Enabled {
					t.Errorf("Expected state enabled %v, got %v", tt.config.State.Enabled, cfg.State.Enabled)
				}
				if cfg.Executor.DryRun != tt.config.Executor.DryRun {
					t.Errorf("Expected dry-run %v, got %v", tt.config.Executor.DryRun, cfg.Executor.DryRun)
				}
			}
		})
	}
}

func TestContainer_InterfaceCompliance(t *testing.T) {
	// Test that container returns proper interfaces for all services
	container, err := di.New()
	if err != nil {
		t.Fatalf("Failed to create container: %v", err)
	}

	// Test that all accessors return non-nil services
	if container.Config() == nil {
		t.Error("Config() returned nil")
	}
	if container.Logger() == nil {
		t.Error("Logger() returned nil")
	}
	if container.HTTPClient() == nil {
		t.Error("HTTPClient() returned nil")
	}
	if container.Manifest() == nil {
		t.Error("Manifest() returned nil")
	}
	if container.Planner() == nil {
		t.Error("Planner() returned nil")
	}
	if container.Executor() == nil {
		t.Error("Executor() returned nil")
	}
	if container.Broker() == nil {
		t.Error("Broker() returned nil")
	}
	if container.State() == nil {
		t.Error("State() returned nil")
	}

	// Test that services implement expected interfaces
	var _ manifest.Loader = container.Manifest()
	var _ planner.Planner = container.Planner()
	var _ executor.Executor = container.Executor()
	var _ broker.Broker = container.Broker()
	var _ state.Manager = container.State()
	var _ di.Logger = container.Logger()
}
