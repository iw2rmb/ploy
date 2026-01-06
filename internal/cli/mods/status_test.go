package mods

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestListRunRepoDiffsCommand_Success(t *testing.T) {
	type testDiff struct {
		ID        string                 `json:"id"`
		JobID     string                 `json:"job_id"`
		CreatedAt string                 `json:"created_at"`
		Size      int                    `json:"gzipped_size"`
		Summary   map[string]interface{} `json:"summary,omitempty"`
	}

	diffs := []testDiff{
		{ID: "diff-1", JobID: "job-1", Size: 200, Summary: map[string]interface{}{"step_index": 1000, "mod_type": "mod"}},
		{ID: "diff-2", JobID: "job-2", Size: 150, Summary: map[string]interface{}{"step_index": 2000, "mod_type": "mod"}},
		{ID: "diff-3", JobID: "job-3", Size: 100, Summary: map[string]interface{}{"step_index": 3000, "mod_type": "mod"}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs/run-123/repos/repo-abc/diffs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Diffs []testDiff `json:"diffs"`
		}{Diffs: diffs})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := ListRunRepoDiffsCommand{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   domaintypes.RunID("run-123"),
		RepoID:  "repo-abc",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d diffs, want 3", len(result))
	}

	if got, ok := result[0].Summary.StepIndex(); !ok || got != 1000 {
		t.Fatalf("result[0].Summary.StepIndex()=%d ok=%v, want 1000 true", got, ok)
	}
	if got, ok := result[2].Summary.StepIndex(); !ok || got != 3000 {
		t.Fatalf("result[2].Summary.StepIndex()=%d ok=%v, want 3000 true", got, ok)
	}
}

func TestListRunRepoDiffsCommand_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Diffs []DiffEntry `json:"diffs"`
		}{Diffs: []DiffEntry{}})
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := ListRunRepoDiffsCommand{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   domaintypes.RunID("run-empty"),
		RepoID:  "repo-abc",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("got %d diffs, want 0", len(result))
	}
}

// TestDownloadDiffCommand_Success verifies successful download and decompression.
func TestDownloadDiffCommand_Success(t *testing.T) {
	patchContent := "diff --git a/test.txt b/test.txt\n+added line\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Query().Get("download") != "true" {
			t.Error("expected download=true query param")
		}

		// Write gzipped content.
		w.Header().Set("Content-Type", "application/gzip")
		gw := gzip.NewWriter(w)
		_, _ = gw.Write([]byte(patchContent))
		_ = gw.Close()
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := DownloadDiffCommand{
		Client:  srv.Client(),
		BaseURL: base,
		DiffID:  "diff-abc",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if string(result) != patchContent {
		t.Errorf("patch = %q, want %q", string(result), patchContent)
	}
}

// TestDownloadDiffCommand_EmptyPatch verifies handling of empty patches.
func TestDownloadDiffCommand_EmptyPatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write empty gzipped content.
		w.Header().Set("Content-Type", "application/gzip")
		gw := gzip.NewWriter(w)
		_ = gw.Close()
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := DownloadDiffCommand{
		Client:  srv.Client(),
		BaseURL: base,
		DiffID:  "diff-empty",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("got %d bytes, want 0 for empty patch", len(result))
	}
}

// TestDecompressGzipBytes_Success verifies gzip decompression.
func TestDecompressGzipBytes_Success(t *testing.T) {
	original := []byte("test content for compression")

	// Compress the content.
	var compressed []byte
	func() {
		var buf = new(bytesBuffer)
		gw := gzip.NewWriter(buf)
		_, _ = gw.Write(original)
		_ = gw.Close()
		compressed = buf.Bytes()
	}()

	result, err := decompressGzipBytes(compressed)
	if err != nil {
		t.Fatalf("decompressGzipBytes() error = %v", err)
	}

	if string(result) != string(original) {
		t.Errorf("result = %q, want %q", string(result), string(original))
	}
}

// TestDecompressGzipBytes_EmptyInput verifies empty input handling.
func TestDecompressGzipBytes_EmptyInput(t *testing.T) {
	result, err := decompressGzipBytes([]byte{})
	if err != nil {
		t.Fatalf("decompressGzipBytes() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("got %d bytes, want 0", len(result))
	}
}

// bytesBuffer is a simple bytes.Buffer wrapper for testing.
type bytesBuffer struct {
	data []byte
}

func (b *bytesBuffer) Write(p []byte) (n int, err error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *bytesBuffer) Bytes() []byte {
	return b.data
}

// contains is a helper to check if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
