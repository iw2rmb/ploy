package arf

import (
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/storage"
)

// StorageService provides generic storage operations for ARF/OpenRewrite
type StorageService interface {
	// Basic storage operations
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// StorageAdapter adapts the new storage.Storage interface to ARF StorageService
type StorageAdapter struct {
	storage storage.Storage
	bucket  string
}

// NewStorageAdapter creates a new adapter for the unified storage interface
func NewStorageAdapter(s storage.Storage) StorageService {
	return &StorageAdapter{
		storage: s,
		bucket:  "arf-recipes", // Default bucket for ARF recipes
	}
}

// Put stores data at the given key
func (a *StorageAdapter) Put(ctx context.Context, key string, data []byte) error {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)
	reader := &bytesReader{data: data}

	// Use the new storage interface Put method
	err := a.storage.Put(ctx, fullKey, reader)
	if err != nil {
		return fmt.Errorf("failed to put key %s: %w", key, err)
	}

	return nil
}

// Get retrieves data from the given key
func (a *StorageAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)

	// Use the new storage interface Get method
	reader, err := a.storage.Get(ctx, fullKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get key %s: %w", key, err)
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// Delete removes data at the given key
func (a *StorageAdapter) Delete(ctx context.Context, key string) error {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)

	// Use the new storage interface Delete method
	err := a.storage.Delete(ctx, fullKey)
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}

	return nil
}

// Exists checks if a key exists in storage
func (a *StorageAdapter) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)

	// Use the new storage interface Exists method
	exists, err := a.storage.Exists(ctx, fullKey)
	if err != nil {
		return false, fmt.Errorf("failed to check existence of key %s: %w", key, err)
	}

	return exists, nil
}

// bytesReader implements io.Reader for []byte
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
