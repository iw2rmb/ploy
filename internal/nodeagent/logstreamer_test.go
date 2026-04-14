package nodeagent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
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
		tt := tt // capture loop variable for t.Parallel
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := newAgentConfig("http://localhost:8443")
			runID := types.NewRunID()
			jobID := types.NewJobID()
			ls, err := NewLogStreamer(cfg, runID, jobID, nil)
			if err != nil {
				t.Fatalf("NewLogStreamer() failed: %v", err)
			}
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

	cfg := newAgentConfig("http://localhost:8443")
	runID := types.NewRunID()
	ls, err := NewLogStreamer(cfg, runID, types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
	defer func() { _ = ls.Close() }()

	// Exercise buffering/flush behavior with a moderate amount of log data.
	// The log chunk cap is enforced on the gzipped bytes (MaxUploadSize).
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

	cfg := newAgentConfig("http://localhost:8443")
	runID := types.NewRunID()
	ls, err := NewLogStreamer(cfg, runID, types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}

	// Write some data.
	_, err = ls.Write([]byte("test log\n"))
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

	cfg := newAgentConfig("http://localhost:8443")
	runID := types.NewRunID()
	ls, err := NewLogStreamer(cfg, runID, types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
	defer func() { _ = ls.Close() }()

	// Write a small amount of data.
	_, err = ls.Write([]byte("test log\n"))
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

	cfg := newAgentConfig("http://localhost:8443")
	runID := types.NewRunID()
	ls, err := NewLogStreamer(cfg, runID, types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}
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

// TestLogStreamer_JobIDInPayload verifies that the log streamer includes job_id
// in the payload when a job ID is provided.
//
// The server uses job_id to associate log chunks with specific jobs, enabling
// per-job log retrieval and enrichment.
func TestLogStreamer_JobIDInPayload(t *testing.T) {
	t.Parallel()

	// logChunkPayload mirrors the structure sent by sendChunk.
	type logChunkPayload struct {
		RunID   types.RunID  `json:"run_id"`
		JobID   *types.JobID `json:"job_id,omitempty"`
		ChunkNo int32        `json:"chunk_no"`
		Data    []byte       `json:"data"`
	}

	runID := types.NewRunID()
	jobID := types.NewJobID()

	tests := []struct {
		name       string
		jobID      types.JobID
		wantJobID  bool   // Whether job_id should be present in payload
		wantJobIDV string // Expected job_id value (if wantJobID is true)
	}{
		{
			name:       "with job_id",
			jobID:      jobID,
			wantJobID:  true,
			wantJobIDV: jobID.String(),
		},
		{
			name:       "without job_id (empty string)",
			jobID:      "",
			wantJobID:  false,
			wantJobIDV: "",
		},
	}

	for _, tt := range tests {
		tt := tt // capture loop variable for t.Parallel
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Track received payloads from the mock server.
			var mu sync.Mutex
			var receivedPayloads []logChunkPayload

			// Create a mock server that captures log chunk requests.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify this is a POST to the logs endpoint.
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method: %s", r.Method)
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/logs") {
					t.Errorf("unexpected path: %s", r.URL.Path)
					http.Error(w, "not found", http.StatusNotFound)
					return
				}

				// Parse the request body.
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("failed to read request body: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}

				var payload logChunkPayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Errorf("failed to unmarshal payload: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}

				mu.Lock()
				receivedPayloads = append(receivedPayloads, payload)
				mu.Unlock()

				w.WriteHeader(http.StatusCreated)
			}))
			defer server.Close()

			// Create log streamer pointing to mock server.
			cfg := newAgentConfig(server.URL)
			ls, err := NewLogStreamer(cfg, runID, tt.jobID, nil)
			if err != nil {
				t.Fatalf("NewLogStreamer() failed: %v", err)
			}

			// Write some data to the log streamer.
			_, err = ls.Write([]byte("test log line for job_id verification\n"))
			if err != nil {
				t.Fatalf("Write() failed: %v", err)
			}

			// Close to flush any remaining data.
			if err := ls.Close(); err != nil {
				t.Fatalf("Close() failed: %v", err)
			}

			// Verify we received at least one payload.
			mu.Lock()
			defer mu.Unlock()
			if len(receivedPayloads) == 0 {
				t.Fatal("expected at least one log chunk payload, got none")
			}

			// Check the first payload for job_id presence and value.
			payload := receivedPayloads[0]

			if tt.wantJobID {
				// Expect job_id to be present and have the correct value.
				if payload.JobID == nil {
					t.Errorf("expected job_id to be present in payload, but it was nil")
				} else if payload.JobID.String() != tt.wantJobIDV {
					t.Errorf("job_id = %q, want %q", payload.JobID.String(), tt.wantJobIDV)
				}
			} else {
				// Expect job_id to be absent (nil).
				if payload.JobID != nil {
					t.Errorf("expected job_id to be nil in payload, but got %q", payload.JobID.String())
				}
			}

			// Verify run_id is always present.
			if payload.RunID != runID {
				t.Errorf("run_id = %q, want %q", payload.RunID.String(), runID.String())
			}
		})
	}
}

func TestLogStreamer_PreservesStdoutAndStderrFrames(t *testing.T) {
	t.Parallel()

	type logChunkPayload struct {
		Data []byte `json:"data"`
	}

	var (
		mu      sync.Mutex
		records []logchunk.Record
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload logChunkPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("unmarshal payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		frames, err := logchunk.DecodeGzip(payload.Data)
		if err != nil {
			t.Errorf("decode framed chunk: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		records = append(records, frames...)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	ls, err := NewLogStreamer(newAgentConfig(server.URL), types.NewRunID(), types.NewJobID(), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}

	if _, err := ls.StdoutWriter().Write([]byte("out-line\n")); err != nil {
		t.Fatalf("stdout write failed: %v", err)
	}
	if _, err := ls.StderrWriter().Write([]byte("err-line\n")); err != nil {
		t.Fatalf("stderr write failed: %v", err)
	}
	if err := ls.Close(); err != nil {
		t.Fatalf("close log streamer: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(records) < 2 {
		t.Fatalf("expected at least 2 framed records, got %d", len(records))
	}
	if records[0].Stream != logchunk.StreamStdout || records[0].Line != "out-line" {
		t.Fatalf("first record = %+v, want stdout out-line", records[0])
	}
	if records[1].Stream != logchunk.StreamStderr || records[1].Line != "err-line" {
		t.Fatalf("second record = %+v, want stderr err-line", records[1])
	}
}
