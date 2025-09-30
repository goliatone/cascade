package planner

import (
	"sync"
	"testing"
	"time"
)

func TestDependencyCache_SetAndGet(t *testing.T) {
	cache := newDependencyCache(5 * time.Minute)

	// Setup test data
	cloneURL := "https://github.com/user/repo"
	ref := "main"
	deps := map[string]string{
		"github.com/foo/bar": "v1.2.3",
		"github.com/baz/qux": "v2.0.0",
	}

	// Set dependencies
	cache.Set(cloneURL, ref, deps)

	// Get existing dependency
	version, found := cache.Get(cloneURL, ref, "github.com/foo/bar")
	if !found {
		t.Error("Expected to find cached dependency")
	}
	if version != "v1.2.3" {
		t.Errorf("Expected version v1.2.3, got %s", version)
	}

	// Get another existing dependency
	version, found = cache.Get(cloneURL, ref, "github.com/baz/qux")
	if !found {
		t.Error("Expected to find cached dependency")
	}
	if version != "v2.0.0" {
		t.Errorf("Expected version v2.0.0, got %s", version)
	}

	// Get non-existing dependency
	version, found = cache.Get(cloneURL, ref, "github.com/missing/module")
	if found {
		t.Error("Expected not to find missing dependency")
	}
	if version != "" {
		t.Errorf("Expected empty version for missing dependency, got %s", version)
	}
}

func TestDependencyCache_TTLExpiration(t *testing.T) {
	// Use short TTL for testing
	shortTTL := 50 * time.Millisecond
	cache := newDependencyCache(shortTTL)

	cloneURL := "https://github.com/user/repo"
	ref := "main"
	deps := map[string]string{
		"github.com/foo/bar": "v1.2.3",
	}

	// Set and immediately get
	cache.Set(cloneURL, ref, deps)
	version, found := cache.Get(cloneURL, ref, "github.com/foo/bar")
	if !found {
		t.Error("Expected to find fresh cache entry")
	}
	if version != "v1.2.3" {
		t.Errorf("Expected version v1.2.3, got %s", version)
	}

	// Wait for expiration
	time.Sleep(shortTTL + 10*time.Millisecond)

	// Get after expiration
	version, found = cache.Get(cloneURL, ref, "github.com/foo/bar")
	if found {
		t.Error("Expected cache entry to be expired")
	}
	if version != "" {
		t.Errorf("Expected empty version for expired entry, got %s", version)
	}
}

func TestDependencyCache_Prune(t *testing.T) {
	shortTTL := 50 * time.Millisecond
	cache := newDependencyCache(shortTTL)

	// Add multiple entries
	cache.Set("https://github.com/user/repo1", "main", map[string]string{"mod1": "v1.0.0"})
	cache.Set("https://github.com/user/repo2", "main", map[string]string{"mod2": "v2.0.0"})
	cache.Set("https://github.com/user/repo3", "main", map[string]string{"mod3": "v3.0.0"})

	stats := cache.Stats()
	if stats.Size != 3 {
		t.Errorf("Expected cache size 3, got %d", stats.Size)
	}

	// Wait for expiration
	time.Sleep(shortTTL + 10*time.Millisecond)

	// Prune expired entries
	pruned := cache.Prune()
	if pruned != 3 {
		t.Errorf("Expected to prune 3 entries, pruned %d", pruned)
	}

	stats = cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected empty cache after pruning, size %d", stats.Size)
	}
}

func TestDependencyCache_Clear(t *testing.T) {
	cache := newDependencyCache(5 * time.Minute)

	// Add entries
	cache.Set("https://github.com/user/repo1", "main", map[string]string{"mod1": "v1.0.0"})
	cache.Set("https://github.com/user/repo2", "main", map[string]string{"mod2": "v2.0.0"})

	// Trigger some hits and misses
	cache.Get("https://github.com/user/repo1", "main", "mod1")
	cache.Get("https://github.com/user/repo1", "main", "missing")

	stats := cache.Stats()
	if stats.Size != 2 {
		t.Errorf("Expected cache size 2, got %d", stats.Size)
	}
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	// Clear cache
	cache.Clear()

	stats = cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected empty cache after clear, size %d", stats.Size)
	}
	if stats.Hits != 0 {
		t.Errorf("Expected hits reset to 0, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Expected misses reset to 0, got %d", stats.Misses)
	}
}

func TestDependencyCache_Stats(t *testing.T) {
	cache := newDependencyCache(5 * time.Minute)

	// Initial stats
	stats := cache.Stats()
	if stats.Hits != 0 || stats.Misses != 0 || stats.Size != 0 {
		t.Error("Expected zero initial stats")
	}

	// Add entry
	cache.Set("https://github.com/user/repo", "main", map[string]string{
		"github.com/foo/bar": "v1.2.3",
	})

	// Hit
	cache.Get("https://github.com/user/repo", "main", "github.com/foo/bar")
	stats = cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}

	// Miss (wrong module)
	cache.Get("https://github.com/user/repo", "main", "missing")
	stats = cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	// Miss (wrong URL)
	cache.Get("https://github.com/user/other", "main", "github.com/foo/bar")
	stats = cache.Stats()
	if stats.Misses != 2 {
		t.Errorf("Expected 2 misses, got %d", stats.Misses)
	}

	// Miss (wrong ref)
	cache.Get("https://github.com/user/repo", "develop", "github.com/foo/bar")
	stats = cache.Stats()
	if stats.Misses != 3 {
		t.Errorf("Expected 3 misses, got %d", stats.Misses)
	}

	if stats.Size != 1 {
		t.Errorf("Expected cache size 1, got %d", stats.Size)
	}
}

