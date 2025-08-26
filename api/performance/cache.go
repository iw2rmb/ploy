package performance

import (
	"sync"
	"time"
)

// CacheEntry represents a cached value with expiration
type CacheEntry struct {
	Value     interface{}
	ExpiresAt time.Time
}

// MemoryCache provides a thread-safe in-memory cache with TTL
type MemoryCache struct {
	entries map[string]*CacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
}

// NewMemoryCache creates a new memory cache with the specified TTL
func NewMemoryCache(ttl time.Duration) *MemoryCache {
	cache := &MemoryCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
	}
	
	// Start cleanup goroutine
	go cache.cleanup()
	
	return cache
}

// Get retrieves a value from the cache
func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}
	
	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	
	return entry.Value, true
}

// Set stores a value in the cache with TTL
func (c *MemoryCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries[key] = &CacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a key from the cache
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.entries, key)
}

// Clear removes all entries from the cache
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries = make(map[string]*CacheEntry)
}

// Size returns the number of entries in the cache
func (c *MemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return len(c.entries)
}

// cleanup periodically removes expired entries
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(c.ttl / 2) // Cleanup at half the TTL interval
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.ExpiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// Stats returns cache statistics
type CacheStats struct {
	Size    int
	Hits    int64
	Misses  int64
	HitRate float64
}

// StatefulCache extends MemoryCache with hit/miss tracking
type StatefulCache struct {
	*MemoryCache
	hits   int64
	misses int64
	mu     sync.RWMutex
}

// NewStatefulCache creates a cache with statistics tracking
func NewStatefulCache(ttl time.Duration) *StatefulCache {
	return &StatefulCache{
		MemoryCache: NewMemoryCache(ttl),
	}
}

// Get retrieves a value and tracks hits/misses
func (c *StatefulCache) Get(key string) (interface{}, bool) {
	value, exists := c.MemoryCache.Get(key)
	
	c.mu.Lock()
	if exists {
		c.hits++
	} else {
		c.misses++
	}
	c.mu.Unlock()
	
	return value, exists
}

// Stats returns cache statistics
func (c *StatefulCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}
	
	return CacheStats{
		Size:    c.MemoryCache.Size(),
		Hits:    c.hits,
		Misses:  c.misses,
		HitRate: hitRate,
	}
}

// ResetStats resets hit/miss statistics
func (c *StatefulCache) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.hits = 0
	c.misses = 0
}