package planner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
	"golang.org/x/mod/modfile"
)

// remoteDependencyChecker implements RemoteDependencyChecker by fetching
// go.mod files from remote repositories via shallow git clones.
type remoteDependencyChecker struct {
	cache   *dependencyCache
	gitOps  gitOperations
	logger  Logger
	options CheckOptions
}

// NewRemoteDependencyChecker creates a new remote dependency checker.
func NewRemoteDependencyChecker(opts CheckOptions, logger Logger) RemoteDependencyChecker {
	// Set default options
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 5 * 60 * 1000000000 // 5 minutes in nanoseconds
	}
	if opts.ParallelChecks == 0 {
		opts.ParallelChecks = 4 // Default concurrency
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * 1000000000 // 30 seconds in nanoseconds
	}

	checker := &remoteDependencyChecker{
		cache:   newDependencyCache(opts.CacheTTL),
		gitOps:  newGitOperations(opts.Timeout),
		logger:  logger,
		options: opts,
	}
	return checker
}

// NeedsUpdate determines if a dependent repository needs an update to the target version.
// It fetches the go.mod file from the remote repository (with caching) and compares versions.
//
// The implementation follows a cache-first strategy:
// 1. Check cache for existing dependency information
// 2. On cache miss, perform shallow clone to fetch go.mod
// 3. Parse go.mod and cache all dependencies
// 4. Compare versions to determine if update is needed
//
// Fail-open behavior: errors don't block planning, we assume update is needed.
func (r *remoteDependencyChecker) NeedsUpdate(
	ctx context.Context,
	dependent manifest.Dependent,
	target Target,
	workspace string, // Ignored in remote mode
) (bool, error) {
	// 1. Parse clone URL from dependent
	cloneURL, err := r.gitOps.parseCloneURL(dependent)
	if err != nil {
		if r.logger != nil {
			r.logger.Debug("failed to parse clone URL, assuming update needed",
				"repo", dependent.Repo,
				"error", err.Error())
		}
		return true, fmt.Errorf("parse clone URL: %w", err)
	}

	// 2. Determine ref (branch/tag)
	ref := dependent.Branch
	if ref == "" {
		ref = "main" // Default branch
	}

	// 3. Check cache first
	if r.options.CacheEnabled {
		currentVersion, cached := r.cache.Get(cloneURL, ref, target.Module)
		if cached {
			if r.logger != nil {
				r.logger.Debug("cache hit for dependency check",
					"repo", dependent.Repo,
					"module", target.Module,
					"cached_version", currentVersion)
			}

			// If dependency not present in go.mod, no update needed
			if currentVersion == "" {
				if r.logger != nil {
					r.logger.Info("dependency not present in go.mod (cached)",
						"repo", dependent.Repo,
						"module", target.Module)
				}
				return false, nil
			}

			// Compare versions
			needsUpdate, err := CompareVersions(currentVersion, target.Version)
			if err != nil {
				return true, fmt.Errorf("compare versions: %w", err)
			}

			if r.logger != nil {
				r.logger.Info("remote dependency check result (cached)",
					"repo", dependent.Repo,
					"module", target.Module,
					"current_version", currentVersion,
					"target_version", target.Version,
					"needs_update", needsUpdate)
			}

			return needsUpdate, nil
		}
	}

	// 4. Cache miss - fetch go.mod from remote
	if r.logger != nil {
		r.logger.Info("remote dependency check started",
			"repo", dependent.Repo,
			"clone_url", cloneURL,
			"ref", ref,
			"cached", false)
	}

	startTime := time.Now()
	goModContent, err := r.gitOps.fetchGoMod(ctx, cloneURL, ref)
	duration := time.Since(startTime)

	if err != nil {
		if r.logger != nil {
			r.logger.Debug("failed to fetch go.mod, assuming update needed",
				"repo", dependent.Repo,
				"clone_url", cloneURL,
				"ref", ref,
				"duration_ms", duration.Milliseconds(),
				"error", err.Error())
		}
		// Fail-open: if we can't fetch go.mod, assume update is needed
		return true, err
	}

	if r.logger != nil {
		r.logger.Info("shallow clone completed",
			"repo", dependent.Repo,
			"duration_ms", duration.Milliseconds(),
			"go_mod_size", len(goModContent))
	}

	// 5. Parse go.mod and cache all dependencies
	deps, err := parseGoModContentAndExtractDeps(goModContent)
	if err != nil {
		if r.logger != nil {
			r.logger.Debug("failed to parse go.mod, assuming update needed",
				"repo", dependent.Repo,
				"error", err.Error())
		}
		return true, fmt.Errorf("parse go.mod: %w", err)
	}

	// Cache the dependencies for future lookups
	if r.options.CacheEnabled {
		r.cache.Set(cloneURL, ref, deps)
	}

	// 6. Extract current version of target module
	currentVersion, exists := deps[target.Module]
	if !exists {
		// Dependency not present in go.mod - no update needed
		if r.logger != nil {
			r.logger.Info("dependency not present in go.mod",
				"repo", dependent.Repo,
				"module", target.Module)
		}
		return false, nil
	}

	// 7. Compare versions
	needsUpdate, err := CompareVersions(currentVersion, target.Version)
	if err != nil {
		return true, fmt.Errorf("compare versions: %w", err)
	}

	if r.logger != nil {
		r.logger.Info("remote dependency check result",
			"repo", dependent.Repo,
			"module", target.Module,
			"current_version", currentVersion,
			"target_version", target.Version,
			"needs_update", needsUpdate,
			"cached", false)
	}

	return needsUpdate, nil
}

