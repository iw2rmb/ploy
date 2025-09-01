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

// NewStorageAdapter creates a new adapter for the unified storage interface with default bucket
// Deprecated: Use NewARFService directly for new code
func NewStorageAdapter(s storage.Storage) StorageService {
	return NewStorageAdapterWithBucket(s, "arf-recipes")
}

// NewStorageAdapterWithBucket creates a new adapter with a specified bucket
// Deprecated: Use NewARFService directly for new code
func NewStorageAdapterWithBucket(s storage.Storage, bucket string) StorageService {
	// Use the new unified ARF service internally for consistency
	service, err := NewARFService(s, bucket)
	if err != nil {
		// This should not happen with valid parameters, but handle gracefully
		return &fallbackStorageAdapter{storage: s, bucket: bucket}
	}
	return service
}

// fallbackStorageAdapter is a minimal fallback implementation
// This should rarely be used, only if NewARFService fails unexpectedly
type fallbackStorageAdapter struct {
	storage storage.Storage
	bucket  string
}

func (a *fallbackStorageAdapter) Put(ctx context.Context, key string, data []byte) error {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)
	return a.storage.Put(ctx, fullKey, &bytesReader{data: data})
}

func (a *fallbackStorageAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)
	reader, err := a.storage.Get(ctx, fullKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (a *fallbackStorageAdapter) Delete(ctx context.Context, key string) error {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)
	return a.storage.Delete(ctx, fullKey)
}

func (a *fallbackStorageAdapter) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := fmt.Sprintf("%s/%s", a.bucket, key)
	return a.storage.Exists(ctx, fullKey)
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
