package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
					{
						ID:        "diff-1",
						StageID:   "stage-1",
						StepIndex: ptrInt32(0),
						Size:      100,
					},
					{
						ID:        "diff-2",
						StageID:   "stage-2",
						StepIndex: ptrInt32(1),
						Size:      200,
					},
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

			// Create test server.
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

			// Create fetcher.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
			}
			fetcher, err := NewDiffFetcher(cfg)
			if err != nil {
				t.Fatalf("NewDiffFetcher() failed: %v", err)
			}

			// Execute list.
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

			// Create test server.
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

			// Create fetcher.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
			}
			fetcher, err := NewDiffFetcher(cfg)
			if err != nil {
				t.Fatalf("NewDiffFetcher() failed: %v", err)
			}

			// Execute fetch.
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

// TestDiffFetcher_FetchDiffsForStep verifies fetching all diffs up to a step.
func TestDiffFetcher_FetchDiffsForStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		runID     string
		stepIndex int32
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
				{ID: "diff-0", StepIndex: ptrInt32(0)},
				{ID: "diff-1", StepIndex: ptrInt32(1)},
				{ID: "diff-2", StepIndex: ptrInt32(2)},
			},
			patches: map[string][]byte{
				"diff-0": gzipBytesHelper(t, []byte("patch 0")),
				"diff-1": gzipBytesHelper(t, []byte("patch 1")),
				"diff-2": gzipBytesHelper(t, []byte("patch 2")),
			},
			wantCount: 2, // step 0 and 1
			wantErr:   false,
		},
		{
			name:      "fetch all diffs for step 2",
			runID:     "run-456",
			stepIndex: 2,
			diffs: []diffListItem{
				{ID: "diff-0", StepIndex: ptrInt32(0)},
				{ID: "diff-1", StepIndex: ptrInt32(1)},
				{ID: "diff-2", StepIndex: ptrInt32(2)},
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
			name:      "exclude legacy diffs (nil step_index)",
			runID:     "run-789",
			stepIndex: 1,
			diffs: []diffListItem{
				{ID: "diff-0", StepIndex: ptrInt32(0)},
				{ID: "diff-1", StepIndex: ptrInt32(1)},
				{ID: "diff-legacy", StepIndex: nil}, // Legacy aggregate diff.
			},
			patches: map[string][]byte{
				"diff-0": gzipBytesHelper(t, []byte("patch 0")),
				"diff-1": gzipBytesHelper(t, []byte("patch 1")),
			},
			wantCount: 2, // Exclude legacy diff.
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/mods/"+tt.runID+"/diffs" {
					// List diffs endpoint.
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: tt.diffs})
					return
				}

				// Fetch individual diff patch endpoint.
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

			// Create fetcher.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
			}
			fetcher, err := NewDiffFetcher(cfg)
			if err != nil {
				t.Fatalf("NewDiffFetcher() failed: %v", err)
			}

			// Execute fetch.
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

// --- Test Helpers ---

// ptrInt32 returns a pointer to an int32 value.
func ptrInt32(v int32) *int32 {
	return &v
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
