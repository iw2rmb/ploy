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
					{
						ID:        "diff-1",
						JobID:     "job-1",
						StepIndex: stepIndex(0),
						Size:      100,
					},
					{
						ID:        "diff-2",
						JobID:     "job-2",
						StepIndex: stepIndex(1),
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
			wantCount: 2, // step 0 and 1
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
			// C2: Healing diffs with the same step_index as mod diffs are stored
			// for observability, but rehydration uses only non-healing diffs.
			name:      "exclude healing diffs from rehydration chain",
			runID:     "run-healing",
			stepIndex: 1,
			diffs: []diffListItem{
				// Step 0: mod + healing.
				{ID: "diff-0-mod", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-0-heal", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "healing"}},
				// Step 1: mod + healing (2 attempts).
				{ID: "diff-1-mod", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-1-heal1", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "healing", "healing_attempt": 1}},
				{ID: "diff-1-heal2", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "healing", "healing_attempt": 2}},
				// Step 2: mod (not included).
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
			// Steps 0-1: only mod diffs ("mod_type=mod") are returned.
			wantCount: 2,
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

// TestExtractPathFromJobName verifies path name extraction from job names.
// E3: Path-local rehydration relies on extracting path IDs from healing job names.
func TestExtractPathFromJobName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jobName  string
		wantPath string
	}{
		// Multi-path healing job names (E3).
		{
			name:     "heal job with path-a",
			jobName:  "heal-branch-a-1-0",
			wantPath: "branch-a",
		},
		{
			name:     "heal job with codex-ai path",
			jobName:  "heal-codex-ai-1-0",
			wantPath: "codex-ai",
		},
		{
			name:     "re-gate job with path-a",
			jobName:  "re-gate-branch-a-1",
			wantPath: "branch-a",
		},
		{
			name:     "heal job with multiple mods",
			jobName:  "heal-static-patch-2-3",
			wantPath: "static-patch",
		},
		// Mainline jobs (not healing branches).
		{
			name:     "pre-gate job",
			jobName:  "pre-gate",
			wantPath: "",
		},
		{
			name:     "mod job",
			jobName:  "mod-0",
			wantPath: "",
		},
		{
			name:     "post-gate job",
			jobName:  "post-gate",
			wantPath: "",
		},
		// Edge cases.
		{
			name:     "empty job name",
			jobName:  "",
			wantPath: "",
		},
		{
			name:     "unrelated job name",
			jobName:  "some-other-job",
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ExtractPathFromJobName(tt.jobName)
			if got != tt.wantPath {
				t.Errorf("ExtractPathFromJobName(%q) = %q, want %q", tt.jobName, got, tt.wantPath)
			}
		})
	}
}

