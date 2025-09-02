package arf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/iw2rmb/ploy/internal/storage"
)

// Ensure ARFService implements StorageService interface
var _ StorageService = (*ARFService)(nil)

// ARFService provides ARF operations using unified storage interface
// This replaces the old StorageService interface and adapter pattern
// ARFService implements the StorageService interface for backward compatibility
type ARFService struct {
	storage storage.Storage
}

// NewARFService creates a new ARF service with unified storage interface
func NewARFService(storage storage.Storage) (*ARFService, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage cannot be nil")
	}

	return &ARFService{
		storage: storage,
	}, nil
}

// Put stores data at the given key
func (s *ARFService) Put(ctx context.Context, key string, data []byte) error {
	// Debug logging to track storage operations - using log instead of fmt for systemd
	log.Printf("[ARFService.Put] Storing data at key: %s (size: %d bytes)\n", key, len(data))
	reader := bytes.NewReader(data)
	err := s.storage.Put(ctx, key, reader)
	if err != nil {
		log.Printf("[ARFService.Put] ERROR storing at key %s: %v\n", key, err)
	} else {
		log.Printf("[ARFService.Put] SUCCESS stored at key: %s\n", key)
	}
	return err
}

// Get retrieves data from the given key
func (s *ARFService) Get(ctx context.Context, key string) ([]byte, error) {
	reader, err := s.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

// Delete removes data at the given key
func (s *ARFService) Delete(ctx context.Context, key string) error {
	return s.storage.Delete(ctx, key)
}

// Exists checks if a key exists in storage
func (s *ARFService) Exists(ctx context.Context, key string) (bool, error) {
	// Debug logging to track existence checks - using log instead of fmt for systemd
	log.Printf("[ARFService.Exists] Checking existence of key: %s\n", key)
	exists, err := s.storage.Exists(ctx, key)
	if err != nil {
		log.Printf("[ARFService.Exists] ERROR checking key %s: %v\n", key, err)
	} else {
		log.Printf("[ARFService.Exists] Key %s exists: %v\n", key, exists)
	}
	return exists, err
}

// GetStorage returns the underlying storage interface for advanced operations
func (s *ARFService) GetStorage() storage.Storage {
	return s.storage
}