// Warm prepopulates the cache with dependency information for all dependents.
// This is useful for CI/CD pipelines where multiple checks will be performed.
// The operation is performed in parallel with configurable concurrency.
func (r *remoteDependencyChecker) Warm(
	ctx context.Context,
	dependents []manifest.Dependent,
) error {
	if !r.options.CacheEnabled {
		return nil // No-op if cache is disabled
	}

	if r.logger != nil {
		r.logger.Info("warming dependency cache",
			"count", len(dependents),
			"parallel", r.options.ParallelChecks)
	}

	// Prepopulate cache for all dependents in parallel
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.options.ParallelChecks)
	errs := make([]error, 0)
	var errMu sync.Mutex

	for _, dep := range dependents {
		wg.Add(1)
		go func(dependent manifest.Dependent) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			cloneURL, err := r.gitOps.parseCloneURL(dependent)
			if err != nil {
				errMu.Lock()
				errs = append(errs, fmt.Errorf("parse clone URL for %s: %w", dependent.Repo, err))
				errMu.Unlock()
				return
			}

			ref := dependent.Branch
			if ref == "" {
				ref = "main"
			}

			goModContent, err := r.gitOps.fetchGoMod(ctx, cloneURL, ref)
			if err != nil {
				if r.logger != nil {
					r.logger.Debug("warm cache failed for repository",
						"repo", dependent.Repo,
						"error", err.Error())
				}
				// Non-fatal: continue warming other repos
				return
			}

			deps, err := parseGoModContentAndExtractDeps(goModContent)
			if err != nil {
				if r.logger != nil {
					r.logger.Debug("failed to parse go.mod during warm",
						"repo", dependent.Repo,
						"error", err.Error())
				}
				// Non-fatal: continue warming other repos
				return
			}

			r.cache.Set(cloneURL, ref, deps)

			if r.logger != nil {
				r.logger.Debug("cached dependencies for repository",
					"repo", dependent.Repo,
					"dependencies", len(deps))
			}
		}(dep)
	}

	wg.Wait()

	if len(errs) > 0 {
		if r.logger != nil {
			r.logger.Warn("cache warming completed with errors",
				"total", len(dependents),
				"failed", len(errs))
		}
		return fmt.Errorf("warm cache errors: %d/%d failed", len(errs), len(dependents))
	}

	if r.logger != nil {
		stats := r.cache.Stats()
		r.logger.Info("cache warming completed",
			"cached_repos", stats.Size)
	}

	return nil
}

// ClearCache removes all cached dependency information.
func (r *remoteDependencyChecker) ClearCache() error {
	r.cache.Clear()
	if r.logger != nil {
		r.logger.Debug("dependency cache cleared")
	}
	return nil
}

// GetCacheStats returns current cache statistics.
// This is useful for observability and performance monitoring in CI/CD environments.
func (r *remoteDependencyChecker) GetCacheStats() CacheStats {
	return r.cache.Stats()
}

// LogCacheStats logs current cache statistics with calculated hit rate.
func (r *remoteDependencyChecker) LogCacheStats() {
	if r.logger == nil {
		return
	}

	stats := r.cache.Stats()
	total := stats.Hits + stats.Misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(stats.Hits) / float64(total)
	}

	r.logger.Debug("cache statistics",
		"hits", stats.Hits,
		"misses", stats.Misses,
		"size", stats.Size,
		"hit_rate", hitRate)
}

// parseGoModContentAndExtractDeps parses go.mod content and extracts all dependencies.
// Returns a map of module path -> version for all dependencies.
func parseGoModContentAndExtractDeps(content string) (map[string]string, error) {
	// Parse go.mod content
	f, err := modfile.Parse("go.mod", []byte(content), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod content: %w", err)
	}

	if f.Module == nil {
		return nil, fmt.Errorf("go.mod missing module directive")
	}

	// Extract all dependencies into a map
	deps := make(map[string]string)

	// Add all require directives
	for _, req := range f.Require {
		if req.Mod.Version != "" {
			deps[req.Mod.Path] = req.Mod.Version
		}
	}

	// Handle replace directives (they override require versions)
	for _, r := range f.Replace {
		if r.New.Version != "" {
			// Only track replaces with versions (not local paths)
			deps[r.Old.Path] = r.New.Version
		} else {
			// For local path replaces, remove from deps (can't compare versions)
			delete(deps, r.Old.Path)
		}
	}

	return deps, nil
}
