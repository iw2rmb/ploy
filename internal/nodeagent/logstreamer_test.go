package nodeagent

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
)

type capturedLogChunk struct {
	RunID   types.RunID  `json:"run_id"`
	JobID   *types.JobID `json:"job_id,omitempty"`
	ChunkNo int32        `json:"chunk_no"`
	Data    []byte       `json:"data"`
}

type capturedLogUpload struct {
	Payload capturedLogChunk
	Records []logchunk.Record
}

func newLogCaptureServer(t *testing.T) (*httptest.Server, func() []capturedLogUpload) {
	t.Helper()

	var (
		mu      sync.Mutex
		uploads []capturedLogUpload
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/logs") {
			t.Errorf("path = %s, want suffix /logs", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var payload capturedLogChunk
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("unmarshal payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		records, err := logchunk.DecodeGzip(payload.Data)
		if err != nil {
			if !strings.Contains(err.Error(), "log chunk contains no decodable records") {
				t.Errorf("decode framed chunk: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		mu.Lock()
		uploads = append(uploads, capturedLogUpload{Payload: payload, Records: records})
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	snapshot := func() []capturedLogUpload {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]capturedLogUpload, len(uploads))
		copy(cp, uploads)
		return cp
	}
	return server, snapshot
}

func flattenLogRecords(uploads []capturedLogUpload) []logchunk.Record {
	var records []logchunk.Record
	for _, upload := range uploads {
		records = append(records, upload.Records...)
	}
	return records
}

func TestLogStreamer_Write(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantLines []string
	}{
		{
			name:  "empty write",
			input: "",
		},
		{
			name:      "small write",
			input:     "test log line\n",
			wantLines: []string{"test log line"},
		},
		{
			name:      "multiple lines",
			input:     "line 1\nline 2\nline 3\n",
			wantLines: []string{"line 1", "line 2", "line 3"},
		},
	}

	for _, tt := range tests {
		tt := tt // capture loop variable for t.Parallel
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, uploads := newLogCaptureServer(t)
			cfg := newAgentConfig(server.URL)
			runID := types.NewRunID()
			jobID := types.NewJobID()
			ls, err := NewLogStreamer(cfg, runID, jobID, nil)
			if err != nil {
				t.Fatalf("NewLogStreamer() failed: %v", err)
			}
			defer func() { _ = ls.Close() }()

			n, err := ls.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if n != len(tt.input) {
				t.Errorf("Write() wrote %d bytes, want %d", n, len(tt.input))
			}
			if err := ls.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			records := flattenLogRecords(uploads())
			if len(records) != len(tt.wantLines) {
				t.Fatalf("records = %+v, want %d records", records, len(tt.wantLines))
			}
			for i, want := range tt.wantLines {
				if records[i].Stream != logchunk.StreamStdout || records[i].Line != want {
					t.Fatalf("record[%d] = %+v, want stdout %q", i, records[i], want)
				}
			}
		})
	}
}

func TestLogStreamer_Close(t *testing.T) {
	t.Parallel()

	server, uploads := newLogCaptureServer(t)
	cfg := newAgentConfig(server.URL)
	runID := types.NewRunID()
	ls, err := NewLogStreamer(cfg, runID, types.JobID(""), nil)
	if err != nil {
		t.Fatalf("NewLogStreamer() failed: %v", err)
	}

	if _, err := ls.Write([]byte("test log without newline")); err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if err := ls.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}
	if err := ls.Close(); err != nil {
		t.Fatalf("second Close() returned error: %v", err)
	}

	records := flattenLogRecords(uploads())
	if len(records) != 1 {
		t.Fatalf("records = %+v, want one record", records)
	}
	if records[0].Line != "test log without newline" {
		t.Fatalf("record = %+v, want flushed pending line", records[0])
	}
}

// TestLogStreamer_JobIDInPayload verifies that the log streamer includes job_id
// in the payload when a job ID is provided.
//
// The server uses job_id to associate log chunks with specific jobs, enabling
// per-job log retrieval and enrichment.
func TestLogStreamer_JobIDInPayload(t *testing.T) {
	t.Parallel()

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

			server, uploads := newLogCaptureServer(t)

			cfg := newAgentConfig(server.URL)
			ls, err := NewLogStreamer(cfg, runID, tt.jobID, nil)
			if err != nil {
				t.Fatalf("NewLogStreamer() failed: %v", err)
			}

			if _, err := ls.Write([]byte("test log line for job_id verification\n")); err != nil {
				t.Fatalf("Write() failed: %v", err)
			}
			if err := ls.Close(); err != nil {
				t.Fatalf("Close() failed: %v", err)
			}

			receivedPayloads := uploads()
			if len(receivedPayloads) == 0 {
				t.Fatal("expected at least one log chunk payload, got none")
			}

			payload := receivedPayloads[0].Payload

			if tt.wantJobID {
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

			if payload.RunID != runID {
				t.Errorf("run_id = %q, want %q", payload.RunID.String(), runID.String())
			}
		})
	}
}

func TestLogStreamer_PreservesStdoutAndStderrFrames(t *testing.T) {
	t.Parallel()

	server, uploads := newLogCaptureServer(t)

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

	records := flattenLogRecords(uploads())
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

func TestLogStreamer_Hook(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hook LogHook
		want string
	}{
		{
			name: "nil hook writes original input",
			want: "hello",
		},
		{
			name: "custom hook transforms input",
			hook: func(p []byte) ([]byte, error) {
				return []byte(strings.ToUpper(string(p))), nil
			},
			want: "HELLO",
		},
		{
			name: "hook error falls back to original input",
			hook: func([]byte) ([]byte, error) {
				return nil, errors.New("hook processing failed")
			},
			want: "hello",
		},
		{
			name: "nil hook result falls back to original input",
			hook: func([]byte) ([]byte, error) {
				return nil, nil
			},
			want: "hello",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, uploads := newLogCaptureServer(t)
			ls, err := NewLogStreamer(newAgentConfig(server.URL), types.NewRunID(), types.NewJobID(), nil)
			if err != nil {
				t.Fatalf("NewLogStreamer() failed: %v", err)
			}
			if tt.hook != nil {
				ls.SetHook(tt.hook)
			}

			if n, err := ls.Write([]byte("hello\n")); err != nil || n != len("hello\n") {
				t.Fatalf("Write() = %d, %v; want %d, nil", n, err, len("hello\n"))
			}
			if err := ls.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			records := flattenLogRecords(uploads())
			if len(records) != 1 {
				t.Fatalf("records = %+v, want one record", records)
			}
			if records[0].Line != tt.want {
				t.Fatalf("record line = %q, want %q", records[0].Line, tt.want)
			}
		})
	}
}
