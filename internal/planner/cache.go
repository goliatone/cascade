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
// Returns the version and true if found, not expired, and matches the requested target version (when provided).
// Returns empty string and false otherwise which signals callers to perform a fresh fetch.
func (c *dependencyCache) Get(cloneURL, ref, modulePath, targetVersion string) (string, bool) {
	key := cacheKey{cloneURL: cloneURL, ref: ref}

	c.mu.RLock()
	entry, exists := c.entries[key]
	if !exists {
		c.mu.RUnlock()
		atomic.AddInt64(&c.misses, 1)
		return "", false
	}

	expired := time.Since(entry.cachedAt) > entry.ttl
	version, hasModule := entry.dependencies[modulePath]
	c.mu.RUnlock()

	if expired {
		atomic.AddInt64(&c.misses, 1)
		c.deleteEntryIfUnchanged(key, entry)
		return "", false
	}

	if !hasModule {
		atomic.AddInt64(&c.misses, 1)
		return "", false
	}

	if targetVersion != "" {
		cachedNorm := normalizeVersion(version)
		targetNorm := normalizeVersion(targetVersion)
		if cachedNorm != targetNorm {
			atomic.AddInt64(&c.misses, 1)
			c.deleteEntryIfUnchanged(key, entry)
			return "", false
		}
	}

	atomic.AddInt64(&c.hits, 1)
	return version, true
}

func (c *dependencyCache) deleteEntryIfUnchanged(key cacheKey, snapshot *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if current, ok := c.entries[key]; ok && current == snapshot {
		delete(c.entries, key)
	}
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
