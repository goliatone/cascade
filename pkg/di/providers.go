package di

import (
	"log/slog"
	"net/http"
	"os"
	"time"

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

// provideExecutorWithConfig creates an executor with configuration-driven timeouts and settings.
// The executor implementation itself doesn't need config changes, but this documents the intent
// for future expansion when we add timeout configuration.
func provideExecutorWithConfig(cfg *config.Config, logger Logger) executor.Executor {
	if cfg == nil {
		logger.Warn("No configuration provided, using default executor")
		return executor.New()
	}

	// The current executor implementation doesn't take configuration,
	// but we can log the intended timeout settings
	if cfg.Executor.Timeout > 0 {
		logger.Debug("Executor configured with timeout", "timeout", cfg.Executor.Timeout)
	}
	if cfg.Executor.ConcurrentLimit > 0 {
		logger.Debug("Executor configured with concurrency limit", "limit", cfg.Executor.ConcurrentLimit)
	}

	return executor.New()
}

// provideBroker creates a default broker implementation.
// Uses stub implementations for provider and notifier since those require configuration.
func provideBroker() broker.Broker {
	return broker.NewStub()
}

// provideBrokerWithConfig creates a broker implementation configured from config.
// Returns a real broker with GitHub provider and Slack notifier if credentials are available,
// otherwise returns a stub broker for dry-run operations with clear warnings.
func provideBrokerWithConfig(cfg *config.Config, httpClient *http.Client, logger Logger) broker.Broker {
	if cfg == nil {
		logger.Warn("No configuration provided, using stub broker")
		return broker.NewStub()
	}

	if cfg.Executor.DryRun {
		logger.Info("Dry-run mode enabled, using stub broker")
		return broker.NewStub()
	}

	// TODO: Create real GitHub provider and Slack notifier implementations
	// For now, return stub until we implement the provider interfaces
	logger.Warn("Real broker implementation not yet available, using stub")
	return broker.NewStub()
}

// provideState creates a default state manager implementation.
// Uses nop implementations for storage and locking, suitable for basic operation.
func provideState() state.Manager {
	return state.NewManager()
}

// provideStateWithConfig creates a state manager with filesystem storage and locking.
// Uses configuration to determine storage directory and other state settings.
func provideStateWithConfig(cfg *config.Config, logger Logger) state.Manager {
	if cfg == nil {
		logger.Warn("No configuration provided, using nop state manager")
		return state.NewManager()
	}

	if !cfg.State.Enabled {
		logger.Info("State persistence disabled, using nop state manager")
		return state.NewManager()
	}

	// Create filesystem storage
	stateStorage, err := state.NewFilesystemStorage(cfg.State.Dir, logger)
	if err != nil {
		logger.Error("Failed to create filesystem storage, using nop state manager", "error", err)
		return state.NewManager()
	}

	// Create filesystem locker
	stateLocker := state.NewFilesystemLocker(cfg.State.Dir, logger)

	return state.NewManager(
		state.WithStorage(stateStorage),
		state.WithLocker(stateLocker),
		state.WithLogger(logger),
	)
}

// provideConfig creates a default configuration.
// Loads configuration from environment variables and defaults.
func provideConfig() *config.Config {
	cfg := config.New()
	// Configuration loading is handled by pkg/config
	return cfg
}

// provideConfigWithDefaults creates a configuration with defaults applied.
func provideConfigWithDefaults() (*config.Config, error) {
	return config.NewWithDefaults()
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

// provideLoggerWithConfig creates a logger configured from the logging config.
// Respects log level, format (text/json), verbose, and quiet settings.
func provideLoggerWithConfig(cfg *config.Config) Logger {
	if cfg == nil {
		return provideLogger()
	}

	// Determine log level from configuration
	var level slog.Level
	if cfg.Logging.Quiet {
		level = slog.LevelWarn
	} else if cfg.Logging.Verbose {
		level = slog.LevelDebug
	} else {
		switch cfg.Logging.Level {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		default:
			level = slog.LevelInfo
		}
	}

	// Create appropriate handler based on format
	var handler slog.Handler
	if cfg.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	return &slogAdapter{
		logger: slog.New(handler),
	}
}

// provideHTTPClient creates a default HTTP client implementation.
// Configured with reasonable defaults for API calls and timeouts.
func provideHTTPClient() *http.Client {
	return &http.Client{}
}

// provideHTTPClientWithConfig creates an HTTP client with configuration-driven timeouts.
// Respects executor timeout settings and sets appropriate user agent.
func provideHTTPClientWithConfig(cfg *config.Config) *http.Client {
	if cfg == nil {
		return provideHTTPClient()
	}

	// Use executor timeout as base for HTTP timeout, with reasonable default
	timeout := 30 * time.Second // Default timeout
	if cfg.Executor.Timeout > 0 {
		// Use 80% of executor timeout to leave buffer for retries
		timeout = time.Duration(float64(cfg.Executor.Timeout) * 0.8)
		if timeout < 10*time.Second {
			timeout = 10 * time.Second // Minimum timeout
		}
	}

	return &http.Client{
		Timeout: timeout,
		// TODO: Add user agent and other common headers
	}
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
