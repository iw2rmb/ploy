package sbom

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	storage "github.com/iw2rmb/ploy/internal/storage"
)

// mockStorage is a simple in-memory implementation of storage.Storage for tests.
// It stores objects in a map and supports basic Get/Put/List/Head operations.
type mockStorage struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockStorage() *mockStorage {
	return &mockStorage{data: make(map[string][]byte)}
}

func (m *mockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.data[key]
	if !ok {
		return io.NopCloser(bytes.NewReader(nil)), errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (m *mockStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.data[key] = b
	return nil
}

func (m *mockStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockStorage) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]storage.Object, 0, len(m.data))
	for k, v := range m.data {
		if opts.Prefix != "" && !bytes.HasPrefix([]byte(k), []byte(opts.Prefix)) {
			continue
		}
		out = append(out, storage.Object{
			Key:          k,
			Size:         int64(len(v)),
			ContentType:  "application/octet-stream",
			ETag:         "",
			LastModified: time.Now(),
			Metadata:     map[string]string{},
		})
	}
	return out, nil
}

func (m *mockStorage) DeleteBatch(ctx context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}

func (m *mockStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &storage.Object{
		Key:          key,
		Size:         int64(len(b)),
		ContentType:  "application/octet-stream",
		ETag:         "",
		LastModified: time.Now(),
		Metadata:     map[string]string{},
	}, nil
}

func (m *mockStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	// Not needed for tests; pretend success
	return nil
}

func (m *mockStorage) Copy(ctx context.Context, src, dst string) error {
	m.mu.RLock()
	b, ok := m.data[src]
	m.mu.RUnlock()
	if !ok {
		return errors.New("not found")
	}
	m.mu.Lock()
	m.data[dst] = append([]byte(nil), b...)
	m.mu.Unlock()
	return nil
}

func (m *mockStorage) Move(ctx context.Context, src, dst string) error {
	if err := m.Copy(ctx, src, dst); err != nil {
		return err
	}
	return m.Delete(ctx, src)
}

func (m *mockStorage) Health(ctx context.Context) error { return nil }

func (m *mockStorage) Metrics() *storage.StorageMetrics { return storage.NewStorageMetrics() }
