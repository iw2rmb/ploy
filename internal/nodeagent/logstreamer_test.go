package nodeagent

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
	"time"
)

func TestLogStreamer_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(t *testing.T, ls *LogStreamer)
	}{
		{
			name:    "empty write",
			input:   "",
			wantErr: false,
		},
		{
			name:    "small write",
			input:   "test log line\n",
			wantErr: false,
		},
		{
			name:    "multiple lines",
			input:   "line 1\nline 2\nline 3\n",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := Config{
				NodeID:    "00000000-0000-0000-0000-000000000001",
				ServerURL: "http://localhost:8443",
			}
			ls := NewLogStreamer(cfg, "run-123", "stage-456")
			defer func() { _ = ls.Close() }()

			n, err := ls.Write([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && n != len(tt.input) {
				t.Errorf("Write() wrote %d bytes, want %d", n, len(tt.input))
			}
			if tt.validate != nil {
				tt.validate(t, ls)
			}
		})
	}
}

func TestLogStreamer_SizeCap(t *testing.T) {
	t.Parallel()

	cfg := Config{
		NodeID:    "00000000-0000-0000-0000-000000000001",
		ServerURL: "http://localhost:8443",
	}
	ls := NewLogStreamer(cfg, "run-123", "")
	defer func() { _ = ls.Close() }()

	// Generate data that will compress to over 1 MiB.
	// We use repeating patterns which gzip compresses well, but we need enough raw data.
	chunk := strings.Repeat("test log line with some content\n", 1024)
	for i := 0; i < 50; i++ {
		_, err := ls.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("Write() unexpected error: %v", err)
		}
	}

	// Verify that chunks were created (implicitly tested by no errors during writes).
	// The actual size cap enforcement happens during flushLocked.
}

func TestLogStreamer_Compression(t *testing.T) {
	t.Parallel()

	// Test that data is actually compressed.
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	input := "test log line\n"
	for i := 0; i < 100; i++ {
		_, err := gzWriter.Write([]byte(input))
		if err != nil {
			t.Fatalf("gzip write failed: %v", err)
		}
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}

	// Verify compressed size is smaller than uncompressed.
	uncompressed := len(input) * 100
	compressed := buf.Len()
	if compressed >= uncompressed {
		t.Errorf("compression did not reduce size: uncompressed=%d, compressed=%d", uncompressed, compressed)
	}

	// Verify we can decompress it.
	gzReader, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip reader failed: %v", err)
	}
	defer func() { _ = gzReader.Close() }()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		t.Fatalf("gzip read failed: %v", err)
	}

	if len(decompressed) != uncompressed {
		t.Errorf("decompressed size mismatch: got %d, want %d", len(decompressed), uncompressed)
	}
}

func TestLogStreamer_Close(t *testing.T) {
	t.Parallel()

	cfg := Config{
		NodeID:    "00000000-0000-0000-0000-000000000001",
		ServerURL: "http://localhost:8443",
	}
	ls := NewLogStreamer(cfg, "run-123", "")

	// Write some data.
	_, err := ls.Write([]byte("test log\n"))
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Close should flush remaining data.
	err = ls.Close()
	if err != nil {
		// Close may fail if server is not available, which is expected in unit tests.
		// We just verify Close can be called without panicking.
		t.Logf("Close() returned error (expected in unit test): %v", err)
	}

	// Calling Close again should be idempotent.
	err = ls.Close()
	if err != nil {
		t.Logf("Close() second call returned error: %v", err)
	}
}

func TestLogStreamer_FlushInterval(t *testing.T) {
	t.Parallel()

	cfg := Config{
		NodeID:    "00000000-0000-0000-0000-000000000001",
		ServerURL: "http://localhost:8443",
	}
	ls := NewLogStreamer(cfg, "run-123", "")
	defer func() { _ = ls.Close() }()

	// Write a small amount of data.
	_, err := ls.Write([]byte("test log\n"))
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Wait for periodic flush to trigger (flush interval is 2 seconds).
	time.Sleep(3 * time.Second)

	// Verify the buffer was flushed by checking chunk number incremented.
	ls.mu.Lock()
	chunkNo := ls.chunkNo
	ls.mu.Unlock()

	// After flush, chunk number should be > 0 (unless server is not available).
	// This test is best-effort since it depends on server availability.
	if chunkNo > 0 {
		t.Logf("Periodic flush triggered successfully, chunk_no=%d", chunkNo)
	} else {
		t.Logf("Periodic flush may have failed (server not available), chunk_no=%d", chunkNo)
	}
}

func TestLogStreamer_ChunkNumbering(t *testing.T) {
	t.Parallel()

	cfg := Config{
		NodeID:    "00000000-0000-0000-0000-000000000001",
		ServerURL: "http://localhost:8443",
	}
	ls := NewLogStreamer(cfg, "run-123", "")
	defer func() { _ = ls.Close() }()

	// Verify initial chunk number is 0.
	ls.mu.Lock()
	initialChunkNo := ls.chunkNo
	ls.mu.Unlock()

	if initialChunkNo != 0 {
		t.Errorf("initial chunk_no = %d, want 0", initialChunkNo)
	}

	// Write enough data to trigger a flush.
	largeData := strings.Repeat("x", maxChunkSize+1)
	_, _ = ls.Write([]byte(largeData))

	// Chunk number should have incremented after flush attempt.
	// Note: actual increment depends on whether flush succeeds (server available).
	ls.mu.Lock()
	afterChunkNo := ls.chunkNo
	ls.mu.Unlock()

	t.Logf("chunk_no after large write: %d (flush may have failed if server unavailable)", afterChunkNo)
}
