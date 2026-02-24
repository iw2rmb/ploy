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
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID1 := domaintypes.NewJobID()
	jobID2 := domaintypes.NewJobID()
	jobID3 := domaintypes.NewJobID()

	type testDiff struct {
		ID        string                 `json:"id"`
		JobID     string                 `json:"job_id"`
		CreatedAt string                 `json:"created_at"`
		Size      int                    `json:"gzipped_size"`
		Summary   map[string]interface{} `json:"summary,omitempty"`
	}

	diffs := []testDiff{
		{ID: "550e8400-e29b-41d4-a716-446655440000", JobID: jobID1.String(), CreatedAt: "2026-01-10T00:00:00Z", Size: 200, Summary: map[string]interface{}{"next_id": 1000, "job_type": "mod"}},
		{ID: "550e8400-e29b-41d4-a716-446655440001", JobID: jobID2.String(), CreatedAt: "2026-01-10T00:00:01Z", Size: 150, Summary: map[string]interface{}{"next_id": 2000, "job_type": "mod"}},
		{ID: "550e8400-e29b-41d4-a716-446655440002", JobID: jobID3.String(), CreatedAt: "2026-01-10T00:00:02Z", Size: 100, Summary: map[string]interface{}{"next_id": 3000, "job_type": "mod"}},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs", func(w http.ResponseWriter, r *http.Request) {
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
		RunID:   runID,
		RepoID:  repoID,
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d diffs, want 3", len(result))
	}

	if got := result[0].Summary.JobType(); got != "mod" {
		t.Fatalf("result[0].Summary.JobType()=%q, want %q", got, "mod")
	}
	if got := result[2].Summary.JobType(); got != "mod" {
		t.Fatalf("result[2].Summary.JobType()=%q, want %q", got, "mod")
	}
}

func TestListRunRepoDiffsCommand_EmptyList(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()

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
		RunID:   runID,
		RepoID:  repoID,
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
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()

	patchContent := "diff --git a/test.txt b/test.txt\n+added line\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		wantPath := "/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs"
		if r.URL.Path != wantPath {
			t.Errorf("expected path %s, got %s", wantPath, r.URL.Path)
		}
		if r.URL.Query().Get("download") != "true" {
			t.Error("expected download=true query param")
		}
		if r.URL.Query().Get("diff_id") != "550e8400-e29b-41d4-a716-4466554400aa" {
			t.Errorf("expected diff_id=550e8400-e29b-41d4-a716-4466554400aa, got %s", r.URL.Query().Get("diff_id"))
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
		RunID:   runID,
		RepoID:  repoID,
		DiffID:  domaintypes.DiffID("550e8400-e29b-41d4-a716-4466554400aa"),
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
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs"
		if r.URL.Path != wantPath {
			t.Errorf("expected path %s, got %s", wantPath, r.URL.Path)
		}
		if r.URL.Query().Get("download") != "true" {
			t.Error("expected download=true query param")
		}
		if r.URL.Query().Get("diff_id") != "550e8400-e29b-41d4-a716-4466554400bb" {
			t.Errorf("expected diff_id=550e8400-e29b-41d4-a716-4466554400bb, got %s", r.URL.Query().Get("diff_id"))
		}
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
		RunID:   runID,
		RepoID:  repoID,
		DiffID:  domaintypes.DiffID("550e8400-e29b-41d4-a716-4466554400bb"),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 0 {
		t.Errorf("got %d bytes, want 0 for empty patch", len(result))
	}
}
