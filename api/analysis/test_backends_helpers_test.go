package analysis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	orchestration "github.com/iw2rmb/ploy/internal/orchestration"
	istorage "github.com/iw2rmb/ploy/internal/storage"
)

type testKV struct {
	mu        sync.RWMutex
	data      map[string][]byte
	putErr    error
	getErr    error
	keysErr   error
	deleteErr error
}

func newTestKV() orchestration.KV {
	return &testKV{data: make(map[string][]byte)}
}

func (k *testKV) Put(key string, value []byte) error {
	if k.putErr != nil {
		return k.putErr
	}
	if key == "" {
		return errors.New("key cannot be empty")
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	if value == nil {
		delete(k.data, key)
		return nil
	}

	copied := append([]byte(nil), value...)
	k.data[key] = copied
	return nil
}

func (k *testKV) Get(key string) ([]byte, error) {
	if k.getErr != nil {
		return nil, k.getErr
	}

	k.mu.RLock()
	defer k.mu.RUnlock()

	value, ok := k.data[key]
	if !ok {
		return nil, nil
	}
	copied := append([]byte(nil), value...)
	return copied, nil
}

func (k *testKV) Keys(prefix, separator string) ([]string, error) {
	if k.keysErr != nil {
		return nil, k.keysErr
	}

	k.mu.RLock()
	defer k.mu.RUnlock()

	keys := make([]string, 0, len(k.data))
	for key := range k.data {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)
	return keys, nil
}

func (k *testKV) Delete(key string) error {
	if k.deleteErr != nil {
		return k.deleteErr
	}

	k.mu.Lock()
	defer k.mu.Unlock()

	delete(k.data, key)
	return nil
}

type storedObject struct {
	data        []byte
	metadata    map[string]string
	contentType string
	lastUpdated time.Time
}

type testStorage struct {
	mu      sync.RWMutex
	objects map[string]storedObject
	metrics *istorage.StorageMetrics

	putErr         error
	getErr         error
	deleteErr      error
	existsErr      error
	listErr        error
	deleteBatchErr error
	headErr        error
	updateErr      error
	copyErr        error
	moveErr        error
	healthErr      error
}

func newTestStorage() istorage.Storage {
	return &testStorage{
		objects: make(map[string]storedObject),
		metrics: istorage.NewStorageMetrics(),
	}
}

func (s *testStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...istorage.PutOption) error {
	if s.putErr != nil {
		return s.putErr
	}
	if key == "" {
		return errors.New("key cannot be empty")
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	metadata := make(map[string]string)
	contentType := ""
	// Options are intentionally ignored for simplicity as tests only rely on stored payload.

	s.mu.Lock()
	defer s.mu.Unlock()

	s.objects[key] = storedObject{
		data:        append([]byte(nil), data...),
		metadata:    metadata,
		contentType: contentType,
		lastUpdated: time.Now(),
	}

	if s.metrics != nil {
		s.metrics.RecordUpload(true, 0, int64(len(data)), "")
	}

	return nil
}

func (s *testStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}

	s.mu.RLock()
	obj, ok := s.objects[key]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}

	if s.metrics != nil {
		s.metrics.RecordDownload(true, 0, int64(len(obj.data)), "")
	}

	return io.NopCloser(bytes.NewReader(append([]byte(nil), obj.data...))), nil
}

func (s *testStorage) Delete(ctx context.Context, key string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.objects, key)
	return nil
}

func (s *testStorage) Exists(ctx context.Context, key string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}

	s.mu.RLock()
	_, ok := s.objects[key]
	s.mu.RUnlock()

	return ok, nil
}

func (s *testStorage) List(ctx context.Context, opts istorage.ListOptions) ([]istorage.Object, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var objects []istorage.Object
	for key, obj := range s.objects {
		if opts.Prefix != "" && !strings.HasPrefix(key, opts.Prefix) {
			continue
		}
		if opts.StartAfter != "" && key <= opts.StartAfter {
			continue
		}

		objects = append(objects, istorage.Object{
			Key:          key,
			Size:         int64(len(obj.data)),
			LastModified: obj.lastUpdated,
			ContentType:  obj.contentType,
			Metadata:     cloneStringMap(obj.metadata),
		})
	}

	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })

	if opts.MaxKeys > 0 && len(objects) > opts.MaxKeys {
		objects = objects[:opts.MaxKeys]
	}

	return objects, nil
}

func (s *testStorage) DeleteBatch(ctx context.Context, keys []string) error {
	if s.deleteBatchErr != nil {
		return s.deleteBatchErr
	}

	for _, key := range keys {
		if err := s.Delete(ctx, key); err != nil {
			return err
		}
	}

	return nil
}

func (s *testStorage) Head(ctx context.Context, key string) (*istorage.Object, error) {
	if s.headErr != nil {
		return nil, s.headErr
	}

	s.mu.RLock()
	obj, ok := s.objects[key]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}

	return &istorage.Object{
		Key:          key,
		Size:         int64(len(obj.data)),
		LastModified: obj.lastUpdated,
		ContentType:  obj.contentType,
		Metadata:     cloneStringMap(obj.metadata),
	}, nil
}

func (s *testStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	if s.updateErr != nil {
		return s.updateErr
	}

	s.mu.Lock()
	obj, ok := s.objects[key]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("object not found: %s", key)
	}

	obj.metadata = cloneStringMap(metadata)
	obj.lastUpdated = time.Now()
	s.objects[key] = obj
	s.mu.Unlock()

	return nil
}

func (s *testStorage) Copy(ctx context.Context, src, dst string) error {
	if s.copyErr != nil {
		return s.copyErr
	}

	s.mu.RLock()
	obj, ok := s.objects[src]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("object not found: %s", src)
	}

	s.mu.Lock()
	s.objects[dst] = storedObject{
		data:        append([]byte(nil), obj.data...),
		metadata:    cloneStringMap(obj.metadata),
		contentType: obj.contentType,
		lastUpdated: time.Now(),
	}
	s.mu.Unlock()

	return nil
}

func (s *testStorage) Move(ctx context.Context, src, dst string) error {
	if s.moveErr != nil {
		return s.moveErr
	}

	if err := s.Copy(ctx, src, dst); err != nil {
		return err
	}
	return s.Delete(ctx, src)
}

func (s *testStorage) Health(ctx context.Context) error {
	return s.healthErr
}

func (s *testStorage) Metrics() *istorage.StorageMetrics {
	return s.metrics
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
