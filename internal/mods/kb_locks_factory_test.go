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

func TestNewKBLockManager_DefaultsToJetStream(t *testing.T) {
	_ = os.Unsetenv("PLOY_USE_JETSTREAM_KV")

	_, url := runTestJetStream(t)
	setJetStreamEnv(t, url)

	kv := &MockKV{}
	mgr := NewKBLockManager(kv)

	if _, ok := mgr.(*JetstreamKBLockManager); !ok {
		t.Fatalf("expected JetstreamKBLockManager by default, got %T", mgr)
	}
}

func TestNewKBLockManager_IgnoresDisableFlag(t *testing.T) {
	_ = os.Setenv("PLOY_USE_JETSTREAM_KV", "false")
	defer func() { _ = os.Unsetenv("PLOY_USE_JETSTREAM_KV") }()

	_, url := runTestJetStream(t)
	setJetStreamEnv(t, url)

	kv := &MockKV{}
	mgr := NewKBLockManager(kv)

	if _, ok := mgr.(*JetstreamKBLockManager); !ok {
		t.Fatalf("expected JetstreamKBLockManager even when flag disables, got %T", mgr)
	}
}

func TestUseJetstreamKV(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"", true},
		{"false", true},
		{"0", true},
		{"no", true},
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
	expectedFull := "writers/java/abcd1234"

	if fullKey != expectedFull {
		t.Errorf("Expected full key %q, got %q", expectedFull, fullKey)
	}
}
