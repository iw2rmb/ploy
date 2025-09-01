package memory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
)

// putOptions holds options for Put operations
type putOptions struct {
	ContentType  string
	Metadata     map[string]string
	CacheControl string
}

// MemoryStorage implements storage.Storage interface with in-memory storage
type MemoryStorage struct {
	mu         sync.RWMutex
	data       map[string][]byte
	metadata   map[string]*storage.Object
	maxMemory  int64
	usedMemory int64
}

// NewMemoryStorage creates a new in-memory storage provider
func NewMemoryStorage(maxMemory int64) *MemoryStorage {
	return &MemoryStorage{
		data:      make(map[string][]byte),
		metadata:  make(map[string]*storage.Object),
		maxMemory: maxMemory,
	}
}

// Get retrieves an object from memory
func (m *MemoryStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, exists := m.data[key]
	if !exists {
		return nil, &storage.StorageError{
			ErrorType: storage.ErrorTypeNotFound,
			Message:   fmt.Sprintf("key not found: %s", key),
			Context: storage.ErrorContext{
				Key: key,
			},
		}
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// Put stores an object in memory
func (m *MemoryStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	// Apply options
	options := &putOptions{}
	// We need to convert storage.PutOption to work with our local putOptions
	// For now, we'll use default values

	// Read all data
	data, err := io.ReadAll(reader)
	if err != nil {
		return &storage.StorageError{
			ErrorType: storage.ErrorTypeInternal,
			Message:   fmt.Sprintf("failed to read data: %v", err),
			Context: storage.ErrorContext{
				Key: key,
			},
		}
	}

	dataSize := int64(len(data))

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check memory limit if set
	if m.maxMemory > 0 {
		// Calculate new memory usage
		oldSize := int64(0)
		if oldData, exists := m.data[key]; exists {
			oldSize = int64(len(oldData))
		}

		newUsage := m.usedMemory - oldSize + dataSize
		if newUsage > m.maxMemory {
			return &storage.StorageError{
				ErrorType: storage.ErrorTypeQuotaExceeded,
				Message:   "memory limit exceeded",
				Context: storage.ErrorContext{
					Key: key,
				},
			}
		}
		m.usedMemory = newUsage
	}

	// Store data
	m.data[key] = data

	// Store metadata
	m.metadata[key] = &storage.Object{
		Key:          key,
		Size:         dataSize,
		ContentType:  options.ContentType,
		LastModified: time.Now(),
		Metadata:     options.Metadata,
	}

	return nil
}

// Delete removes an object from memory
func (m *MemoryStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if data, exists := m.data[key]; exists {
		if m.maxMemory > 0 {
			m.usedMemory -= int64(len(data))
		}
		delete(m.data, key)
		delete(m.metadata, key)
		return nil
	}

	return &storage.StorageError{
		ErrorType: storage.ErrorTypeNotFound,
		Message:   fmt.Sprintf("key not found: %s", key),
		Context: storage.ErrorContext{
			Key: key,
		},
	}
}

// Exists checks if an object exists in memory
func (m *MemoryStorage) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.data[key]
	return exists, nil
}

// List returns objects matching the given options
func (m *MemoryStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []storage.Object
	count := 0

	for key, meta := range m.metadata {
		// Check prefix
		if opts.Prefix != "" && !hasPrefix(key, opts.Prefix) {
			continue
		}

		// Check start after
		if opts.StartAfter != "" && key <= opts.StartAfter {
			continue
		}

		results = append(results, *meta)
		count++

		// Check max keys
		if opts.MaxKeys > 0 && count >= opts.MaxKeys {
			break
		}
	}

	return results, nil
}

// DeleteBatch removes multiple objects from memory
func (m *MemoryStorage) DeleteBatch(ctx context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, key := range keys {
		if data, exists := m.data[key]; exists {
			if m.maxMemory > 0 {
				m.usedMemory -= int64(len(data))
			}
			delete(m.data, key)
			delete(m.metadata, key)
		}
	}

	return nil
}

// Head returns metadata for an object
func (m *MemoryStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	meta, exists := m.metadata[key]
	if !exists {
		return nil, &storage.StorageError{
			ErrorType: storage.ErrorTypeNotFound,
			Message:   fmt.Sprintf("key not found: %s", key),
			Context: storage.ErrorContext{
				Key: key,
			},
		}
	}

	// Return a copy to prevent external modification
	result := *meta
	return &result, nil
}

// UpdateMetadata updates the metadata for an object
func (m *MemoryStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, exists := m.metadata[key]
	if !exists {
		return &storage.StorageError{
			ErrorType: storage.ErrorTypeNotFound,
			Message:   fmt.Sprintf("key not found: %s", key),
			Context: storage.ErrorContext{
				Key: key,
			},
		}
	}

	// Update metadata
	if meta.Metadata == nil {
		meta.Metadata = make(map[string]string)
	}
	for k, v := range metadata {
		meta.Metadata[k] = v
	}

	return nil
}

// Copy duplicates an object in memory
func (m *MemoryStorage) Copy(ctx context.Context, src, dst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srcData, exists := m.data[src]
	if !exists {
		return &storage.StorageError{
			ErrorType: storage.ErrorTypeNotFound,
			Message:   fmt.Sprintf("source key not found: %s", src),
			Context: storage.ErrorContext{
				Key: src,
			},
		}
	}

	srcMeta, _ := m.metadata[src]

	// Check memory limit for copy
	dataSize := int64(len(srcData))
	if m.maxMemory > 0 {
		newUsage := m.usedMemory + dataSize
		if newUsage > m.maxMemory {
			return &storage.StorageError{
				ErrorType: storage.ErrorTypeQuotaExceeded,
				Message:   "memory limit exceeded",
				Context: storage.ErrorContext{
					Key: dst,
				},
			}
		}
		m.usedMemory = newUsage
	}

	// Copy data
	dstData := make([]byte, len(srcData))
	copy(dstData, srcData)
	m.data[dst] = dstData

	// Copy metadata
	dstMeta := *srcMeta
	dstMeta.Key = dst
	dstMeta.LastModified = time.Now()
	m.metadata[dst] = &dstMeta

	return nil
}

// Move relocates an object in memory
func (m *MemoryStorage) Move(ctx context.Context, src, dst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	srcData, exists := m.data[src]
	if !exists {
		return &storage.StorageError{
			ErrorType: storage.ErrorTypeNotFound,
			Message:   fmt.Sprintf("source key not found: %s", src),
			Context: storage.ErrorContext{
				Key: src,
			},
		}
	}

	srcMeta, _ := m.metadata[src]

	// Move data (no memory change since we're just moving)
	m.data[dst] = srcData
	delete(m.data, src)

	// Move metadata
	dstMeta := *srcMeta
	dstMeta.Key = dst
	dstMeta.LastModified = time.Now()
	m.metadata[dst] = &dstMeta
	delete(m.metadata, src)

	return nil
}

// Health checks if the storage is healthy
func (m *MemoryStorage) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if we're over memory limit
	if m.maxMemory > 0 && m.usedMemory > m.maxMemory {
		return &storage.StorageError{
			ErrorType: storage.ErrorTypeInternal,
			Message:   "memory usage exceeds limit",
		}
	}

	return nil
}

// Metrics returns storage metrics
func (m *MemoryStorage) Metrics() *storage.StorageMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := storage.NewStorageMetrics()
	// Set some basic metrics based on current state
	metrics.TotalUploads = int64(len(m.data))
	metrics.TotalBytesUploaded = m.usedMemory

	return metrics
}

// hasPrefix checks if a string has the given prefix
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
