package di

import (
	"net/http"
	"os"
	"reflect"
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
