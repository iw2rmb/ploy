package mods

import "testing"

// MockKV implements orchestration.KV for testing
type MockKV struct{}

func (m *MockKV) Put(key string, value []byte) error              { return nil }
func (m *MockKV) Get(key string) ([]byte, error)                  { return nil, nil }
func (m *MockKV) Keys(prefix, separator string) ([]string, error) { return nil, nil }
func (m *MockKV) Delete(key string) error                         { return nil }

func TestNewKBLockManager_DefaultsToJetStream(t *testing.T) {
	_, url := runTestJetStream(t)
	setJetStreamEnv(t, url)

	kv := &MockKV{}
	mgr := NewKBLockManager(kv)

	if _, ok := mgr.(*JetstreamKBLockManager); !ok {
		t.Fatalf("expected JetstreamKBLockManager by default, got %T", mgr)
	}
}

func TestNewKBLockManagerFallsBackToConsulWhenJetStreamUnavailable(t *testing.T) {
	kv := &MockKV{}
	mgr := NewKBLockManager(kv)

	if _, ok := mgr.(*ConsulKBLockManager); !ok {
		t.Fatalf("expected ConsulKBLockManager when JetStream is unavailable, got %T", mgr)
	}
}

func TestBuildSignatureLockKey_Compatibility(t *testing.T) {
	// Ensure the lock key builder is compatible with both implementations
	key := BuildSignatureLockKey("java", "abcd1234")
	expected := "java/abcd1234"

	if key != expected {
		t.Errorf("Expected %q, got %q", expected, key)
	}

	// Test with JetStream manager
	mgr := &JetstreamKBLockManager{}
	fullKey := mgr.buildLockKey(key)
	expectedFull := "writers/java/abcd1234"

	if fullKey != expectedFull {
		t.Errorf("Expected full key %q, got %q", expectedFull, fullKey)
	}
}
