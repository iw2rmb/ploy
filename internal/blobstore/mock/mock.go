// Package mock provides an in-memory implementation of blobstore.Store for testing.
package mock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/iw2rmb/ploy/internal/blobstore"
)

// ErrNotFound is returned when an object is not found.
// It wraps blobstore.ErrNotFound so callers can use errors.Is(err, blobstore.ErrNotFound).
var ErrNotFound = fmt.Errorf("mock: %w", blobstore.ErrNotFound)

// Store is an in-memory implementation of blobstore.Store.
type Store struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

// Ensure Store implements blobstore.Store.
var _ blobstore.Store = (*Store)(nil)

// New creates a new mock blobstore.
func New() *Store {
	return &Store{
		objects: make(map[string][]byte),
	}
}

// Put stores data at the given key.
func (s *Store) Put(_ context.Context, key, _ string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Make a copy to prevent external modification.
	s.objects[key] = append([]byte(nil), data...)
	return "mock-etag", nil
}

// Get retrieves an object by key.
func (s *Store) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, 0, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
}

// Delete removes an object by key.
func (s *Store) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

// Count returns the number of stored objects (for testing).
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.objects)
}

// GetData returns the raw data for a key (for testing).
func (s *Store) GetData(key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}
