package nodeagent

import (
	"errors"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestNilLogHook(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "aB3xY9",
	}

	ls, err := NewLogStreamer(cfg, types.NewRunID(), types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
	defer func() { _ = ls.Close() }()

	// Default hook is nil (no-op).
	if ls.hook != nil {
		t.Fatal("LogStreamer.hook should be nil by default")
	}

	// Write should work with nil hook.
	input := []byte("default hook test")
	n, err := ls.Write(input)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write() returned %d bytes, want %d", n, len(input))
	}
}

func TestLogStreamer_WithCustomHook(t *testing.T) {
	t.Parallel()

	// Create a hook function that uppercases input.
	upperHook := LogHook(func(p []byte) ([]byte, error) {
		result := make([]byte, len(p))
		for i, b := range p {
			if b >= 'a' && b <= 'z' {
				result[i] = b - ('a' - 'A')
			} else {
				result[i] = b
			}
		}
		return result, nil
	})

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "aB3xY9",
	}

	ls, err := NewLogStreamer(cfg, types.NewRunID(), types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
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
}

func TestLogStreamer_HookError(t *testing.T) {
	t.Parallel()

	// Create a hook that always fails.
	errorHook := LogHook(func(p []byte) ([]byte, error) {
		return nil, errors.New("hook processing failed")
	})

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "aB3xY9",
	}

	ls, err := NewLogStreamer(cfg, types.NewRunID(), types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
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
}

func TestLogStreamer_HookReturnsNil(t *testing.T) {
	t.Parallel()

	// Hook returns nil slice with no error; streamer should fall back to original input.
	nilHook := LogHook(func(p []byte) ([]byte, error) {
		return nil, nil
	})

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "aB3xY9",
	}

	ls, err := NewLogStreamer(cfg, types.NewRunID(), types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
	ls.SetHook(nilHook)
	defer func() { _ = ls.Close() }()

	input := []byte("some data that should still be written")
	n, err := ls.Write(input)
	if err != nil {
		t.Fatalf("Write() returned error: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write() returned %d bytes, want %d", n, len(input))
	}
}

func TestLogStreamer_SetHook(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "aB3xY9",
	}

	ls, err := NewLogStreamer(cfg, types.NewRunID(), types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
	defer func() { _ = ls.Close() }()

	// Set a custom hook.
	ls.SetHook(func(p []byte) ([]byte, error) {
		return append([]byte("[PREFIX] "), p...), nil
	})

	// Verify hook was set.
	if ls.hook == nil {
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

// BenchmarkLogHook measures the overhead of calling a LogHook function.
func BenchmarkLogHook(b *testing.B) {
	hook := LogHook(func(p []byte) ([]byte, error) { return p, nil })
	input := []byte("benchmark test data with some length to it")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = hook(input)
	}
}

// BenchmarkLogStreamer_WithNilHook measures write performance with nil (no-op) hook.
func BenchmarkLogStreamer_WithNilHook(b *testing.B) {
	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    "aB3xY9",
	}

	ls, err := NewLogStreamer(cfg, types.NewRunID(), types.JobID(""), nil)
	if err != nil {
		b.Fatalf("NewLogStreamer() failed: %v", err)
	}
	defer func() { _ = ls.Close() }()

	input := []byte("benchmark log line\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ls.Write(input)
	}
}
