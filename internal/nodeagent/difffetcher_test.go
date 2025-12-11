package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestDiffFetcher_ListRunDiffs verifies listing diffs for a run.
func TestDiffFetcher_ListRunDiffs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		runID          string
		serverResponse diffListResponse
		serverStatus   int
		wantErr        bool
		wantCount      int
	}{
		{
			name:  "successful list with diffs",
			runID: "test-run-123",
			serverResponse: diffListResponse{
				Diffs: []diffListItem{
					{ID: "diff-1", JobID: "job-1", StepIndex: stepIndex(0), Size: 100},
					{ID: "diff-2", JobID: "job-2", StepIndex: stepIndex(1), Size: 200},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    2,
		},
		{
			name:  "empty diff list",
			runID: "test-run-empty",
			serverResponse: diffListResponse{
				Diffs: []diffListItem{},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    0,
		},
		{
			name:         "server error",
			runID:        "test-run-error",
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				expectedPath := "/v1/mods/" + tt.runID + "/diffs"
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
			}
			fetcher, err := NewDiffFetcher(cfg)
			if err != nil {
				t.Fatalf("NewDiffFetcher() failed: %v", err)
			}

			ctx := context.Background()
			diffs, err := fetcher.ListRunDiffs(ctx, tt.runID)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListRunDiffs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(diffs) != tt.wantCount {
				t.Errorf("ListRunDiffs() returned %d diffs, want %d", len(diffs), tt.wantCount)
			}
		})
	}
}

// TestDiffFetcher_FetchDiffPatch verifies fetching a single diff patch.
func TestDiffFetcher_FetchDiffPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		diffID       string
		patchContent []byte
		serverStatus int
		wantErr      bool
	}{
		{
			name:         "successful fetch",
			diffID:       "diff-123",
			patchContent: gzipBytesHelper(t, []byte("test patch content")),
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "empty patch",
			diffID:       "diff-empty",
			patchContent: gzipBytesHelper(t, []byte("")),
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "server error",
			diffID:       "diff-error",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				expectedPath := "/v1/diffs/" + tt.diffID
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}
				if r.URL.Query().Get("download") != "true" {
					t.Error("expected download=true query parameter")
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					w.Header().Set("Content-Type", "application/gzip")
					_, _ = w.Write(tt.patchContent)
				}
			}))
			defer server.Close()

			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
			}
			fetcher, err := NewDiffFetcher(cfg)
			if err != nil {
				t.Fatalf("NewDiffFetcher() failed: %v", err)
			}

			ctx := context.Background()
			patch, err := fetcher.FetchDiffPatch(ctx, tt.diffID)

			if (err != nil) != tt.wantErr {
				t.Errorf("FetchDiffPatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !bytes.Equal(patch, tt.patchContent) {
				t.Errorf("FetchDiffPatch() returned %d bytes, want %d bytes", len(patch), len(tt.patchContent))
			}
		})
	}
}

// TestDiffFetcher_FetchDiffsForStep verifies fetching all diffs up to a step and
// excluding healing diffs from the rehydration chain.
func TestDiffFetcher_FetchDiffsForStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		runID     string
		stepIndex types.StepIndex
		diffs     []diffListItem
		patches   map[string][]byte
		wantCount int
		wantErr   bool
	}{
		{
			name:      "fetch diffs for step 1 (includes step 0 and 1)",
			runID:     "run-123",
			stepIndex: 1,
			diffs: []diffListItem{
				{ID: "diff-0", JobID: "job-0", StepIndex: stepIndex(0)},
				{ID: "diff-1", JobID: "job-1", StepIndex: stepIndex(1)},
				{ID: "diff-2", JobID: "job-2", StepIndex: stepIndex(2)},
			},
			patches: map[string][]byte{
				"diff-0": gzipBytesHelper(t, []byte("patch 0")),
				"diff-1": gzipBytesHelper(t, []byte("patch 1")),
				"diff-2": gzipBytesHelper(t, []byte("patch 2")),
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "fetch all diffs for step 2",
			runID:     "run-456",
			stepIndex: 2,
			diffs: []diffListItem{
				{ID: "diff-0", JobID: "job-0", StepIndex: stepIndex(0)},
				{ID: "diff-1", JobID: "job-1", StepIndex: stepIndex(1)},
				{ID: "diff-2", JobID: "job-2", StepIndex: stepIndex(2)},
			},
			patches: map[string][]byte{
				"diff-0": gzipBytesHelper(t, []byte("patch 0")),
				"diff-1": gzipBytesHelper(t, []byte("patch 1")),
				"diff-2": gzipBytesHelper(t, []byte("patch 2")),
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "exclude healing diffs from rehydration chain",
			runID:     "run-healing",
			stepIndex: 1,
			diffs: []diffListItem{
				{ID: "diff-0-mod", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-0-heal", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "healing"}},
				{ID: "diff-1-mod", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-1-heal1", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "healing", "healing_attempt": 1}},
				{ID: "diff-1-heal2", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "healing", "healing_attempt": 2}},
				{ID: "diff-2-mod", JobID: "job-2", StepIndex: stepIndex(2), Summary: map[string]any{"mod_type": "mod"}},
			},
			patches: map[string][]byte{
				"diff-0-mod":   gzipBytesHelper(t, []byte("patch 0 mod")),
				"diff-0-heal":  gzipBytesHelper(t, []byte("patch 0 heal")),
				"diff-1-mod":   gzipBytesHelper(t, []byte("patch 1 mod")),
				"diff-1-heal1": gzipBytesHelper(t, []byte("patch 1 heal 1")),
				"diff-1-heal2": gzipBytesHelper(t, []byte("patch 1 heal 2")),
				"diff-2-mod":   gzipBytesHelper(t, []byte("patch 2 mod")),
			},
			wantCount: 2,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/mods/"+tt.runID+"/diffs" {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: tt.diffs})
					return
				}

				for _, d := range tt.diffs {
					if r.URL.Path == "/v1/diffs/"+d.ID {
						w.Header().Set("Content-Type", "application/gzip")
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write(tt.patches[d.ID])
						return
					}
				}

				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
			}
			fetcher, err := NewDiffFetcher(cfg)
			if err != nil {
				t.Fatalf("NewDiffFetcher() failed: %v", err)
			}

			ctx := context.Background()
			patches, err := fetcher.FetchDiffsForStep(ctx, tt.runID, tt.stepIndex)

			if (err != nil) != tt.wantErr {
				t.Errorf("FetchDiffsForStep() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(patches) != tt.wantCount {
				t.Errorf("FetchDiffsForStep() returned %d patches, want %d", len(patches), tt.wantCount)
			}
		})
	}
}

// stepIndex returns a StepIndex value.
func stepIndex(v int32) types.StepIndex {
	return types.StepIndex(v)
}

// gzipBytesHelper compresses input bytes using gzip (test helper).
func gzipBytesHelper(t *testing.T, input []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}
