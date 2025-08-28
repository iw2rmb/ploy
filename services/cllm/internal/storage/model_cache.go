package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/services/cllm/internal/config"
)

// ModelCache provides local caching for LLM models with LRU eviction
type ModelCache struct {
	config     config.LocalCacheConfig
	cacheDir   string
	entries    map[string]*CacheEntry
	accessList *AccessList
	totalSize  int64
	mu         sync.RWMutex
	metrics    *CacheMetrics
}

// CacheEntry represents a cached model with metadata
type CacheEntry struct {
	ModelID     string
	FilePath    string
	Size        int64
	AccessTime  time.Time
	CreatedTime time.Time
	Metadata    ModelMetadata
	next        *CacheEntry
	prev        *CacheEntry
}

// AccessList implements doubly-linked list for LRU tracking
type AccessList struct {
	head *CacheEntry
	tail *CacheEntry
}

// CacheMetrics tracks cache performance
type CacheMetrics struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	TotalSize   int64
	ModelCount  int
	LastCleanup time.Time
}

// NewModelCache creates a new model cache with the given configuration
func NewModelCache(config config.LocalCacheConfig) (*ModelCache, error) {
	if !config.Enabled {
		return nil, nil
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(config.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cache := &ModelCache{
		config:     config,
		cacheDir:   config.CacheDir,
		entries:    make(map[string]*CacheEntry),
		accessList: &AccessList{},
		metrics:    &CacheMetrics{LastCleanup: time.Now()},
	}

	// Initialize doubly-linked list
	cache.accessList.head = &CacheEntry{}
	cache.accessList.tail = &CacheEntry{}
	cache.accessList.head.next = cache.accessList.tail
	cache.accessList.tail.prev = cache.accessList.head

	// Load existing cache entries from disk
	if err := cache.loadExistingEntries(); err != nil {
		return nil, fmt.Errorf("failed to load existing cache entries: %w", err)
	}

	return cache, nil
}

// Get retrieves a model from cache, returning reader and metadata
func (c *ModelCache) Get(ctx context.Context, modelID string) (io.ReadCloser, *ModelMetadata, bool) {
	if c == nil {
		return nil, nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[modelID]
	if !exists {
		c.metrics.Misses++
		return nil, nil, false
	}

	// Check if file still exists on disk
	if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
		// Remove stale entry
		c.removeEntryUnsafe(entry)
		c.metrics.Misses++
		return nil, nil, false
	}

	// Update access time and move to front of LRU list
	entry.AccessTime = time.Now()
	c.moveToFrontUnsafe(entry)

	// Open file for reading
	file, err := os.Open(entry.FilePath)
	if err != nil {
		c.metrics.Misses++
		return nil, nil, false
	}

	c.metrics.Hits++
	return file, &entry.Metadata, true
}

// Put stores a model in cache with metadata
func (c *ModelCache) Put(ctx context.Context, modelID string, reader io.Reader, metadata ModelMetadata) error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create cache file path
	fileName := c.sanitizeModelID(modelID)
	filePath := filepath.Join(c.cacheDir, fileName)

	// Create temporary file first
	tempPath := filePath + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}
	defer tempFile.Close()

	// Copy data to temp file and get size
	size, err := io.Copy(tempFile, reader)
	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to write to cache file: %w", err)
	}

	// Check cache limits and make room if necessary
	if err := c.makeRoomUnsafe(size); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to make room in cache: %w", err)
	}

	// Atomic rename to final location
	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename cache file: %w", err)
	}

	// Remove existing entry if present (but not the file since we're overwriting)
	if existingEntry, exists := c.entries[modelID]; exists {
		// Remove from map and linked list, but don't delete file
		delete(c.entries, existingEntry.ModelID)
		if existingEntry.prev != nil {
			existingEntry.prev.next = existingEntry.next
		}
		if existingEntry.next != nil {
			existingEntry.next.prev = existingEntry.prev
		}
		c.totalSize -= existingEntry.Size
	}

	// Create new cache entry
	entry := &CacheEntry{
		ModelID:     modelID,
		FilePath:    filePath,
		Size:        size,
		AccessTime:  time.Now(),
		CreatedTime: time.Now(),
		Metadata:    metadata,
	}

	// Add to cache
	c.entries[modelID] = entry
	c.addToFrontUnsafe(entry)
	c.totalSize += size
	c.updateMetricsUnsafe()

	return nil
}

// Remove removes a model from cache
func (c *ModelCache) Remove(modelID string) error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[modelID]
	if !exists {
		return nil // Already removed
	}

	c.removeEntryUnsafe(entry)
	return nil
}