// TestDiffFetcher_FetchDiffsForPath verifies path-local diff filtering.
// E3: Ensures execution-path workspaces are isolated by only including mainline + same-path diffs.
func TestDiffFetcher_FetchDiffsForPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		runID       string
		stepIndex   types.StepIndex
		targetPath  string
		diffs       []diffListItem
		patches     map[string][]byte
		wantCount   int
		wantDiffIDs []string // Expected diff IDs in order.
	}{
		{
			name:       "mainline only (no path filter)",
			runID:      "run-mainline",
			stepIndex:  2,
			targetPath: "", // Empty = mainline only.
			diffs: []diffListItem{
				{ID: "diff-0", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-1", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-2-a", JobID: "job-2a", StepIndex: stepIndex(2), Summary: map[string]any{"mod_type": "mod", "path_id": "path-a"}},
				{ID: "diff-2-b", JobID: "job-2b", StepIndex: stepIndex(2), Summary: map[string]any{"mod_type": "mod", "path_id": "path-b"}},
			},
			patches: map[string][]byte{
				"diff-0":   gzipBytesHelper(t, []byte("patch 0")),
				"diff-1":   gzipBytesHelper(t, []byte("patch 1")),
				"diff-2-a": gzipBytesHelper(t, []byte("patch 2a")),
				"diff-2-b": gzipBytesHelper(t, []byte("patch 2b")),
			},
			wantCount:   2, // Only mainline diffs (no path_id).
			wantDiffIDs: []string{"diff-0", "diff-1"},
		},
		{
			name:       "path-a isolation (mainline + path-a only)",
			runID:      "run-path-a",
			stepIndex:  3,
			targetPath: "path-a",
			diffs: []diffListItem{
				{ID: "diff-mainline-0", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-mainline-1", JobID: "job-1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-path-a-2", JobID: "job-a2", StepIndex: stepIndex(2), Summary: map[string]any{"mod_type": "mod", "path_id": "path-a"}},
				{ID: "diff-path-b-2", JobID: "job-b2", StepIndex: stepIndex(2), Summary: map[string]any{"mod_type": "mod", "path_id": "path-b"}},
				{ID: "diff-path-a-3", JobID: "job-a3", StepIndex: stepIndex(3), Summary: map[string]any{"mod_type": "mod", "path_id": "path-a"}},
			},
			patches: map[string][]byte{
				"diff-mainline-0": gzipBytesHelper(t, []byte("patch mainline 0")),
				"diff-mainline-1": gzipBytesHelper(t, []byte("patch mainline 1")),
				"diff-path-a-2":   gzipBytesHelper(t, []byte("patch path-a 2")),
				"diff-path-b-2":   gzipBytesHelper(t, []byte("patch path-b 2")),
				"diff-path-a-3":   gzipBytesHelper(t, []byte("patch path-a 3")),
			},
			wantCount:   4, // Mainline (2) + path-a (2), excludes path-b.
			wantDiffIDs: []string{"diff-mainline-0", "diff-mainline-1", "diff-path-a-2", "diff-path-a-3"},
		},
		{
			name:       "path-b isolation (mainline + path-b only)",
			runID:      "run-path-b",
			stepIndex:  3,
			targetPath: "path-b",
			diffs: []diffListItem{
				{ID: "diff-mainline-0", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-path-a-1", JobID: "job-a1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "mod", "path_id": "path-a"}},
				{ID: "diff-path-b-2", JobID: "job-b2", StepIndex: stepIndex(2), Summary: map[string]any{"mod_type": "mod", "path_id": "path-b"}},
			},
			patches: map[string][]byte{
				"diff-mainline-0": gzipBytesHelper(t, []byte("patch mainline 0")),
				"diff-path-a-1":   gzipBytesHelper(t, []byte("patch path-a 1")),
				"diff-path-b-2":   gzipBytesHelper(t, []byte("patch path-b 2")),
			},
			wantCount:   2, // Mainline (1) + path-b (1), excludes path-a.
			wantDiffIDs: []string{"diff-mainline-0", "diff-path-b-2"},
		},
		{
			name:       "healing diffs excluded regardless of path",
			runID:      "run-healing-excluded",
			stepIndex:  2,
			targetPath: "path-a",
			diffs: []diffListItem{
				{ID: "diff-mod-0", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "mod"}},
				{ID: "diff-heal-0", JobID: "job-0", StepIndex: stepIndex(0), Summary: map[string]any{"mod_type": "healing", "path_id": "path-a"}},
				{ID: "diff-mod-a-1", JobID: "job-a1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "mod", "path_id": "path-a"}},
				{ID: "diff-heal-a-1", JobID: "job-a1", StepIndex: stepIndex(1), Summary: map[string]any{"mod_type": "healing", "path_id": "path-a"}},
			},
			patches: map[string][]byte{
				"diff-mod-0":    gzipBytesHelper(t, []byte("patch mod 0")),
				"diff-heal-0":   gzipBytesHelper(t, []byte("patch heal 0")),
				"diff-mod-a-1":  gzipBytesHelper(t, []byte("patch mod a 1")),
				"diff-heal-a-1": gzipBytesHelper(t, []byte("patch heal a 1")),
			},
			wantCount:   2, // Only mod diffs (excludes healing).
			wantDiffIDs: []string{"diff-mod-0", "diff-mod-a-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Track fetched diff IDs.
			var fetchedIDs []string

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
						fetchedIDs = append(fetchedIDs, d.ID)
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

			// Execute fetch with path filter.
			ctx := context.Background()
			patches, err := fetcher.FetchDiffsForPath(ctx, tt.runID, tt.stepIndex, tt.targetPath)

			if err != nil {
				t.Fatalf("FetchDiffsForBranch() error = %v", err)
			}

			if len(patches) != tt.wantCount {
				t.Errorf("FetchDiffsForBranch() returned %d patches, want %d", len(patches), tt.wantCount)
			}

			// Verify fetched diff IDs match expected order.
			if len(tt.wantDiffIDs) > 0 {
				if len(fetchedIDs) != len(tt.wantDiffIDs) {
					t.Errorf("fetched %d diffs, want %d: got %v, want %v", len(fetchedIDs), len(tt.wantDiffIDs), fetchedIDs, tt.wantDiffIDs)
				}
				for i, wantID := range tt.wantDiffIDs {
					if i < len(fetchedIDs) && fetchedIDs[i] != wantID {
						t.Errorf("diff[%d] = %q, want %q", i, fetchedIDs[i], wantID)
					}
				}
			}
		})
	}
}

// --- Test Helpers ---

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
