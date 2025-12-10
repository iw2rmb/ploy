package mods

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestFetchRunWithCommitSHA_Success verifies successful retrieval of run details
// including commit_sha via GET /v1/runs/{id}.
func TestFetchRunWithCommitSHA_Success(t *testing.T) {
	commitSHA := "abc123def456"
	runDetails := RunDetails{
		ID:        "run-123",
		RepoURL:   "https://github.com/org/repo.git",
		Status:    "succeeded",
		BaseRef:   "main",
		TargetRef: "feature-branch",
		CommitSHA: &commitSHA,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v1/runs/run-123" {
			t.Errorf("expected path /v1/runs/run-123, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(runDetails)
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := FetchRunWithCommitSHA{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   "run-123",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.ID != "run-123" {
		t.Errorf("ID = %q, want %q", result.ID, "run-123")
	}
	if result.CommitSHA == nil || *result.CommitSHA != commitSHA {
		t.Errorf("CommitSHA = %v, want %q", result.CommitSHA, commitSHA)
	}
	if result.BaseRef != "main" {
		t.Errorf("BaseRef = %q, want %q", result.BaseRef, "main")
	}
	if result.TargetRef != "feature-branch" {
		t.Errorf("TargetRef = %q, want %q", result.TargetRef, "feature-branch")
	}
}

// TestFetchRunWithCommitSHA_NotFound verifies 404 handling.
func TestFetchRunWithCommitSHA_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "run not found", http.StatusNotFound)
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := FetchRunWithCommitSHA{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   "nonexistent",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// TestFetchRunWithCommitSHA_MissingParams verifies parameter validation.
func TestFetchRunWithCommitSHA_MissingParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cmd     FetchRunWithCommitSHA
		wantErr string
	}{
		{
			name:    "missing client",
			cmd:     FetchRunWithCommitSHA{BaseURL: &url.URL{}, RunID: "r1"},
			wantErr: "http client required",
		},
		{
			name:    "missing base url",
			cmd:     FetchRunWithCommitSHA{Client: http.DefaultClient, RunID: "r1"},
			wantErr: "base url required",
		},
		{
			name:    "missing run id",
			cmd:     FetchRunWithCommitSHA{Client: http.DefaultClient, BaseURL: &url.URL{}},
			wantErr: "run id required",
		},
		{
			name:    "empty run id",
			cmd:     FetchRunWithCommitSHA{Client: http.DefaultClient, BaseURL: &url.URL{}, RunID: "   "},
			wantErr: "run id required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.cmd.Run(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != "" && tt.wantErr != "" {
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

// TestListAllDiffsCommand_Success verifies successful listing and sorting of diffs.
func TestListAllDiffsCommand_Success(t *testing.T) {
	// Use a simpler struct for test response that doesn't have custom JSON marshaling.
	type testDiff struct {
		ID        string `json:"id"`
		JobID     string `json:"job_id"`
		CreatedAt string `json:"created_at"`
		Size      int    `json:"gzipped_size"`
		StepIndex int    `json:"step_index"`
	}

	// Return diffs in unsorted order; verify they're sorted by step_index.
	diffs := []testDiff{
		{ID: "diff-3", JobID: "job-3", StepIndex: 3000, Size: 100},
		{ID: "diff-1", JobID: "job-1", StepIndex: 1000, Size: 200},
		{ID: "diff-2", JobID: "job-2", StepIndex: 2000, Size: 150},
	}

	// Use a mux to handle the specific endpoint path.
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods/run-123/diffs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			Diffs []testDiff `json:"diffs"`
		}{Diffs: diffs}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := ListAllDiffsCommand{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   "run-123",
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d diffs, want 3", len(result))
	}

	// Verify sorting by step_index.
	if result[0].ID != "diff-1" {
		t.Errorf("result[0].ID = %q, want diff-1 (lowest step_index)", result[0].ID)
	}
	if result[1].ID != "diff-2" {
		t.Errorf("result[1].ID = %q, want diff-2", result[1].ID)
	}
	if result[2].ID != "diff-3" {
		t.Errorf("result[2].ID = %q, want diff-3 (highest step_index)", result[2].ID)
	}
}

// TestListAllDiffsCommand_EmptyList verifies handling of empty diff lists.
func TestListAllDiffsCommand_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			Diffs []DiffEntry `json:"diffs"`
		}{Diffs: []DiffEntry{}})
	}))
	defer srv.Close()

	base, _ := url.Parse(srv.URL)
	cmd := ListAllDiffsCommand{
		Client:  srv.Client(),
		BaseURL: base,
		RunID:   "run-empty",
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
