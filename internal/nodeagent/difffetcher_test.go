package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestDiffFetcher_FetchDiffsForStepRepo_Ordering verifies that diffs are sorted
// deterministically by (step_index, created_at, id) before downloading, ensuring
// patches are applied in a stable order regardless of server response ordering.
func TestDiffFetcher_FetchDiffsForStepRepo_Ordering(t *testing.T) {
	t.Parallel()

	// Define timestamps for deterministic ordering tests.
	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2025, 1, 1, 10, 1, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 10, 2, 0, 0, time.UTC)

	// Test constants for run/repo IDs.
	const testRunID = "run-order-test"
	const testRepoID = "repo-order-test"

	tests := []struct {
		name             string
		diffs            []diffListItem  // Shuffled input order from server.
		expectedPatchIDs []string        // Expected order after sorting.
		stepIndex        types.StepIndex // Target step index for fetch.
	}{
		{
			name: "shuffled by step_index - sorted correctly",
			diffs: []diffListItem{
				// Server returns diffs out of step order.
				{ID: "diff-step2", JobID: "job-2", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(2).ModType("mod").MustBuild()},
				{ID: "diff-step0", JobID: "job-0", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-step1", JobID: "job-1", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
			},
			expectedPatchIDs: []string{"diff-step0", "diff-step1", "diff-step2"},
			stepIndex:        2,
		},
		{
			name: "same step_index - sorted by created_at",
			diffs: []diffListItem{
				// Same step, different creation times (shuffled).
				{ID: "diff-late", JobID: "job-late", CreatedAt: t2, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-early", JobID: "job-early", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-mid", JobID: "job-mid", CreatedAt: t1, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
			},
			expectedPatchIDs: []string{"diff-early", "diff-mid", "diff-late"},
			stepIndex:        0,
		},
		{
			name: "same step_index and created_at - sorted by id",
			diffs: []diffListItem{
				// Same step and timestamp, different IDs (shuffled).
				{ID: "diff-c", JobID: "job-c", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-a", JobID: "job-a", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "diff-b", JobID: "job-b", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
			},
			expectedPatchIDs: []string{"diff-a", "diff-b", "diff-c"},
			stepIndex:        0,
		},
		{
			name: "complex mixed ordering - all sort keys exercised",
			diffs: []diffListItem{
				// Mix of step_index, created_at, and id variations (shuffled).
				{ID: "s1-t1-b", JobID: "job-s1-t1-b", CreatedAt: t1, Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
				{ID: "s0-t0-a", JobID: "job-s0-t0-a", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "s1-t0-a", JobID: "job-s1-t0-a", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
				{ID: "s0-t0-b", JobID: "job-s0-t0-b", CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().StepIndex(0).ModType("mod").MustBuild()},
				{ID: "s1-t1-a", JobID: "job-s1-t1-a", CreatedAt: t1, Summary: types.NewDiffSummaryBuilder().StepIndex(1).ModType("mod").MustBuild()},
			},
			// Expected: step 0 first, then step 1.
			// Within step 0: t0-a before t0-b (same time, id tiebreaker).
			// Within step 1: t0-a first, then t1-a, then t1-b (time, then id).
			expectedPatchIDs: []string{"s0-t0-a", "s0-t0-b", "s1-t0-a", "s1-t1-a", "s1-t1-b"},
			stepIndex:        1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track the order in which patches are fetched.
			var fetchedIDs []string

			// Build patch content map.
			patches := make(map[string][]byte)
			for _, d := range tt.diffs {
				patches[d.ID] = gzipBytesHelper(t, []byte("patch-"+d.ID))
			}

			// Expected path for this test.
			expectedPath := "/v1/runs/" + testRunID + "/repos/" + testRepoID + "/diffs"

			// Copy tt.diffs locally for the closure.
			testDiffs := tt.diffs

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate the request path.
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				// List endpoint (no download param or download != "true").
				if r.URL.Query().Get("download") != "true" {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: testDiffs})
					return
				}

				// Download endpoint - track fetch order.
				diffID := r.URL.Query().Get("diff_id")
				fetchedIDs = append(fetchedIDs, diffID)
				w.Header().Set("Content-Type", "application/gzip")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(patches[diffID])
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
			_, err = fetcher.FetchDiffsForStepRepo(ctx, testRunID, testRepoID, tt.stepIndex)
			if err != nil {
				t.Fatalf("FetchDiffsForStepRepo() failed: %v", err)
			}

			// Verify the patches were fetched in the expected sorted order.
			if len(fetchedIDs) != len(tt.expectedPatchIDs) {
				t.Fatalf("fetched %d patches, expected %d", len(fetchedIDs), len(tt.expectedPatchIDs))
			}
			for i, expected := range tt.expectedPatchIDs {
				if fetchedIDs[i] != expected {
					t.Errorf("patch %d: fetched %q, expected %q", i, fetchedIDs[i], expected)
				}
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
