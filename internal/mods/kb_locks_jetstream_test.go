package mods

import (
	"encoding/json"
	"testing"
	"time"
)

func TestJetstreamLockData_Marshal(t *testing.T) {
	lockData := &JetstreamLockData{
		SessionID:  "test-session",
		Holder:     "test-holder",
		TTL:        300,
		AcquiredAt: time.Now(),
	}

	data, err := json.Marshal(lockData)
	if err != nil {
		t.Fatalf("Failed to marshal lock data: %v", err)
	}

	var unmarshaled JetstreamLockData
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal lock data: %v", err)
	}

	if unmarshaled.SessionID != lockData.SessionID {
		t.Errorf("Expected session ID %q, got %q", lockData.SessionID, unmarshaled.SessionID)
	}

	if unmarshaled.Holder != lockData.Holder {
		t.Errorf("Expected holder %q, got %q", lockData.Holder, unmarshaled.Holder)
	}

	if unmarshaled.TTL != lockData.TTL {
		t.Errorf("Expected TTL %d, got %d", lockData.TTL, unmarshaled.TTL)
	}
}

func TestJetstreamKBLockManager_buildLockKey(t *testing.T) {
	mgr := &JetstreamKBLockManager{}

	key := mgr.buildLockKey("test-key")
	expected := "kb/locks/test-key"

	if key != expected {
		t.Errorf("Expected lock key %q, got %q", expected, key)
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		expect bool
	}{
		{"hello world", "world", true},
		{"hello world", "foo", false},
		{"lock already held", "already held", true},
		{"", "test", false},
		{"test", "", true},
	}

	for _, test := range tests {
		result := containsString(test.s, test.substr)
		if result != test.expect {
			t.Errorf("containsString(%q, %q) = %v, expected %v", test.s, test.substr, result, test.expect)
		}
	}
}

// Test that the default lock config works
func TestDefaultLockConfig(t *testing.T) {
	config := DefaultLockConfig()

	if config.DefaultTTL != 5*time.Second {
		t.Errorf("Expected DefaultTTL to be 5s, got %v", config.DefaultTTL)
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries to be 3, got %d", config.MaxRetries)
	}

	if config.RetryInterval != 100*time.Millisecond {
		t.Errorf("Expected RetryInterval to be 100ms, got %v", config.RetryInterval)
	}
}

// Test signature lock key building
func TestBuildSignatureLockKey_WithJetStream(t *testing.T) {
	key := BuildSignatureLockKey("java", "abcd1234")
	expected := "java/abcd1234"

	if key != expected {
		t.Errorf("Expected %q, got %q", expected, key)
	}

	// Test with jetstream manager
	mgr := &JetstreamKBLockManager{}
	fullKey := mgr.buildLockKey(key)
	expectedFull := "kb/locks/java/abcd1234"

	if fullKey != expectedFull {
		t.Errorf("Expected full key %q, got %q", expectedFull, fullKey)
	}
}
