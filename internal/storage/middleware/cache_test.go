package middleware

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for CacheMiddleware
func TestCacheMiddleware_ImplementsStorageInterface(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)

	// This should compile - middleware must implement storage.Storage interface
	var _ storage.Storage = cacheMiddleware
}

func TestCacheMiddleware_Get_CacheHit(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// First Get - should call underlying storage
	reader1, err := cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, reader1)

	// Read content to populate cache
	content1, err := io.ReadAll(reader1)
	require.NoError(t, err)
	reader1.Close()

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Second Get - should be served from cache
	reader2, err := cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	require.NotNil(t, reader2)

	content2, err := io.ReadAll(reader2)
	require.NoError(t, err)
	reader2.Close()

	// Content should match
	assert.Equal(t, content1, content2)

	// Underlying storage should NOT have been called for second Get
	assert.Len(t, mockStorage.getCalls, 0)

	// Verify cache statistics
	stats := cacheMiddleware.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 0.5, stats.HitRate) // 1 hit, 1 miss = 50% hit rate
}

func TestCacheMiddleware_Get_CacheMiss(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// Get non-cached key
	reader, err := cacheMiddleware.Get(ctx, "uncached-key")
	require.NoError(t, err)
	require.NotNil(t, reader)
	reader.Close()

	// Verify underlying storage was called
	assert.Len(t, mockStorage.getCalls, 1)
	assert.Equal(t, "uncached-key", mockStorage.getCalls[0])

	// Verify cache statistics
	stats := cacheMiddleware.GetStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestCacheMiddleware_Put_InvalidatesCache(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// First, populate cache with a Get
	reader, err := cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	io.ReadAll(reader)
	reader.Close()

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Put should invalidate the cache entry
	err = cacheMiddleware.Put(ctx, "test-key", strings.NewReader("new content"))
	require.NoError(t, err)

	// Next Get should call underlying storage (cache was invalidated)
	reader, err = cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	reader.Close()

	// Verify underlying storage was called after Put
	assert.Len(t, mockStorage.getCalls, 1)
}

func TestCacheMiddleware_Delete_InvalidatesCache(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// First, populate cache with a Get
	reader, err := cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	io.ReadAll(reader)
	reader.Close()

	// Delete should invalidate the cache entry
	err = cacheMiddleware.Delete(ctx, "test-key")
	require.NoError(t, err)

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Next Get should call underlying storage (cache was invalidated)
	reader, err = cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	reader.Close()

	// Verify underlying storage was called after Delete
	assert.Len(t, mockStorage.getCalls, 1)
}

func TestCacheMiddleware_TTL_Expiration(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     100 * time.Millisecond, // Short TTL for testing
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// First Get - populate cache
	reader, err := cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	io.ReadAll(reader)
	reader.Close()

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Get should call underlying storage (cache expired)
	reader, err = cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	reader.Close()

	// Verify underlying storage was called after TTL expiration
	assert.Len(t, mockStorage.getCalls, 1)
}

func TestCacheMiddleware_MaxSize_Eviction(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 2, // Very small cache for testing
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// Add first entry
	reader1, err := cacheMiddleware.Get(ctx, "key1")
	require.NoError(t, err)
	io.ReadAll(reader1)
	reader1.Close()

	// Add second entry
	reader2, err := cacheMiddleware.Get(ctx, "key2")
	require.NoError(t, err)
	io.ReadAll(reader2)
	reader2.Close()

	// Add third entry - should evict oldest (key1)
	reader3, err := cacheMiddleware.Get(ctx, "key3")
	require.NoError(t, err)
	io.ReadAll(reader3)
	reader3.Close()

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Get key2 - should be in cache
	reader, err := cacheMiddleware.Get(ctx, "key2")
	require.NoError(t, err)
	reader.Close()
	assert.Len(t, mockStorage.getCalls, 0) // Should be served from cache

	// Get key3 - should be in cache
	reader, err = cacheMiddleware.Get(ctx, "key3")
	require.NoError(t, err)
	reader.Close()
	assert.Len(t, mockStorage.getCalls, 0) // Should be served from cache

	// Get key1 - should NOT be in cache (evicted)
	reader, err = cacheMiddleware.Get(ctx, "key1")
	require.NoError(t, err)
	reader.Close()
	assert.Len(t, mockStorage.getCalls, 1) // Should call underlying storage
}

func TestCacheMiddleware_UpdateMetadata_InvalidatesCache(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// First, populate cache with a Get
	reader, err := cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	io.ReadAll(reader)
	reader.Close()

	// UpdateMetadata should invalidate the cache entry
	metadata := map[string]string{"key": "value"}
	err = cacheMiddleware.UpdateMetadata(ctx, "test-key", metadata)
	require.NoError(t, err)

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Next Get should call underlying storage (cache was invalidated)
	reader, err = cacheMiddleware.Get(ctx, "test-key")
	require.NoError(t, err)
	reader.Close()

	// Verify underlying storage was called after UpdateMetadata
	assert.Len(t, mockStorage.getCalls, 1)
}

func TestCacheMiddleware_Move_InvalidatesCache(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// Populate cache for both source and potential destination
	reader1, err := cacheMiddleware.Get(ctx, "source-key")
	require.NoError(t, err)
	io.ReadAll(reader1)
	reader1.Close()

	reader2, err := cacheMiddleware.Get(ctx, "dest-key")
	require.NoError(t, err)
	io.ReadAll(reader2)
	reader2.Close()

	// Move should invalidate both source and destination cache entries
	err = cacheMiddleware.Move(ctx, "source-key", "dest-key")
	require.NoError(t, err)

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Both keys should now require underlying storage calls
	reader, err := cacheMiddleware.Get(ctx, "source-key")
	if err == nil {
		reader.Close()
	}

	reader, err = cacheMiddleware.Get(ctx, "dest-key")
	require.NoError(t, err)
	reader.Close()

	// Verify underlying storage was called for both
	assert.GreaterOrEqual(t, len(mockStorage.getCalls), 1)
}

func TestCacheMiddleware_ConcurrentAccess(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// Run multiple concurrent operations
	done := make(chan bool, 5)

	// Multiple concurrent Gets for same key
	for i := 0; i < 5; i++ {
		go func() {
			reader, err := cacheMiddleware.Get(ctx, "concurrent-key")
			if err == nil && reader != nil {
				io.ReadAll(reader)
				reader.Close()
			}
			done <- true
		}()
	}

	// Wait for all operations
	for i := 0; i < 5; i++ {
		<-done
	}

	// Should have minimal calls to underlying storage (ideally 1, but may be more due to race)
	assert.LessOrEqual(t, len(mockStorage.getCalls), 2)
}

func TestCacheMiddleware_NonCacheableOperations(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// List operation should not be cached
	objects, err := cacheMiddleware.List(ctx, storage.ListOptions{Prefix: "test/"})
	require.NoError(t, err)
	assert.NotNil(t, objects)

	// Exists operation should not be cached
	exists, err := cacheMiddleware.Exists(ctx, "test-key")
	require.NoError(t, err)
	assert.True(t, exists)

	// Head operation should not be cached
	obj, err := cacheMiddleware.Head(ctx, "test-key")
	require.NoError(t, err)
	assert.NotNil(t, obj)

	// These operations should always call underlying storage
	// (we're not caching them in this minimal implementation)
}

func TestCacheMiddleware_GetStats(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// Initial stats should be zero
	stats := cacheMiddleware.GetStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, 0, stats.Size)

	// Perform some operations
	reader1, _ := cacheMiddleware.Get(ctx, "key1")
	io.ReadAll(reader1)
	reader1.Close()

	reader2, _ := cacheMiddleware.Get(ctx, "key1") // Hit
	reader2.Close()

	reader3, _ := cacheMiddleware.Get(ctx, "key2") // Miss
	io.ReadAll(reader3)
	reader3.Close()

	// Check updated stats
	stats = cacheMiddleware.GetStats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(2), stats.Misses)
	assert.Equal(t, 2, stats.Size)                // 2 entries cached
	assert.InDelta(t, 0.333, stats.HitRate, 0.01) // 1/3 = 33.3% hit rate
}

func TestCacheMiddleware_Clear(t *testing.T) {
	mockStorage := NewMockStorage()
	config := &CacheConfig{
		MaxSize: 100,
		TTL:     1 * time.Minute,
	}

	cacheMiddleware := NewCacheMiddleware(mockStorage, config)
	ctx := context.Background()

	// Populate cache
	reader1, _ := cacheMiddleware.Get(ctx, "key1")
	io.ReadAll(reader1)
	reader1.Close()

	reader2, _ := cacheMiddleware.Get(ctx, "key2")
	io.ReadAll(reader2)
	reader2.Close()

	// Verify cache has entries
	stats := cacheMiddleware.GetStats()
	assert.Equal(t, 2, stats.Size)

	// Clear cache
	cacheMiddleware.Clear()

	// Verify cache is empty
	stats = cacheMiddleware.GetStats()
	assert.Equal(t, 0, stats.Size)

	// Reset mock call count
	mockStorage.getCalls = []string{}

	// Next Get should call underlying storage
	reader, _ := cacheMiddleware.Get(ctx, "key1")
	reader.Close()

	assert.Len(t, mockStorage.getCalls, 1)
}
