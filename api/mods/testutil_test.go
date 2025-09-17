package mods

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// kvMem is a lightweight in-memory KV used in tests
type kvMem struct {
	m  map[string][]byte
	mu sync.RWMutex
}

func (k *kvMem) Put(key string, v []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil {
		k.m = map[string][]byte{}
	}
	k.m[key] = append([]byte(nil), v...)
	return nil
}

func (k *kvMem) Get(key string) ([]byte, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.m == nil {
		return nil, nil
	}
	return k.m[key], nil
}

func (k *kvMem) Keys(prefix, _ string) ([]string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	keys := []string{}
	for key := range k.m {
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (k *kvMem) Delete(key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, key)
	return nil
}

// fakeStorage implements internalStorage.Storage for handler tests
// The implementation is intentionally minimal and in-memory.
type fakeStorage struct {
	files   map[string][]byte
	meta    map[string]map[string]string
	metrics *internalStorage.StorageMetrics
	mu      sync.RWMutex
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		files:   map[string][]byte{},
		meta:    map[string]map[string]string{},
		metrics: internalStorage.NewStorageMetrics(),
	}
}

func (f *fakeStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, ok := f.files[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), data...))), nil
}

func (f *fakeStorage) Put(_ context.Context, key string, reader io.Reader, _ ...internalStorage.PutOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	buf, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.files[key] = append([]byte(nil), buf...)
	delete(f.meta, key)
	return nil
}

func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, key)
	delete(f.meta, key)
	return nil
}

func (f *fakeStorage) Exists(_ context.Context, key string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.files[key]
	return ok, nil
}

func (f *fakeStorage) List(_ context.Context, opts internalStorage.ListOptions) ([]internalStorage.Object, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	objects := []internalStorage.Object{}
	for key, data := range f.files {
		if opts.Prefix != "" && !strings.HasPrefix(key, opts.Prefix) {
			continue
		}
		objects = append(objects, internalStorage.Object{Key: key, Size: int64(len(data))})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (f *fakeStorage) DeleteBatch(ctx context.Context, keys []string) error {
	for _, key := range keys {
		if err := f.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeStorage) Head(_ context.Context, key string) (*internalStorage.Object, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, ok := f.files[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	obj := &internalStorage.Object{
		Key:  key,
		Size: int64(len(data)),
	}
	if meta, ok := f.meta[key]; ok {
		obj.Metadata = meta
	}
	return obj, nil
}

func (f *fakeStorage) UpdateMetadata(_ context.Context, key string, metadata map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.files[key]; !ok {
		return fmt.Errorf("not found")
	}
	if metadata == nil {
		delete(f.meta, key)
		return nil
	}
	copyMeta := map[string]string{}
	for k, v := range metadata {
		copyMeta[k] = v
	}
	f.meta[key] = copyMeta
	return nil
}

func (f *fakeStorage) Copy(_ context.Context, src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[src]
	if !ok {
		return fmt.Errorf("not found")
	}
	f.files[dst] = append([]byte(nil), data...)
	if meta, ok := f.meta[src]; ok {
		copyMeta := map[string]string{}
		for k, v := range meta {
			copyMeta[k] = v
		}
		f.meta[dst] = copyMeta
	}
	return nil
}

func (f *fakeStorage) Move(ctx context.Context, src, dst string) error {
	if err := f.Copy(ctx, src, dst); err != nil {
		return err
	}
	return f.Delete(ctx, src)
}

func (f *fakeStorage) Health(context.Context) error { return nil }

func (f *fakeStorage) Metrics() *internalStorage.StorageMetrics { return f.metrics }
