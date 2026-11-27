package nodeagent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
)

type capturedLogChunk struct {
	ResumeCount int    `json:"resume_count"`
	ChunkNo     *int32 `json:"chunk_no"`
	Data        []byte `json:"data"`
}

type capturedLogUpload struct {
	Payload capturedLogChunk
	Records []logchunk.Record
}

func newLogCaptureServer(t *testing.T, jobID types.JobID) (*httptest.Server, func() []capturedLogUpload) {
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
		if want := "/v1/jobs/" + jobID.String() + "/logs"; r.URL.Path != want {
			t.Errorf("path = %s, want %s", r.URL.Path, want)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var payload capturedLogChunk
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
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

			jobID := types.NewJobID()
			server, uploads := newLogCaptureServer(t, jobID)
			cfg := newAgentConfig(server.URL)
			runID := types.NewRunID()
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

	jobID := types.NewJobID()
	server, uploads := newLogCaptureServer(t, jobID)
	cfg := newAgentConfig(server.URL)
	runID := types.NewRunID()
	ls, err := NewLogStreamer(cfg, runID, jobID, nil)
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

// Both initial and resumed executions upload chunks with their current resume count.
func TestLogStreamer_UploadsForJobExecution(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		resumeCount int
	}{{"initial", 0}, {"resumed", 4}} {
		t.Run(tt.name, func(t *testing.T) {
			jobID := types.NewJobID()
			server, uploads := newLogCaptureServer(t, jobID)
			ls, err := NewLogStreamer(newAgentConfig(server.URL), types.NewRunID(), jobID, server.Client(), tt.resumeCount)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ls.Write([]byte("job log\n")); err != nil {
				t.Fatal(err)
			}
			if err := ls.Close(); err != nil {
				t.Fatal(err)
			}
			got := uploads()
			if len(got) != 1 {
				t.Fatalf("uploads = %d, want 1", len(got))
			}
			payload := got[0].Payload
			if payload.ResumeCount != tt.resumeCount {
				t.Errorf("resume_count = %d, want %d", payload.ResumeCount, tt.resumeCount)
			}
			if payload.ChunkNo == nil || *payload.ChunkNo != 0 {
				t.Error("first upload must include chunk_no=0")
			}
		})
	}
}

func TestLogStreamer_PreservesStdoutAndStderrFrames(t *testing.T) {
	t.Parallel()

	jobID := types.NewJobID()
	server, uploads := newLogCaptureServer(t, jobID)

	ls, err := NewLogStreamer(newAgentConfig(server.URL), types.NewRunID(), jobID, nil)
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

			jobID := types.NewJobID()
			server, uploads := newLogCaptureServer(t, jobID)
			ls, err := NewLogStreamer(newAgentConfig(server.URL), types.NewRunID(), jobID, nil)
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
