package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ASTCache provides high-performance AST caching with memory-mapped files
type ASTCache interface {
	Get(key string) (*AST, bool)
	Put(key string, ast *AST) error
	Delete(key string) error
	Stats() ASTCacheStats
	Clear() error
	Close() error
}

// MemoryMappedCache implements ASTCache using memory-mapped files + LRU
type MemoryMappedCache struct {
	cacheDir    string
	maxSize     int64
	maxEntries  int
	mu          sync.RWMutex
	entries     map[string]*cacheEntry
	lru         *lruList
	stats       ASTCacheStats
	totalSize   int64
}

type cacheEntry struct {
	key        string
	filePath   string
	mappedData []byte
	size       int64
	accessTime time.Time
	next       *cacheEntry
	prev       *cacheEntry
}

type lruList struct {
	head *cacheEntry
	tail *cacheEntry
	size int
}

// NewMemoryMappedCache creates a new memory-mapped AST cache
func NewMemoryMappedCache(cacheDir string, maxSize int64, maxEntries int) (ASTCache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cache := &MemoryMappedCache{
		cacheDir:   cacheDir,
		maxSize:    maxSize,
		maxEntries: maxEntries,
		entries:    make(map[string]*cacheEntry),
		lru:        &lruList{},
	}

	// Load existing cache entries
	if err := cache.loadExistingEntries(); err != nil {
		return nil, fmt.Errorf("failed to load existing cache: %w", err)
	}

	return cache, nil
}

// Get retrieves an AST from the cache
func (c *MemoryMappedCache) Get(key string) (*AST, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		c.stats.Misses++
		c.updateHitRate()
		return nil, false
	}

	// Update access time and move to front of LRU
	entry.accessTime = time.Now()
	c.lru.moveToFront(entry)

	// Deserialize AST from memory-mapped data
	var ast AST
	if err := json.Unmarshal(entry.mappedData, &ast); err != nil {
		// Cache corruption, remove entry
		c.removeEntry(key)
		c.stats.Misses++
		c.updateHitRate()
		return nil, false
	}

	c.stats.Hits++
	c.updateHitRate()
	return &ast, true
}

// Put stores an AST in the cache
func (c *MemoryMappedCache) Put(key string, ast *AST) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Serialize AST to JSON
	data, err := json.Marshal(ast)
	if err != nil {
		return fmt.Errorf("failed to serialize AST: %w", err)
	}

	// Check if we need to evict entries
	dataSize := int64(len(data))
	if err := c.ensureCapacity(dataSize); err != nil {
		return fmt.Errorf("failed to ensure cache capacity: %w", err)
	}

	// Create cache file
	filePath := filepath.Join(c.cacheDir, fmt.Sprintf("%s.ast", key))
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create cache file: %w", err)
	}
	defer file.Close()

	// Write data and sync
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write cache data: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync cache file: %w", err)
	}

	// Memory-map the file
	mappedData, err := c.mmapFile(filePath, dataSize)
	if err != nil {
		os.Remove(filePath)
		return fmt.Errorf("failed to memory-map cache file: %w", err)
	}

	// Remove existing entry if it exists
	if existingEntry, exists := c.entries[key]; exists {
		c.removeEntry(key)
		c.unmapFile(existingEntry.mappedData)
	}

	// Create new cache entry
	entry := &cacheEntry{
		key:        key,
		filePath:   filePath,
		mappedData: mappedData,
		size:       dataSize,
		accessTime: time.Now(),
	}

	c.entries[key] = entry
	c.lru.addToFront(entry)
	c.totalSize += dataSize
	c.stats.Size = int64(len(c.entries))
	c.stats.MemoryUsage = c.totalSize

	return nil
}

// Delete removes an AST from the cache
func (c *MemoryMappedCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeEntry(key)
}

// Stats returns cache performance statistics
func (c *MemoryMappedCache) Stats() ASTCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// Clear removes all entries from the cache
func (c *MemoryMappedCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		c.removeEntry(key)
	}

	return nil
}

// Close closes the cache and unmaps all files
func (c *MemoryMappedCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, entry := range c.entries {
		c.unmapFile(entry.mappedData)
	}

	c.entries = make(map[string]*cacheEntry)
	c.lru = &lruList{}
	c.totalSize = 0

	return nil
}

// Helper methods

func (c *MemoryMappedCache) loadExistingEntries() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".ast" {
			key := entry.Name()[:len(entry.Name())-4] // Remove .ast extension
			filePath := filepath.Join(c.cacheDir, entry.Name())

			info, err := entry.Info()
			if err != nil {
				continue
			}

			mappedData, err := c.mmapFile(filePath, info.Size())
			if err != nil {
				continue
			}

			cacheEntry := &cacheEntry{
				key:        key,
				filePath:   filePath,
				mappedData: mappedData,
				size:       info.Size(),
				accessTime: info.ModTime(),
			}

			c.entries[key] = cacheEntry
			c.lru.addToBack(cacheEntry)
			c.totalSize += info.Size()
		}
	}

	c.stats.Size = int64(len(c.entries))
	c.stats.MemoryUsage = c.totalSize
	return nil
}

func (c *MemoryMappedCache) ensureCapacity(newSize int64) error {
	for (c.totalSize+newSize > c.maxSize || len(c.entries) >= c.maxEntries) && c.lru.size > 0 {
		// Evict least recently used entry
		tail := c.lru.tail
		if tail != nil {
			c.removeEntry(tail.key)
		}
	}
	return nil
}

func (c *MemoryMappedCache) removeEntry(key string) error {
	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	// Unmap memory and remove file
	c.unmapFile(entry.mappedData)
	os.Remove(entry.filePath)

	// Remove from data structures
	delete(c.entries, key)
	c.lru.remove(entry)
	c.totalSize -= entry.size

	c.stats.Size = int64(len(c.entries))
	c.stats.MemoryUsage = c.totalSize

	return nil
}

func (c *MemoryMappedCache) updateHitRate() {
	total := c.stats.Hits + c.stats.Misses
	if total > 0 {
		c.stats.HitRate = float64(c.stats.Hits) / float64(total)
	}
}

func (c *MemoryMappedCache) mmapFile(filePath string, size int64) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (c *MemoryMappedCache) unmapFile(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return syscall.Munmap(data)
}

// LRU list methods

func (l *lruList) addToFront(entry *cacheEntry) {
	if l.head == nil {
		l.head = entry
		l.tail = entry
	} else {
		entry.next = l.head
		l.head.prev = entry
		l.head = entry
	}
	l.size++
}

func (l *lruList) addToBack(entry *cacheEntry) {
	if l.tail == nil {
		l.head = entry
		l.tail = entry
	} else {
		entry.prev = l.tail
		l.tail.next = entry
		l.tail = entry
	}
	l.size++
}

func (l *lruList) moveToFront(entry *cacheEntry) {
	if entry == l.head {
		return
	}

	l.remove(entry)
	l.addToFront(entry)
}

func (l *lruList) remove(entry *cacheEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		l.head = entry.next
	}

	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		l.tail = entry.prev
	}

	entry.prev = nil
	entry.next = nil
	l.size--
}