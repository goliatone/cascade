package di

import (
	"log/slog"
	"os"

	"github.com/goliatone/cascade/pkg/config"
)

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