// Clear removes all entries from cache
func (c *ModelCache) Clear() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.entries {
		os.Remove(entry.FilePath)
	}

	c.entries = make(map[string]*CacheEntry)
	c.accessList.head.next = c.accessList.tail
	c.accessList.tail.prev = c.accessList.head
	c.totalSize = 0
	c.updateMetricsUnsafe()

	return nil
}

// GetMetrics returns current cache performance metrics
func (c *ModelCache) GetMetrics() CacheMetrics {
	if c == nil {
		return CacheMetrics{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics := *c.metrics
	metrics.TotalSize = c.totalSize
	metrics.ModelCount = len(c.entries)
	return metrics
}

// Cleanup performs maintenance operations (remove expired entries, enforce limits)
func (c *ModelCache) Cleanup() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check each entry for validity
	var toRemove []*CacheEntry
	for _, entry := range c.entries {
		if _, err := os.Stat(entry.FilePath); os.IsNotExist(err) {
			toRemove = append(toRemove, entry)
		}
	}

	// Remove invalid entries
	for _, entry := range toRemove {
		c.removeEntryUnsafe(entry)
	}

	// Enforce size and count limits
	if err := c.makeRoomUnsafe(0); err != nil {
		return err
	}

	c.metrics.LastCleanup = time.Now()
	return nil
}

// Private helper methods

// loadExistingEntries scans cache directory and loads existing entries
func (c *ModelCache) loadExistingEntries() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}

		filePath := filepath.Join(c.cacheDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Create cache entry from file
		modelID := c.desanitizeFileName(entry.Name())
		cacheEntry := &CacheEntry{
			ModelID:     modelID,
			FilePath:    filePath,
			Size:        info.Size(),
			AccessTime:  info.ModTime(),
			CreatedTime: info.ModTime(),
			Metadata: ModelMetadata{
				ID:         modelID,
				Size:       info.Size(),
				UploadedAt: info.ModTime(),
			},
		}

		c.entries[modelID] = cacheEntry
		c.addToFrontUnsafe(cacheEntry)
		c.totalSize += cacheEntry.Size
	}

	c.updateMetricsUnsafe()
	return nil
}

// makeRoomUnsafe ensures cache has room for new entry of given size
func (c *ModelCache) makeRoomUnsafe(newSize int64) error {
	maxSizeBytes := int64(c.config.MaxSizeGB) * 1024 * 1024 * 1024
	maxModels := c.config.MaxModels

	// Enforce model count limit first
	for len(c.entries) >= maxModels {
		if c.accessList.tail.prev == c.accessList.head {
			break // Empty list
		}
		oldest := c.accessList.tail.prev
		c.removeEntryUnsafe(oldest)
		c.metrics.Evictions++
	}

	// Enforce size limit
	for c.totalSize+newSize > maxSizeBytes && c.accessList.tail.prev != c.accessList.head {
		oldest := c.accessList.tail.prev
		c.removeEntryUnsafe(oldest)
		c.metrics.Evictions++
	}

	// Check if we still don't have enough room
	if c.totalSize+newSize > maxSizeBytes {
		return fmt.Errorf("model size %d bytes exceeds cache capacity %d bytes", newSize, maxSizeBytes)
	}

	return nil
}

// removeEntryUnsafe removes entry from cache and filesystem
func (c *ModelCache) removeEntryUnsafe(entry *CacheEntry) {
	// Remove from map
	delete(c.entries, entry.ModelID)

	// Remove from linked list
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}

	// Remove file
	os.Remove(entry.FilePath)

	// Update total size
	c.totalSize -= entry.Size

	c.updateMetricsUnsafe()
}

// addToFrontUnsafe adds entry to front of LRU list (most recently used)
func (c *ModelCache) addToFrontUnsafe(entry *CacheEntry) {
	entry.next = c.accessList.head.next
	entry.prev = c.accessList.head
	c.accessList.head.next.prev = entry
	c.accessList.head.next = entry
}

// moveToFrontUnsafe moves existing entry to front of LRU list
func (c *ModelCache) moveToFrontUnsafe(entry *CacheEntry) {
	// Remove from current position
	entry.prev.next = entry.next
	entry.next.prev = entry.prev

	// Add to front
	c.addToFrontUnsafe(entry)
}

// updateMetricsUnsafe updates internal metrics
func (c *ModelCache) updateMetricsUnsafe() {
	c.metrics.TotalSize = c.totalSize
	c.metrics.ModelCount = len(c.entries)
}

// sanitizeModelID creates a safe filename from model ID
func (c *ModelCache) sanitizeModelID(modelID string) string {
	// Replace unsafe characters with underscores
	safe := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	).Replace(modelID)
	
	return safe + ".model"
}

// desanitizeFileName extracts model ID from filename
func (c *ModelCache) desanitizeFileName(filename string) string {
	if strings.HasSuffix(filename, ".model") {
		return strings.TrimSuffix(filename, ".model")
	}
	return filename
}