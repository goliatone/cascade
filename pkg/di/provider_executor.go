package di

import (
	"github.com/goliatone/cascade/internal/executor"
	"github.com/goliatone/cascade/pkg/config"
)

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
