package planner

import (
	"context"
	"fmt"
	"os"

	"github.com/goliatone/cascade/internal/manifest"
)

// hybridDependencyChecker intelligently selects between local and remote dependency checking
// based on the configured strategy. It supports three modes:
// - local: Use workspace-based checking only
// - remote: Use remote git operations only
// - auto: Try local first, fallback to remote on error
type hybridDependencyChecker struct {
	localChecker  DependencyChecker
	remoteChecker RemoteDependencyChecker
	strategy      CheckStrategy
	workspace     string
	logger        Logger
}

// NewHybridDependencyChecker creates a new hybrid dependency checker that intelligently
// selects between local and remote checking based on strategy configuration.
func NewHybridDependencyChecker(
	localChecker DependencyChecker,
	remoteChecker RemoteDependencyChecker,
	strategy CheckStrategy,
	workspace string,
	logger Logger,
) DependencyChecker {
	return &hybridDependencyChecker{
		localChecker:  localChecker,
		remoteChecker: remoteChecker,
		strategy:      strategy,
		workspace:     workspace,
		logger:        logger,
	}
}

// NeedsUpdate determines if a dependent repository needs an update to the target version.
// The implementation behavior depends on the configured strategy:
// - CheckStrategyLocal: Uses workspace-based checking only
// - CheckStrategyRemote: Uses remote git operations only
// - CheckStrategyAuto: Tries local first, falls back to remote on error
func (h *hybridDependencyChecker) NeedsUpdate(
	ctx context.Context,
	dependent manifest.Dependent,
	target Target,
	workspace string,
) (bool, error) {
	switch h.strategy {
	case CheckStrategyLocal:
		if h.logger != nil {
			h.logger.Debug("using local dependency checker",
				"repo", dependent.Repo,
				"strategy", "local")
		}
		return h.localChecker.NeedsUpdate(ctx, dependent, target, workspace)

	case CheckStrategyRemote:
		if h.logger != nil {
			h.logger.Debug("using remote dependency checker",
				"repo", dependent.Repo,
				"strategy", "remote")
		}
		return h.remoteChecker.NeedsUpdate(ctx, dependent, target, "")

	case CheckStrategyAuto:
		// Try local first, fallback to remote
		if h.logger != nil {
			h.logger.Debug("attempting local dependency check",
				"repo", dependent.Repo,
				"strategy", "auto")
		}

		needsUpdate, err := h.localChecker.NeedsUpdate(ctx, dependent, target, workspace)
		if err == nil {
			if h.logger != nil {
				h.logger.Debug("local check succeeded",
					"repo", dependent.Repo,
					"needs_update", needsUpdate)
			}
			return needsUpdate, nil
		}

		// Local check failed - fallback to remote
		if h.logger != nil {
			h.logger.Debug("local check failed, falling back to remote",
				"repo", dependent.Repo,
				"error", err.Error())
		}

		return h.remoteChecker.NeedsUpdate(ctx, dependent, target, "")

	default:
		return true, fmt.Errorf("unknown check strategy: %s", h.strategy)
	}
}

// detectCheckStrategy automatically detects the appropriate check strategy based on
// workspace availability. This is used when CheckStrategyAuto is configured.
//
// Logic:
// - If strategy is explicitly set (not auto), return it unchanged
// - If workspace exists and is accessible, prefer local checking
// - Otherwise, fallback to remote checking
func detectCheckStrategy(workspace string, cfg CheckOptions) CheckStrategy {
	// Respect explicit configuration
	if cfg.Strategy != CheckStrategyAuto && cfg.Strategy != "" {
		return cfg.Strategy
	}

	// Auto-detect based on workspace availability
	if workspace != "" {
		if info, err := os.Stat(workspace); err == nil && info.IsDir() {
			return CheckStrategyLocal
		}
	}

	// Default to remote when workspace unavailable
	return CheckStrategyRemote
}

// DetectCheckStrategy is a package-level function that exposes the strategy detection
// logic for external use (e.g., in CLI or DI container).
func DetectCheckStrategy(workspace string, cfg CheckOptions) CheckStrategy {
	return detectCheckStrategy(workspace, cfg)
}
