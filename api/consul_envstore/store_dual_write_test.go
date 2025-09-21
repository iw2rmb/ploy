package consul_envstore

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	"github.com/stretchr/testify/require"
)

type fakeConsulKV struct {
	mu        sync.Mutex
	data      map[string][]byte
	putErr    error
	getErr    error
	deleteErr error
}

func newFakeConsulKV() *fakeConsulKV {
	return &fakeConsulKV{data: make(map[string][]byte)}
}

func (f *fakeConsulKV) Get(key string, opts *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return nil, nil, f.getErr
	}
	if v, ok := f.data[key]; ok {
		return &api.KVPair{Key: key, Value: append([]byte(nil), v...)}, nil, nil
	}
	return nil, nil, nil
}

func (f *fakeConsulKV) Put(pair *api.KVPair, opts *api.WriteOptions) (*api.WriteMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return nil, f.putErr
	}
	f.data[pair.Key] = append([]byte(nil), pair.Value...)
	return nil, nil
}

func (f *fakeConsulKV) Delete(key string, opts *api.WriteOptions) (*api.WriteMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	delete(f.data, key)
	return nil, nil
}

type fakeSecondaryKV struct {
	mu          sync.Mutex
	data        map[string][]byte
	putErr      error
	deleteErr   error
	putCalls    int
	deleteCalls int
}

func newFakeSecondaryKV() *fakeSecondaryKV {
	return &fakeSecondaryKV{data: make(map[string][]byte)}
}

func (f *fakeSecondaryKV) Put(key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return f.putErr
	}
	f.putCalls++
	f.data[key] = append([]byte(nil), value...)
	return nil
}

func (f *fakeSecondaryKV) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleteCalls++
	delete(f.data, key)
	return nil
}

func (f *fakeSecondaryKV) snapshot(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	return append([]byte(nil), v...), ok
}

type metricsEntry struct {
	target    string
	operation string
	status    string
}

type fakeMetrics struct {
	mu      sync.Mutex
	entries []metricsEntry
}

func (f *fakeMetrics) RecordEnvStoreOperation(target, operation, status string, _ time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, metricsEntry{target: target, operation: operation, status: status})
}

func (f *fakeMetrics) entriesFor(target, operation string) []metricsEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]metricsEntry, 0)
	for _, entry := range f.entries {
		if entry.target == target && entry.operation == operation {
			out = append(out, entry)
		}
	}
	return out
}

func TestConsulEnvStoreDualWriteSuccess(t *testing.T) {
	consulKV := newFakeConsulKV()
	secondary := newFakeSecondaryKV()
	metrics := &fakeMetrics{}

	store, err := New("", "ploy/apps",
		WithKV(consulKV),
		WithSecondary(secondary),
		WithMetrics(metrics),
	)
	require.NoError(t, err)

	payload := envstore.AppEnvVars{"FOO": "bar"}
	err = store.SetAll("demo-app", payload)
	require.NoError(t, err)

	key := store.appEnvKey("demo-app")

	consulPayload := consulKV.data[key]
	require.NotNil(t, consulPayload)

	var stored envstore.AppEnvVars
	require.NoError(t, json.Unmarshal(consulPayload, &stored))
	require.Equal(t, payload, stored)

	secondaryPayload, ok := secondary.snapshot(key)
	require.True(t, ok)
	require.NoError(t, json.Unmarshal(secondaryPayload, &stored))
	require.Equal(t, payload, stored)

	require.Equal(t, 1, secondary.putCalls)

	require.Len(t, metrics.entriesFor("consul", "set"), 1)
	require.Len(t, metrics.entriesFor("jetstream", "set"), 1)

	require.Equal(t, metricsEntry{target: "consul", operation: "set", status: "success"}, metrics.entries[0])
	require.Equal(t, metricsEntry{target: "jetstream", operation: "set", status: "success"}, metrics.entries[1])
}

func TestConsulEnvStoreDualWriteSecondaryFailure(t *testing.T) {
	consulKV := newFakeConsulKV()
	secondary := newFakeSecondaryKV()
	secondary.putErr = errors.New("boom")
	metrics := &fakeMetrics{}

	store, err := New("", "ploy/apps",
		WithKV(consulKV),
		WithSecondary(secondary),
		WithMetrics(metrics),
	)
	require.NoError(t, err)

	payload := envstore.AppEnvVars{"FOO": "bar"}
	err = store.SetAll("demo-app", payload)
	require.NoError(t, err)

	key := store.appEnvKey("demo-app")
	_, ok := secondary.snapshot(key)
	require.False(t, ok)

	entries := metrics.entries
	require.Len(t, entries, 2)
	// First entry is Consul success, second entry should report jetstream failure
	require.Equal(t, metricsEntry{target: "consul", operation: "set", status: "success"}, entries[0])
	require.Equal(t, metricsEntry{target: "jetstream", operation: "set", status: "failure"}, entries[1])
}

func TestConsulEnvStoreDualWriteDelete(t *testing.T) {
	consulKV := newFakeConsulKV()
	secondary := newFakeSecondaryKV()
	metrics := &fakeMetrics{}

	store, err := New("", "ploy/apps",
		WithKV(consulKV),
		WithSecondary(secondary),
		WithMetrics(metrics),
	)
	require.NoError(t, err)

	payload := envstore.AppEnvVars{"FOO": "bar"}
	require.NoError(t, store.SetAll("demo-app", payload))

	require.NoError(t, store.Delete("demo-app", "FOO"))

	key := store.appEnvKey("demo-app")
	// JetStream delete should have been called once
	require.Equal(t, 1, secondary.deleteCalls)
	_, ok := secondary.snapshot(key)
	require.False(t, ok)

	// Metrics should include both the Consul delete and the JetStream shadow delete
	require.Len(t, metrics.entriesFor("consul", "delete"), 1)
	require.Len(t, metrics.entriesFor("jetstream", "delete"), 1)
}
