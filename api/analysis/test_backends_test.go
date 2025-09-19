package analysis

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	istorage "github.com/iw2rmb/ploy/internal/storage"
)

func TestFakeKVBasicOperations(t *testing.T) {
	t.Parallel()

	kv := newTestKV()

	key := "ploy/analysis/jobs/job-123"
	value := []byte("payload")
	if err := kv.Put(key, value); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	// Mutate original slice to ensure fake stores a copy
	value[0] = 'X'

	got, err := kv.Get(key)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("Get returned %q, want %q", got, "payload")
	}

	keys, err := kv.Keys("ploy/analysis/jobs/", "/")
	if err != nil {
		t.Fatalf("Keys returned error: %v", err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("Keys returned %v, want [%s]", keys, key)
	}

	if err := kv.Delete(key); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	afterDelete, err := kv.Get(key)
	if err != nil {
		t.Fatalf("Get after delete returned error: %v", err)
	}
	if afterDelete != nil {
		t.Fatalf("Get after delete returned %v, want nil", afterDelete)
	}
}

func TestFakeKVConcurrentAccess(t *testing.T) {
	t.Parallel()

	kv := newTestKV()

	const routines = 64
	var wg sync.WaitGroup
	wg.Add(routines)

	for i := 0; i < routines; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("ploy/analysis/jobs/job-%d", i%8)
			payload := []byte(fmt.Sprintf("value-%d", i))
			if err := kv.Put(key, payload); err != nil {
				t.Errorf("Put: %v", err)
				return
			}
			got, err := kv.Get(key)
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			if got == nil {
				t.Errorf("Get returned nil for key %s", key)
			}
		}()
	}

	wg.Wait()

	keys, err := kv.Keys("ploy/analysis/jobs/", "/")
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatalf("Keys returned empty slice")
	}

	for _, key := range keys {
		if err := kv.Delete(key); err != nil {
			t.Fatalf("Delete %s: %v", key, err)
		}
	}
}

func TestFakeStorageBasicOperations(t *testing.T) {
	t.Parallel()

	storage := newTestStorage()
	ctx := context.Background()

	key := "analysis/inputs/job-123.tar.gz"
	body := []byte("test artifact")

	if err := storage.Put(ctx, key, bytes.NewReader(body)); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	reader, err := storage.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(data) != string(body) {
		t.Fatalf("Get returned %q, want %q", data, body)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	exists, err := storage.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists returned error: %v", err)
	}
	if !exists {
		t.Fatalf("Exists returned false, want true")
	}

	objs, err := storage.List(ctx, istorage.ListOptions{Prefix: "analysis/inputs/"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(objs) != 1 || objs[0].Key != key {
		t.Fatalf("List returned %#v", objs)
	}

	if err := storage.Delete(ctx, key); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	exists, err = storage.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists after delete returned error: %v", err)
	}
	if exists {
		t.Fatalf("Exists returned true after delete")
	}
}

func TestFakeStorageConcurrentAccess(t *testing.T) {
	t.Parallel()

	storage := newTestStorage()
	ctx := context.Background()

	const routines = 48
	var wg sync.WaitGroup
	wg.Add(routines)

	for i := 0; i < routines; i++ {
		i := i
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("analysis/outputs/job-%d.json", i%6)
			payload := []byte(fmt.Sprintf(`{"value":%d}`, i))
			if err := storage.Put(ctx, key, bytes.NewReader(payload)); err != nil {
				t.Errorf("Put: %v", err)
				return
			}
			reader, err := storage.Get(ctx, key)
			if err != nil {
				t.Errorf("Get: %v", err)
				return
			}
			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Errorf("Read: %v", err)
			}
			_ = reader.Close()
		}()
	}

	wg.Wait()

	objs, err := storage.List(ctx, istorage.ListOptions{Prefix: "analysis/outputs/"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objs) == 0 {
		t.Fatalf("List returned no objects")
	}

	keys := make([]string, len(objs))
	for i, obj := range objs {
		keys[i] = obj.Key
	}
	if err := storage.DeleteBatch(ctx, keys); err != nil {
		t.Fatalf("DeleteBatch: %v", err)
	}
}
