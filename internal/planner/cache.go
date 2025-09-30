package planner

import (
	"sync"
	"sync/atomic"
	"time"
)

// dependencyCache provides thread-safe caching of dependency information
// retrieved from remote repositories. It tracks cache hits/misses and
// automatically handles TTL expiration.
type dependencyCache struct {
	entries map[cacheKey]*cacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	hits    int64
	misses  int64
}

// newDependencyCache creates a new dependency cache with the specified TTL.
func newDependencyCache(ttl time.Duration) *dependencyCache {
	return &dependencyCache{
		entries: make(map[cacheKey]*cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves the version of a module from the cache.
// Returns the version and true if found and not expired, empty string and false otherwise.
func (c *dependencyCache) Get(cloneURL, ref, modulePath string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey{cloneURL: cloneURL, ref: ref}
	entry, exists := c.entries[key]

	// Cache miss: entry doesn't exist
	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return "", false
	}

	// Cache miss: entry expired
	if time.Since(entry.cachedAt) > entry.ttl {
		atomic.AddInt64(&c.misses, 1)
		return "", false
	}

	// Check if the specific module exists in the dependencies
	version, exists := entry.dependencies[modulePath]
	if !exists {
		atomic.AddInt64(&c.misses, 1)
		return "", false
	}

	// Cache hit
	atomic.AddInt64(&c.hits, 1)
	return version, true
}

// Set stores all dependencies for a repository at a specific ref.
// This caches the entire go.mod dependency set for efficient batch lookups.
func (c *dependencyCache) Set(cloneURL, ref string, deps map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey{cloneURL: cloneURL, ref: ref}
	c.entries[key] = &cacheEntry{
		dependencies: deps,
		cachedAt:     time.Now(),
		ttl:          c.ttl,
	}
}

// Clear removes all entries from the cache.
func (c *dependencyCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[cacheKey]*cacheEntry)
	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
}

// Prune removes expired entries from the cache.
// Returns the number of entries pruned.
func (c *dependencyCache) Prune() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	pruned := 0
	for key, entry := range c.entries {
		if time.Since(entry.cachedAt) > entry.ttl {
			delete(c.entries, key)
			pruned++
		}
	}
	return pruned
}

// CacheStats represents cache performance metrics.
type CacheStats struct {
	Hits   int64
	Misses int64
	Size   int
}

// Stats returns current cache statistics.
func (c *dependencyCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Hits:   atomic.LoadInt64(&c.hits),
		Misses: atomic.LoadInt64(&c.misses),
		Size:   len(c.entries),
	}
}
