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

// NewStorageAdapter creates a new adapter for the unified storage interface
// Deprecated: Use NewARFService directly for new code
func NewStorageAdapter(s storage.Storage) StorageService {
	// Use the new unified ARF service internally for consistency
	service, err := NewARFService(s)
	if err != nil {
		// This should not happen with valid parameters
		panic(fmt.Sprintf("failed to create ARF service: %v", err))
	}
	return service
}

// NewStorageAdapterWithBucket creates a new adapter (bucket parameter is ignored)
// Deprecated: Use NewARFService directly for new code. The bucket parameter is no longer used.
func NewStorageAdapterWithBucket(s storage.Storage, bucket string) StorageService {
	// Ignore bucket parameter - storage provider manages its own bucket/collection
	return NewStorageAdapter(s)
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
