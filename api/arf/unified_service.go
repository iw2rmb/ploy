package arf

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/iw2rmb/ploy/internal/storage"
)

// Ensure ARFService implements StorageService interface
var _ StorageService = (*ARFService)(nil)

// ARFService provides ARF operations using unified storage interface
// This replaces the old StorageService interface and adapter pattern
// ARFService implements the StorageService interface for backward compatibility
type ARFService struct {
	storage storage.Storage
	bucket  string
}

// NewARFService creates a new ARF service with unified storage interface
func NewARFService(storage storage.Storage, bucket string) (*ARFService, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage cannot be nil")
	}
	if bucket == "" {
		bucket = "arf-recipes" // Default bucket for backward compatibility
	}
	
	return &ARFService{
		storage: storage,
		bucket:  bucket,
	}, nil
}

// Put stores data at the given key with bucket prefixing
func (s *ARFService) Put(ctx context.Context, key string, data []byte) error {
	fullKey := fmt.Sprintf("%s/%s", s.bucket, key)
	reader := bytes.NewReader(data)
	
	return s.storage.Put(ctx, fullKey, reader)
}

// Get retrieves data from the given key with bucket prefixing
func (s *ARFService) Get(ctx context.Context, key string) ([]byte, error) {
	fullKey := fmt.Sprintf("%s/%s", s.bucket, key)
	
	reader, err := s.storage.Get(ctx, fullKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	return io.ReadAll(reader)
}

// Delete removes data at the given key with bucket prefixing
func (s *ARFService) Delete(ctx context.Context, key string) error {
	fullKey := fmt.Sprintf("%s/%s", s.bucket, key)
	
	return s.storage.Delete(ctx, fullKey)
}

// Exists checks if a key exists in storage with bucket prefixing
func (s *ARFService) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := fmt.Sprintf("%s/%s", s.bucket, key)
	
	return s.storage.Exists(ctx, fullKey)
}

// GetStorage returns the underlying storage interface for advanced operations
func (s *ARFService) GetStorage() storage.Storage {
	return s.storage
}

// GetBucket returns the configured bucket name
func (s *ARFService) GetBucket() string {
	return s.bucket
}