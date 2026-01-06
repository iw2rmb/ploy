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

// TestDiffFetcher_ListRunRepoDiffs verifies listing diffs for a repo execution within a run.
func TestDiffFetcher_ListRunRepoDiffs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		runID          string
		repoID         string
		serverResponse diffListResponse
		serverStatus   int
		wantErr        bool
		wantCount      int
	}{
		{
			name:   "successful list with diffs",
			runID:  "test-run-123",
			repoID: "repo-abc",
			serverResponse: diffListResponse{
				Diffs: []diffListItem{
					{ID: "diff-1", JobID: "job-1", Size: 100},
					{ID: "diff-2", JobID: "job-2", Size: 200},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    2,
		},
		{
			name:   "empty diff list",
			runID:  "test-run-empty",
			repoID: "repo-abc",
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
			repoID:       "repo-abc",
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
				expectedPath := "/v1/runs/" + tt.runID + "/repos/" + tt.repoID + "/diffs"
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
			diffs, err := fetcher.ListRunRepoDiffs(ctx, tt.runID, tt.repoID)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListRunRepoDiffs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(diffs) != tt.wantCount {
				t.Errorf("ListRunRepoDiffs() returned %d diffs, want %d", len(diffs), tt.wantCount)
			}
		})
	}
}

// TestDiffFetcher_FetchRunRepoDiffPatch verifies fetching a single diff patch for a repo execution.
func TestDiffFetcher_FetchRunRepoDiffPatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		runID        string
		repoID       string
		diffID       string
		patchContent []byte
		serverStatus int
		wantErr      bool
	}{
		{
			name:         "successful fetch",
			runID:        "run-123",
			repoID:       "repo-abc",
			diffID:       "diff-123",
			patchContent: gzipBytesHelper(t, []byte("test patch content")),
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "empty patch",
			runID:        "run-123",
			repoID:       "repo-abc",
			diffID:       "diff-empty",
			patchContent: gzipBytesHelper(t, []byte("")),
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "server error",
			runID:        "run-123",
			repoID:       "repo-abc",
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
				expectedPath := "/v1/runs/" + tt.runID + "/repos/" + tt.repoID + "/diffs"
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}
				if r.URL.Query().Get("download") != "true" {
					t.Error("expected download=true query parameter")
				}
				if r.URL.Query().Get("diff_id") != tt.diffID {
					t.Errorf("expected diff_id=%q, got %q", tt.diffID, r.URL.Query().Get("diff_id"))
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
			patch, err := fetcher.FetchRunRepoDiffPatch(ctx, tt.runID, tt.repoID, tt.diffID)

			if (err != nil) != tt.wantErr {
				t.Errorf("FetchRunRepoDiffPatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !bytes.Equal(patch, tt.patchContent) {
				t.Errorf("FetchRunRepoDiffPatch() returned %d bytes, want %d bytes", len(patch), len(tt.patchContent))
			}
		})
	}
}

// TestDiffFetcher_FetchDiffsForStepRepo verifies fetching all diffs up to a step (inclusive)
// for a repo execution, and excluding healing diffs from the rehydration chain.
func TestDiffFetcher_FetchDiffsForStepRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		runID     string
		repoID    string
		stepIndex types.StepIndex
		diffs     []diffListItem
		patches   map[string][]byte
		wantCount int
		wantErr   bool
	}{
		{
			name:      "fetch diffs for step 1 (includes step 0 and 1)",
			runID:     "run-123",
			repoID:    "repo-abc",
			stepIndex: 1,
			diffs: []diffListItem{
				{ID: "diff-0", JobID: "job-0", Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-1", JobID: "job-1", Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
				{ID: "diff-2", JobID: "job-2", Summary: types.NewDiffSummaryBuilder().StepIndex(2).ModType("mod").MustBuild()},
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
			repoID:    "repo-abc",
			stepIndex: 2,
			diffs: []diffListItem{
				{ID: "diff-0", JobID: "job-0", Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-1", JobID: "job-1", Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
				{ID: "diff-2", JobID: "job-2", Summary: types.NewDiffSummaryBuilder().StepIndex(2).ModType("mod").MustBuild()},
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
			repoID:    "repo-abc",
			stepIndex: 1,
			diffs: []diffListItem{
				{ID: "diff-0-mod", JobID: "job-0", Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-0-heal", JobID: "job-0", Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("healing").MustBuild()},
				{ID: "diff-1-mod", JobID: "job-1", Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
				{ID: "diff-1-heal1", JobID: "job-1", Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("healing").MustBuild()},
				{ID: "diff-1-heal2", JobID: "job-1", Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("healing").MustBuild()},
				{ID: "diff-2-mod", JobID: "job-2", Summary: types.NewDiffSummaryBuilder().StepIndex(2).ModType("mod").MustBuild()},
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
				if r.URL.Path == "/v1/runs/"+tt.runID+"/repos/"+tt.repoID+"/diffs" && r.URL.Query().Get("download") != "true" {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: tt.diffs})
					return
				}

				if r.URL.Path == "/v1/runs/"+tt.runID+"/repos/"+tt.repoID+"/diffs" && r.URL.Query().Get("download") == "true" {
					diffID := r.URL.Query().Get("diff_id")
					for _, d := range tt.diffs {
						if diffID == d.ID {
							w.Header().Set("Content-Type", "application/gzip")
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write(tt.patches[d.ID])
							return
						}
					}
					w.WriteHeader(http.StatusNotFound)
					return
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
			patches, err := fetcher.FetchDiffsForStepRepo(ctx, tt.runID, tt.repoID, tt.stepIndex)

			if (err != nil) != tt.wantErr {
				t.Errorf("FetchDiffsForStepRepo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(patches) != tt.wantCount {
				t.Errorf("FetchDiffsForStepRepo() returned %d patches, want %d", len(patches), tt.wantCount)
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