func TestDependencyCache_ConcurrentAccess(t *testing.T) {
	cache := newDependencyCache(5 * time.Minute)

	const goroutines = 100
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Concurrent writers
	for i := 0; i < goroutines/2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				deps := map[string]string{
					"github.com/foo/bar": "v1.2.3",
				}
				cache.Set("https://github.com/user/repo", "main", deps)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < goroutines/2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				cache.Get("https://github.com/user/repo", "main", "github.com/foo/bar")
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is in consistent state
	stats := cache.Stats()
	if stats.Size < 0 {
		t.Error("Cache size should not be negative")
	}
	if stats.Hits < 0 {
		t.Error("Cache hits should not be negative")
	}
	if stats.Misses < 0 {
		t.Error("Cache misses should not be negative")
	}
}

func TestDependencyCache_ConcurrentPrune(t *testing.T) {
	shortTTL := 50 * time.Millisecond
	cache := newDependencyCache(shortTTL)

	const goroutines = 10

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Add entries
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			deps := map[string]string{
				"github.com/foo/bar": "v1.2.3",
			}
			cache.Set("https://github.com/user/repo", "main", deps)
		}(i)
	}

	// Wait for expiration
	time.Sleep(shortTTL + 10*time.Millisecond)

	// Concurrent prune operations
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			cache.Prune()
		}()
	}

	wg.Wait()

	// Verify final state
	stats := cache.Stats()
	if stats.Size != 0 {
		t.Errorf("Expected empty cache after concurrent prune, size %d", stats.Size)
	}
}

func TestDependencyCache_MultipleBranches(t *testing.T) {
	cache := newDependencyCache(5 * time.Minute)

	cloneURL := "https://github.com/user/repo"

	// Set dependencies for main branch
	cache.Set(cloneURL, "main", map[string]string{
		"github.com/foo/bar": "v1.2.3",
	})

	// Set dependencies for develop branch (different version)
	cache.Set(cloneURL, "develop", map[string]string{
		"github.com/foo/bar": "v2.0.0",
	})

	// Verify main branch
	version, found := cache.Get(cloneURL, "main", "github.com/foo/bar")
	if !found {
		t.Error("Expected to find main branch dependency")
	}
	if version != "v1.2.3" {
		t.Errorf("Expected main branch version v1.2.3, got %s", version)
	}

	// Verify develop branch
	version, found = cache.Get(cloneURL, "develop", "github.com/foo/bar")
	if !found {
		t.Error("Expected to find develop branch dependency")
	}
	if version != "v2.0.0" {
		t.Errorf("Expected develop branch version v2.0.0, got %s", version)
	}

	// Verify cache has 2 entries (one per branch)
	stats := cache.Stats()
	if stats.Size != 2 {
		t.Errorf("Expected cache size 2 (one per branch), got %d", stats.Size)
	}
}

// Benchmark tests

func BenchmarkCache_Get_Hit(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)
	cache.Set("https://github.com/user/repo", "main", map[string]string{
		"github.com/foo/bar": "v1.2.3",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("https://github.com/user/repo", "main", "github.com/foo/bar")
	}
}

func BenchmarkCache_Get_Miss(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("https://github.com/user/repo", "main", "github.com/foo/bar")
	}
}

func BenchmarkCache_Set(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)
	deps := map[string]string{
		"github.com/foo/bar": "v1.2.3",
		"github.com/baz/qux": "v2.0.0",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("https://github.com/user/repo", "main", deps)
	}
}

func BenchmarkCache_ConcurrentReads(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)
	cache.Set("https://github.com/user/repo", "main", map[string]string{
		"github.com/foo/bar": "v1.2.3",
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("https://github.com/user/repo", "main", "github.com/foo/bar")
		}
	})
}

func BenchmarkCache_ConcurrentWrites(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)
	deps := map[string]string{
		"github.com/foo/bar": "v1.2.3",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Set("https://github.com/user/repo", "main", deps)
		}
	})
}

func BenchmarkCache_MixedReadWrite(b *testing.B) {
	cache := newDependencyCache(5 * time.Minute)
	deps := map[string]string{
		"github.com/foo/bar": "v1.2.3",
	}

	// Pre-populate
	cache.Set("https://github.com/user/repo", "main", deps)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				cache.Get("https://github.com/user/repo", "main", "github.com/foo/bar")
			} else {
				cache.Set("https://github.com/user/repo", "main", deps)
			}
			i++
		}
	})
}
