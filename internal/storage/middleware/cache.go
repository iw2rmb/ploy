package middleware

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// CacheEntry represents a cached storage object
type CacheEntry struct {
	Content   []byte
	Timestamp time.Time
}

// CacheStats represents cache statistics
type CacheStats struct {
	Hits    int64
	Misses  int64
	Size    int
	HitRate float64
}

// CacheConfig configures the cache middleware
type CacheConfig struct {
	MaxSize int           // Maximum number of entries in cache
	TTL     time.Duration // Time-to-live for cache entries
}

// CacheMiddleware implements storage.Storage with caching
type CacheMiddleware struct {
	next   storage.Storage
	config *CacheConfig
	cache  map[string]*CacheEntry
	order  []string // Track insertion order for LRU eviction
	mu     sync.RWMutex
	hits   int64
	misses int64
}

// NewCacheMiddleware creates a new cache middleware
func NewCacheMiddleware(next storage.Storage, config *CacheConfig) *CacheMiddleware {
	if config == nil {
		config = &CacheConfig{
			MaxSize: 1000,
			TTL:     5 * time.Minute,
		}
	}

	return &CacheMiddleware{
		next:   next,
		config: config,
		cache:  make(map[string]*CacheEntry),
		order:  make([]string, 0, config.MaxSize),
	}
}

// Get retrieves an object, using cache when possible
func (c *CacheMiddleware) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	// Check cache first
	c.mu.RLock()
	entry, exists := c.cache[key]
	if exists {
		// Check if entry is still valid (not expired)
		if time.Since(entry.Timestamp) < c.config.TTL {
			c.hits++
			content := make([]byte, len(entry.Content))
			copy(content, entry.Content)
			c.mu.RUnlock()
			return io.NopCloser(bytes.NewReader(content)), nil
		}
		// Entry expired, remove it
		c.mu.RUnlock()
		c.invalidate(key)
	} else {
		c.mu.RUnlock()
	}

	// Cache miss - fetch from underlying storage
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()

	reader, err := c.next.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Read content to cache it
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict (LRU)
	if len(c.cache) >= c.config.MaxSize && !exists {
		// Evict oldest entry
		if len(c.order) > 0 {
			oldestKey := c.order[0]
			delete(c.cache, oldestKey)
			c.order = c.order[1:]
		}
	}

	// Add new entry
	c.cache[key] = &CacheEntry{
		Content:   content,
		Timestamp: time.Now(),
	}

	// Update order for LRU
	if !exists {
		c.order = append(c.order, key)
	} else {
		// Move to end if it was already in order
		c.updateOrder(key)
	}

	// Return a new reader with the cached content
	return io.NopCloser(bytes.NewReader(content)), nil
}

// Put stores an object and invalidates cache
func (c *CacheMiddleware) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	// Invalidate cache entry for this key
	c.invalidate(key)

	// Pass through to underlying storage
	return c.next.Put(ctx, key, reader, opts...)
}

// Delete removes an object and invalidates cache
func (c *CacheMiddleware) Delete(ctx context.Context, key string) error {
	// Invalidate cache entry for this key
	c.invalidate(key)

	// Pass through to underlying storage
	return c.next.Delete(ctx, key)
}

// Exists checks if an object exists (not cached)
func (c *CacheMiddleware) Exists(ctx context.Context, key string) (bool, error) {
	// Don't cache exists checks
	return c.next.Exists(ctx, key)
}

// List lists objects (not cached)
func (c *CacheMiddleware) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	// Don't cache list operations
	return c.next.List(ctx, opts)
}

// DeleteBatch deletes multiple objects and invalidates cache
func (c *CacheMiddleware) DeleteBatch(ctx context.Context, keys []string) error {
	// Invalidate all keys
	for _, key := range keys {
		c.invalidate(key)
	}

	// Pass through to underlying storage
	return c.next.DeleteBatch(ctx, keys)
}

// Head gets object metadata (not cached)
func (c *CacheMiddleware) Head(ctx context.Context, key string) (*storage.Object, error) {
	// Don't cache head operations
	return c.next.Head(ctx, key)
}

// UpdateMetadata updates object metadata and invalidates cache
func (c *CacheMiddleware) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	// Invalidate cache entry for this key
	c.invalidate(key)

	// Pass through to underlying storage
	return c.next.UpdateMetadata(ctx, key, metadata)
}

// Copy copies an object (cache not affected)
func (c *CacheMiddleware) Copy(ctx context.Context, src, dst string) error {
	// Invalidate destination cache (it will be overwritten)
	c.invalidate(dst)

	// Pass through to underlying storage
	return c.next.Copy(ctx, src, dst)
}

// Move moves an object and invalidates cache for both keys
func (c *CacheMiddleware) Move(ctx context.Context, src, dst string) error {
	// Invalidate both source and destination
	c.invalidate(src)
	c.invalidate(dst)

	// Pass through to underlying storage
	return c.next.Move(ctx, src, dst)
}

// Health checks storage health (not cached)
func (c *CacheMiddleware) Health(ctx context.Context) error {
	// Don't cache health checks
	return c.next.Health(ctx)
}

// Metrics returns storage metrics from underlying storage
func (c *CacheMiddleware) Metrics() *storage.StorageMetrics {
	return c.next.Metrics()
}

// GetStats returns cache statistics
func (c *CacheMiddleware) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Size:    len(c.cache),
		HitRate: hitRate,
	}
}

// Clear removes all entries from the cache
func (c *CacheMiddleware) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CacheEntry)
	c.order = make([]string, 0, c.config.MaxSize)
}

// invalidate removes a key from the cache
func (c *CacheMiddleware) invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.cache[key]; exists {
		delete(c.cache, key)
		// Remove from order
		c.removeFromOrder(key)
	}
}

// removeFromOrder removes a key from the order slice
func (c *CacheMiddleware) removeFromOrder(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// updateOrder moves a key to the end of the order slice (most recently used)
func (c *CacheMiddleware) updateOrder(key string) {
	c.removeFromOrder(key)
	c.order = append(c.order, key)
}
