package di

import (
	"os"
	"path/filepath"

	"github.com/goliatone/cascade/internal/state"
	"github.com/goliatone/cascade/pkg/config"
)

// provideState creates a default state manager implementation.
// Uses nop implementations for storage and locking, suitable for basic operation.
func provideState() state.Manager {
	return state.NewManager()
}

// provideStateWithConfig creates a state manager with filesystem storage and locking.
// Uses configuration to determine storage directory and other state settings.
// State persistence is enabled by default unless explicitly disabled by the user.
func provideStateWithConfig(cfg *config.Config, logger Logger) state.Manager {
	if cfg == nil {
		logger.Warn("No configuration provided, using nop state manager")
		return state.NewManager()
	}

	// Apply defaults for state configuration if not explicitly set by user.
	// This ensures state persistence is enabled by default as documented.
	stateDir := cfg.State.Dir
	if stateDir == "" {
		stateDir = getDefaultStateDir()
	}

	// Only disable state if user explicitly disabled it.
	// If Enabled is false but wasn't explicitly set, enable it (default behavior).
	explicitlyDisabled := cfg.State.Enabled == false && cfg.ExplicitlySetStateEnabled()
	if explicitlyDisabled {
		logger.Info("State persistence explicitly disabled, using nop state manager")
		return state.NewManager()
	}

	// Create filesystem storage
	stateStorage, err := state.NewFilesystemStorage(stateDir, logger)
	if err != nil {
		logger.Error("Failed to create filesystem storage, using nop state manager", "error", err)
		return state.NewManager()
	}

	// Create filesystem locker
	stateLocker := state.NewFilesystemLocker(stateDir, logger)

	logger.Debug("State persistence enabled", "dir", stateDir)

	return state.NewManager(
		state.WithStorage(stateStorage),
		state.WithLocker(stateLocker),
		state.WithLogger(logger),
	)
}

// getDefaultStateDir returns the default state directory following XDG Base Directory spec.
func getDefaultStateDir() string {
	// Follow XDG Base Directory specification
	if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
		return filepath.Join(xdgState, "cascade")
	}

	// Fallback to ~/.local/state/cascade
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "state", "cascade")
	}

	// Last resort fallback
	return filepath.Join(os.TempDir(), "cascade-state")
}
