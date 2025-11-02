package nodeagent

import (
	"bytes"
	"errors"
	"testing"
)

func TestNoOpLogHook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "empty input",
			input: []byte{},
		},
		{
			name:  "simple text",
			input: []byte("hello world"),
		},
		{
			name:  "multiline with newlines",
			input: []byte("line1\nline2\nline3"),
		},
		{
			name:  "binary data",
			input: []byte{0x00, 0xFF, 0x01, 0xAB},
		},
		{
			name:  "large input",
			input: bytes.Repeat([]byte("test "), 1000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hook := &NoOpLogHook{}
			result, err := hook.Process(tt.input)

			if err != nil {
				t.Errorf("Process() returned unexpected error: %v", err)
			}

			// NoOpLogHook should return the exact same slice (identity).
			if len(tt.input) > 0 && len(result) > 0 {
				if &result[0] != &tt.input[0] {
					t.Error("Process() should return the same slice pointer")
				}
			}

			if !bytes.Equal(result, tt.input) {
				t.Errorf("Process() = %v, want %v", result, tt.input)
			}
		})
	}
}

func TestNoOpLogHook_Concurrent(t *testing.T) {
	t.Parallel()

	hook := &NoOpLogHook{}
	input := []byte("concurrent test data")

	// Run multiple goroutines to verify thread safety.
	const goroutines = 10
	const iterations = 100

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				result, err := hook.Process(input)
				if err != nil {
					t.Errorf("Process() error in goroutine: %v", err)
				}
				if !bytes.Equal(result, input) {
					t.Errorf("Process() concurrent mismatch")
				}
			}
			done <- true
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// mockHook is a test hook that transforms data.
type mockHook struct {
	transform func([]byte) ([]byte, error)
}

func (h *mockHook) Process(p []byte) ([]byte, error) {
	return h.transform(p)
}

func TestLogStreamer_WithCustomHook(t *testing.T) {
	t.Parallel()

	// Create a mock hook that uppercases input.
	upperHook := &mockHook{
		transform: func(p []byte) ([]byte, error) {
			result := make([]byte, len(p))
			for i, b := range p {
				if b >= 'a' && b <= 'z' {
					result[i] = b - ('a' - 'A')
				} else {
					result[i] = b
				}
			}
			return result, nil
		},
	}

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "00000000-0000-0000-0000-000000000001",
	}

	ls := NewLogStreamer(cfg, "run-123", "")
	ls.SetHook(upperHook)
	defer func() { _ = ls.Close() }()

	// Write some lowercase text.
	input := []byte("hello world")
	n, err := ls.Write(input)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d bytes, want %d", n, len(input))
	}

	// We can't directly verify the gzipped buffer content easily,
	// but we've tested that the hook was invoked without error.
	// Integration tests would verify end-to-end behavior.
}

func TestLogStreamer_HookError(t *testing.T) {
	t.Parallel()

	// Create a hook that always fails.
	errorHook := &mockHook{
		transform: func(p []byte) ([]byte, error) {
			return nil, errors.New("hook processing failed")
		},
	}

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "00000000-0000-0000-0000-000000000001",
	}

	ls := NewLogStreamer(cfg, "run-456", "")
	ls.SetHook(errorHook)
	defer func() { _ = ls.Close() }()

	// Write should succeed even though hook fails (fallback behavior).
	input := []byte("test data")
	n, err := ls.Write(input)
	if err != nil {
		t.Fatalf("Write() should succeed despite hook error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d bytes, want %d", n, len(input))
	}

	// The original data should have been written (not nil from hook).
	// We verify this indirectly by confirming Write succeeded.
}

func TestLogStreamer_DefaultHook(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "00000000-0000-0000-0000-000000000001",
	}

	ls := NewLogStreamer(cfg, "run-789", "")
	defer func() { _ = ls.Close() }()

	// Verify default hook is NoOpLogHook.
	if ls.hook == nil {
		t.Fatal("LogStreamer.hook should not be nil")
	}

	// Type assertion to verify it's NoOpLogHook.
	if _, ok := ls.hook.(*NoOpLogHook); !ok {
		t.Errorf("Default hook type = %T, want *NoOpLogHook", ls.hook)
	}

	// Write should work with default hook.
	input := []byte("default hook test")
	n, err := ls.Write(input)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d bytes, want %d", n, len(input))
	}
}

func TestLogStreamer_SetHook(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "00000000-0000-0000-0000-000000000001",
	}

	ls := NewLogStreamer(cfg, "run-abc", "")
	defer func() { _ = ls.Close() }()

	// Create a custom hook.
	customHook := &mockHook{
		transform: func(p []byte) ([]byte, error) {
			return append([]byte("[PREFIX] "), p...), nil
		},
	}

	// Set the hook.
	ls.SetHook(customHook)

	// Verify hook was set.
	if ls.hook != customHook {
		t.Error("SetHook() did not set the hook correctly")
	}

	// Write should use the custom hook.
	input := []byte("test")
	n, err := ls.Write(input)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d bytes, want %d", n, len(input))
	}
}

// BenchmarkNoOpLogHook measures the performance of NoOpLogHook.
func BenchmarkNoOpLogHook(b *testing.B) {
	hook := &NoOpLogHook{}
	input := []byte("benchmark test data with some length to it")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = hook.Process(input)
	}
}

// BenchmarkLogStreamer_WithNoOpHook measures write performance with default hook.
func BenchmarkLogStreamer_WithNoOpHook(b *testing.B) {
	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "00000000-0000-0000-0000-000000000001",
	}

	ls := NewLogStreamer(cfg, "bench-run", "")
	defer func() { _ = ls.Close() }()

	input := []byte("benchmark log line\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ls.Write(input)
	}
}
