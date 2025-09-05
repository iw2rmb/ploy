package analysis

import (
	"sync"
	"time"
)

// cacheEntry represents a cached analysis result
type cacheEntry struct {
	result    *AnalysisResult
	expiresAt time.Time
}

// InMemoryCache is an in-memory implementation of CacheManager
type InMemoryCache struct {
	entries map[string]*cacheEntry
	mu      sync.RWMutex
	hits    int64
	misses  int64
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache() *InMemoryCache {
	cache := &InMemoryCache{
		entries: make(map[string]*cacheEntry),
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// Get retrieves a cached result
func (c *InMemoryCache) Get(key string) (*AnalysisResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		c.misses++
		return nil, false
	}

	c.hits++
	return entry.result, true
}

// Set stores a result in the cache
func (c *InMemoryCache) Set(key string, result *AnalysisResult, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes a result from the cache
func (c *InMemoryCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
	return nil
}

// Clear removes all entries from the cache
func (c *InMemoryCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry)
	c.hits = 0
	c.misses = 0

	return nil
}

// GetMetrics returns cache metrics
func (c *InMemoryCache) GetMetrics() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(c.hits) / float64(total) * 100
	}

	return map[string]int64{
		"hits":     c.hits,
		"misses":   c.misses,
		"entries":  int64(len(c.entries)),
		"hit_rate": int64(hitRate),
	}
}

// cleanupExpired periodically removes expired entries
func (c *InMemoryCache) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}
