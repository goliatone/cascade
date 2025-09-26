package di

import (
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

type testLogger struct{}

func (testLogger) Debug(string, ...any) {}
func (testLogger) Info(string, ...any)  {}
func (testLogger) Warn(string, ...any)  {}
func (testLogger) Error(string, ...any) {}

func TestProviderFunctions(t *testing.T) {
	tests := []struct {
		name     string
		provider func() interface{}
		wantType string
	}{
		{
			name:     "provideManifest returns manifest.Loader",
			provider: func() interface{} { return provideManifest() },
			wantType: "*manifest.loader",
		},
		{
			name:     "providePlanner returns planner.Planner",
			provider: func() interface{} { return providePlanner() },
			wantType: "*planner.planner",
		},
		{
			name:     "provideExecutor returns executor.Executor",
			provider: func() interface{} { return provideExecutor() },
			wantType: "*executor.executor",
		},
		{
			name:     "provideBroker returns broker.Broker",
			provider: func() interface{} { return provideBroker() },
			wantType: "*broker.broker",
		},
		{
			name:     "provideState returns state.Manager",
			provider: func() interface{} { return provideState() },
			wantType: "*state.manager",
		},
		{
			name:     "provideConfig returns *config.Config",
			provider: func() interface{} { return provideConfig() },
			wantType: "*config.Config",
		},
		{
			name:     "provideLogger returns Logger",
			provider: func() interface{} { return provideLogger() },
			wantType: "*di.slogAdapter",
		},
		{
			name:     "provideHTTPClient returns *http.Client",
			provider: func() interface{} { return provideHTTPClient() },
			wantType: "*http.Client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.provider()

			if result == nil {
				t.Errorf("provider returned nil")
				return
			}

			// Verify the result implements the expected interface
			switch tt.name {
			case "provideManifest returns manifest.Loader":
				if _, ok := result.(manifest.Loader); !ok {
					t.Errorf("result does not implement manifest.Loader")
				}
			case "providePlanner returns planner.Planner":
				if _, ok := result.(planner.Planner); !ok {
					t.Errorf("result does not implement planner.Planner")
				}
			case "provideExecutor returns executor.Executor":
				if _, ok := result.(executor.Executor); !ok {
					t.Errorf("result does not implement executor.Executor")
				}
			case "provideBroker returns broker.Broker":
				if _, ok := result.(broker.Broker); !ok {
					t.Errorf("result does not implement broker.Broker")
				}
			case "provideState returns state.Manager":
				if _, ok := result.(state.Manager); !ok {
					t.Errorf("result does not implement state.Manager")
				}
			case "provideConfig returns *config.Config":
				if _, ok := result.(*config.Config); !ok {
					t.Errorf("result is not *config.Config")
				}
			case "provideLogger returns Logger":
				if _, ok := result.(Logger); !ok {
					t.Errorf("result does not implement Logger interface")
				}
			}
		})
	}
}

func TestProvideBrokerWithConfig_NoTokenFallsBackToStub(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		logger := testLogger{}
		cfg := &config.Config{}
		b := provideBrokerWithConfig(cfg, &http.Client{}, logger)

		if !isStubBroker(b) {
			t.Fatalf("expected stub broker when no GitHub token is configured")
		}
	})
}

func TestProvideBrokerWithConfig_WithTokenCreatesProvider(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		logger := testLogger{}
		cfg := &config.Config{}
		cfg.Integration.GitHub.Token = "test-token"

		b := provideBrokerWithConfig(cfg, &http.Client{}, logger)

		if isStubBroker(b) {
			t.Fatalf("expected real broker when GitHub token present")
		}
	})
}

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

func isStubBroker(b broker.Broker) bool {
	if b == nil {
		return true
	}
	value := reflect.ValueOf(b)
	if value.Kind() == reflect.Interface {
		value = value.Elem()
	}
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return true
	}
	value = value.Elem()
	providerField := value.FieldByName("provider")
	return providerField.IsValid() && providerField.IsNil()
}

func TestProvideBrokerForProduction_NoToken(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		logger := testLogger{}
		cfg := &config.Config{}

		broker, err := provideBrokerForProduction(cfg, &http.Client{}, logger)

		if err == nil {
			t.Fatal("expected error when no GitHub token provided for production broker")
		}
		if broker != nil {
			t.Fatal("expected nil broker when production broker creation fails")
		}

		expectedMsg := "production commands require GitHub credentials"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("error message should mention production credentials requirement, got: %v", err)
		}
	})
}

func TestProvideBrokerForProduction_WithToken(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		logger := testLogger{}
		cfg := &config.Config{}
		cfg.Integration.GitHub.Token = "test-token"

		broker, err := provideBrokerForProduction(cfg, &http.Client{}, logger)

		if err != nil {
			t.Fatalf("expected no error when GitHub token is provided, got: %v", err)
		}
		if broker == nil {
			t.Fatal("expected non-nil broker when GitHub token is provided")
		}
		if isStubBroker(broker) {
			t.Fatal("expected real broker when GitHub token is provided")
		}
	})
}

func TestProvideBrokerForProduction_DryRunMode(t *testing.T) {
	withClearedGitHubEnv(t, func() {
		logger := testLogger{}
		cfg := &config.Config{}
		cfg.Executor.DryRun = true // Enable dry-run mode

		broker, err := provideBrokerForProduction(cfg, &http.Client{}, logger)

		if err != nil {
			t.Fatalf("expected no error in dry-run mode, got: %v", err)
		}
		if broker == nil {
			t.Fatal("expected non-nil broker in dry-run mode")
		}
		if !isStubBroker(broker) {
			t.Fatal("expected stub broker in dry-run mode")
		}
	})
}

func TestProvideBrokerForProduction_NilConfig(t *testing.T) {
	logger := testLogger{}

	broker, err := provideBrokerForProduction(nil, &http.Client{}, logger)

	if err == nil {
		t.Fatal("expected error when config is nil")
	}
	if broker != nil {
		t.Fatal("expected nil broker when config is nil")
	}

	expectedMsg := "configuration is required for production broker"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got: %v", expectedMsg, err)
	}
}

func TestSlogAdapter(t *testing.T) {
	logger := provideLogger()

	// Verify it implements the Logger interface
	if logger == nil {
		t.Fatal("provideLogger returned nil")
	}

	// Test that methods don't panic (basic smoke test)
	logger.Debug("test debug message", "key", "value")
	logger.Info("test info message", "key", "value")
	logger.Warn("test warn message", "key", "value")
	logger.Error("test error message", "key", "value")
}
