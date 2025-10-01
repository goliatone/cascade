package di

import (
	"runtime"
	"time"

	"github.com/goliatone/cascade/internal/planner"
	"github.com/goliatone/cascade/pkg/config"
)

// providePlanner creates a default planner implementation.
// The planner computes cascade plans from manifests and targets.
func providePlanner() planner.Planner {
	return planner.New()
}

// providePlannerWithConfig creates a planner with configuration-driven dependency checking.
// When SkipUpToDate is enabled (and ForceAll is false), the planner checks if dependents
// already have the target dependency version and skips them if no update is needed.
func providePlannerWithConfig(cfg *config.Config, logger Logger) planner.Planner {
	if cfg == nil {
		logger.Warn("No configuration provided, using default planner")
		return planner.New()
	}

	opts := []planner.Option{}

	// Only enable dependency checking if SkipUpToDate is true and ForceAll is false
	if cfg.Executor.SkipUpToDate && !cfg.Executor.ForceAll {
		// Set default values if not configured
		strategy := cfg.Executor.CheckStrategy
		if strategy == "" {
			strategy = "auto"
		}

		cacheTTL := cfg.Executor.CheckCacheTTL
		if cacheTTL == 0 {
			cacheTTL = 5 * time.Minute
		}

		parallel := cfg.Executor.CheckParallel
		if parallel == 0 {
			parallel = runtime.NumCPU()
		}

		timeout := cfg.Executor.CheckTimeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		checkOpts := planner.CheckOptions{
			Strategy:       planner.CheckStrategy(strategy),
			CacheEnabled:   true,
			CacheTTL:       cacheTTL,
			ParallelChecks: parallel,
			Timeout:        timeout,
		}

		// Create checkers based on strategy
		var checker planner.DependencyChecker
		localChecker := planner.NewDependencyChecker(logger)
		remoteChecker := planner.NewRemoteDependencyChecker(checkOpts, logger)

		switch checkOpts.Strategy {
		case planner.CheckStrategyLocal:
			if cfg.Workspace.Path == "" {
				logger.Warn("Local strategy requested but workspace path not configured, using remote")
				checker = remoteChecker
			} else {
				logger.Debug("Using local dependency checking", "workspace", cfg.Workspace.Path)
				checker = localChecker
			}

		case planner.CheckStrategyRemote:
			logger.Debug("Using remote dependency checking",
				"cache_ttl", cacheTTL,
				"parallel", parallel,
				"timeout", timeout)
			checker = remoteChecker

		case planner.CheckStrategyAuto:
			logger.Debug("Using auto dependency checking (local with remote fallback)",
				"workspace", cfg.Workspace.Path,
				"cache_ttl", cacheTTL,
				"parallel", parallel,
				"timeout", timeout)
			checker = planner.NewHybridDependencyChecker(
				localChecker,
				remoteChecker,
				checkOpts.Strategy,
				cfg.Workspace.Path,
				logger,
			)

		default:
			logger.Warn("Unknown check strategy, using auto", "strategy", strategy)
			checker = planner.NewHybridDependencyChecker(
				localChecker,
				remoteChecker,
				planner.CheckStrategyAuto,
				cfg.Workspace.Path,
				logger,
			)
		}

		// Wrap in parallel checker if concurrency > 1
		if checkOpts.ParallelChecks > 1 {
			logger.Debug("Enabling parallel dependency checking", "concurrency", checkOpts.ParallelChecks)
			checker = planner.NewParallelDependencyChecker(
				checker,
				checkOpts.ParallelChecks,
				logger,
			)
		}

		opts = append(opts,
			planner.WithDependencyChecker(checker),
			planner.WithWorkspace(cfg.Workspace.Path))
	} else if cfg.Executor.ForceAll {
		logger.Debug("ForceAll enabled, processing all dependents without version checking")
	} else {
		logger.Debug("SkipUpToDate disabled, processing all dependents")
	}

	return planner.New(opts...)
}
