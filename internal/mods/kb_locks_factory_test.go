package mods

import (
	"os"
	"testing"
)

// MockKV implements orchestration.KV for testing
type MockKV struct{}

func (m *MockKV) Put(key string, value []byte) error              { return nil }
func (m *MockKV) Get(key string) ([]byte, error)                  { return nil, nil }
func (m *MockKV) Keys(prefix, separator string) ([]string, error) { return nil, nil }
func (m *MockKV) Delete(key string) error                         { return nil }

func TestNewKBLockManager_DefaultsToConsul(t *testing.T) {
	// Ensure env var is not set
	_ = os.Unsetenv("PLOY_USE_JETSTREAM_KV")

	kv := &MockKV{}
	mgr := NewKBLockManager(kv)

	// Should return Consul lock manager
	if _, ok := mgr.(*ConsulKBLockManager); !ok {
		t.Errorf("Expected ConsulKBLockManager, got %T", mgr)
	}
}

func TestNewKBLockManager_JetStreamWithEnvVar(t *testing.T) {
	// Set env var to enable JetStream
	_ = os.Setenv("PLOY_USE_JETSTREAM_KV", "true")
	defer func() { _ = os.Unsetenv("PLOY_USE_JETSTREAM_KV") }()

	// Force connection failure by setting invalid NATS address
	_ = os.Setenv("NATS_ADDR", "nats://invalid:4222")
	defer func() { _ = os.Unsetenv("NATS_ADDR") }()

	// JetStream will fail to connect, so should fall back to Consul
	kv := &MockKV{}
	mgr := NewKBLockManager(kv)

	// Should fall back to Consul due to connection failure
	if _, ok := mgr.(*ConsulKBLockManager); !ok {
		t.Errorf("Expected fallback to ConsulKBLockManager, got %T", mgr)
	}
}

func TestUseJetstreamKV(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"", false},
		{"false", false},
		{"0", false},
		{"no", false},
		{"true", true},
		{"1", true},
		{"yes", true},
		{"on", true},
		{"TRUE", true},
		{"YES", true},
		{"ON", true},
	}

	for _, test := range tests {
		t.Run(test.envValue, func(t *testing.T) {
			if test.envValue == "" {
				_ = os.Unsetenv("PLOY_USE_JETSTREAM_KV")
			} else {
				_ = os.Setenv("PLOY_USE_JETSTREAM_KV", test.envValue)
			}
			defer func() { _ = os.Unsetenv("PLOY_USE_JETSTREAM_KV") }()

			result := useJetstreamKV()
			if result != test.expected {
				t.Errorf("For env value %q, expected %v, got %v", test.envValue, test.expected, result)
			}
		})
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
	expectedFull := "kb/locks/java/abcd1234"

	if fullKey != expectedFull {
		t.Errorf("Expected full key %q, got %q", expectedFull, fullKey)
	}
}
