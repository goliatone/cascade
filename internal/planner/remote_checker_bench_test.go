package planner

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/goliatone/cascade/internal/manifest"
)

// BenchmarkRemoteChecking_CachePerformance benchmarks remote checking with cache warm/cold
func BenchmarkRemoteChecking_CachePerformance(b *testing.B) {
	ctx := context.Background()
	logger := &mockLogger{}

	// Create mock git operations
	mockGit := &mockGitOperations{
		parseCloneURLFunc: func(dependent manifest.Dependent) (string, error) {
			return "https://github.com/" + dependent.Repo + ".git", nil
		},
		fetchGoModFunc: func(ctx context.Context, cloneURL, ref string) (string, error) {
			return `module test
go 1.21
require github.com/example/lib v1.0.0
`, nil
		},
	}

	b.Run("cold_cache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			// Create new checker for each iteration (cold cache)
			checker := &remoteDependencyChecker{
				cache:  newDependencyCache(5 * time.Minute),
				gitOps: mockGit,
				logger: logger,
			}

			dependent := manifest.Dependent{Repo: "example/test", Branch: "main"}
			target := Target{Module: "github.com/example/lib", Version: "v1.2.0"}

			_, _ = checker.NeedsUpdate(ctx, dependent, target, "")
		}
	})

	b.Run("warm_cache", func(b *testing.B) {
		// Create checker once (warm cache after first call)
		checker := &remoteDependencyChecker{
			cache:  newDependencyCache(5 * time.Minute),
			gitOps: mockGit,
			logger: logger,
		}

		dependent := manifest.Dependent{Repo: "example/test", Branch: "main"}
		target := Target{Module: "github.com/example/lib", Version: "v1.2.0"}

		// Warm the cache
		_, _ = checker.NeedsUpdate(ctx, dependent, target, "")

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = checker.NeedsUpdate(ctx, dependent, target, "")
		}
	})
}

// BenchmarkParallelChecking benchmarks parallel vs sequential dependency checking
func BenchmarkParallelChecking(b *testing.B) {
	ctx := context.Background()
	logger := &mockLogger{}

	// Create mock checker
	mockChecker := &mockDependencyChecker{
		needsUpdateFunc: func(ctx context.Context, dependent manifest.Dependent, target Target, workspace string) (bool, error) {
			return true, nil
		},
	}

	// Create test data
	dependents := make([]manifest.Dependent, 20)
	for i := 0; i < 20; i++ {
		dependents[i] = manifest.Dependent{
			Repo:   fmt.Sprintf("example/repo-%d", i),
			Branch: "main",
		}
	}

	target := Target{Module: "github.com/example/lib", Version: "v1.2.0"}

	b.Run("sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, dep := range dependents {
				_, _ = mockChecker.NeedsUpdate(ctx, dep, target, "")
			}
		}
	})

	b.Run("parallel_4", func(b *testing.B) {
		parallel := NewParallelDependencyChecker(mockChecker, 4, logger)
		for i := 0; i < b.N; i++ {
			for _, dep := range dependents {
				_, _ = parallel.NeedsUpdate(ctx, dep, target, "")
			}
		}
	})

	b.Run("parallel_8", func(b *testing.B) {
		parallel := NewParallelDependencyChecker(mockChecker, 8, logger)
		for i := 0; i < b.N; i++ {
			for _, dep := range dependents {
				_, _ = parallel.NeedsUpdate(ctx, dep, target, "")
			}
		}
	})
}

// BenchmarkCacheOperations benchmarks cache performance
func BenchmarkCacheOperations(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)

	// Pre-populate cache with some entries
	for i := 0; i < 100; i++ {
		cloneURL := fmt.Sprintf("https://github.com/example/repo-%d.git", i)
		deps := map[string]string{
			"github.com/example/lib1": "v1.0.0",
			"github.com/example/lib2": "v2.0.0",
			"github.com/example/lib3": "v3.0.0",
		}
		cache.Set(cloneURL, "main", deps)
	}

	b.Run("cache_hit", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = cache.Get("https://github.com/example/repo-50.git", "main", "github.com/example/lib1", "")
		}
	})

	b.Run("cache_miss", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = cache.Get("https://github.com/example/nonexistent.git", "main", "github.com/example/lib1", "")
		}
	})

	b.Run("cache_set", func(b *testing.B) {
		deps := map[string]string{
			"github.com/example/lib": "v1.0.0",
		}
		for i := 0; i < b.N; i++ {
			cache.Set(fmt.Sprintf("https://github.com/example/bench-%d.git", i), "main", deps)
		}
	})
}
