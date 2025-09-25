package di

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/goliatone/cascade/internal/broker"
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/internal/manifest"
	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

// provideManifest creates a default manifest loader implementation.
// Uses the basic file-based loader that reads YAML manifests from disk.
func provideManifest() manifest.Loader {
	return manifest.NewLoader()
}

// providePlanner creates a default planner implementation.
// The planner computes cascade plans from manifests and targets.
func providePlanner() planner.Planner {
	return planner.New()
}

// provideExecutor creates a default executor implementation.
// The executor orchestrates git operations, dependency updates, and command execution.
func provideExecutor() executor.Executor {
	return executor.New()
}

// provideBroker creates a default broker implementation.
// Uses stub implementations for provider and notifier since those require configuration.
func provideBroker() broker.Broker {
	return broker.NewStub()
}

// provideState creates a default state manager implementation.
// Uses nop implementations for storage and locking, suitable for basic operation.
func provideState() state.Manager {
	return state.NewManager()
}

// provideConfig creates a default configuration.
// Loads configuration from environment variables and defaults.
func provideConfig() *config.Config {
	cfg := config.New()
	// Configuration loading is handled by pkg/config
	return cfg
}

// provideLogger creates a default structured logger implementation.
// Uses the standard library slog for structured logging output.
func provideLogger() Logger {
	return &slogAdapter{
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

// provideHTTPClient creates a default HTTP client implementation.
// Configured with reasonable defaults for API calls and timeouts.
func provideHTTPClient() *http.Client {
	return &http.Client{}
}

// slogAdapter adapts slog.Logger to implement our Logger interface.
type slogAdapter struct {
	logger *slog.Logger
}

func (s *slogAdapter) Debug(msg string, args ...any) {
	s.logger.Debug(msg, args...)
}

func (s *slogAdapter) Info(msg string, args ...any) {
	s.logger.Info(msg, args...)
}

func (s *slogAdapter) Warn(msg string, args ...any) {
	s.logger.Warn(msg, args...)
}

func (s *slogAdapter) Error(msg string, args ...any) {
	s.logger.Error(msg, args...)
}
