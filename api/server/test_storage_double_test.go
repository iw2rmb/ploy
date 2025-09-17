package server

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/storage"
)

// testStorage implements storage.Storage with configurable health/metrics for tests across server package.
type testStorage struct {
	healthy bool
	metrics *storage.StorageMetrics
}

func newTestStorage() *testStorage {
	return &testStorage{healthy: true}
}

func (s *testStorage) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (s *testStorage) Put(context.Context, string, io.Reader, ...storage.PutOption) error {
	return nil
}

func (s *testStorage) Delete(context.Context, string) error { return nil }

func (s *testStorage) Exists(context.Context, string) (bool, error) { return false, nil }

func (s *testStorage) List(context.Context, storage.ListOptions) ([]storage.Object, error) {
	return nil, nil
}

func (s *testStorage) DeleteBatch(context.Context, []string) error { return nil }

func (s *testStorage) Head(context.Context, string) (*storage.Object, error) { return nil, nil }

func (s *testStorage) UpdateMetadata(context.Context, string, map[string]string) error { return nil }

func (s *testStorage) Copy(context.Context, string, string) error { return nil }

func (s *testStorage) Move(ctx context.Context, src, dst string) error {
	return nil
}

func (s *testStorage) Health(context.Context) error {
	if s == nil || s.healthy {
		return nil
	}
	return fmt.Errorf("unhealthy")
}

func (s *testStorage) Metrics() *storage.StorageMetrics {
	if s.metrics == nil {
		s.metrics = storage.NewStorageMetrics()
	}
	return s.metrics
}
