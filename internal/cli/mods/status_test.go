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
		{ID: "550e8400-e29b-41d4-a716-446655440000", JobID: "job-1", CreatedAt: "2026-01-10T00:00:00Z", Size: 200, Summary: map[string]interface{}{"step_index": 1000, "mod_type": "mod"}},
		{ID: "550e8400-e29b-41d4-a716-446655440001", JobID: "job-2", CreatedAt: "2026-01-10T00:00:01Z", Size: 150, Summary: map[string]interface{}{"step_index": 2000, "mod_type": "mod"}},
		{ID: "550e8400-e29b-41d4-a716-446655440002", JobID: "job-3", CreatedAt: "2026-01-10T00:00:02Z", Size: 100, Summary: map[string]interface{}{"step_index": 3000, "mod_type": "mod"}},
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
		RepoID:  domaintypes.ModRepoID("repo-abc"),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("got %d diffs, want 3", len(result))
	}

	if got, ok := result[0].Summary.StepIndex(); !ok || got != 1000 {
		t.Fatalf("result[0].Summary.StepIndex()=%v ok=%v, want 1000 true", got, ok)
	}
	if got, ok := result[2].Summary.StepIndex(); !ok || got != 3000 {
		t.Fatalf("result[2].Summary.StepIndex()=%v ok=%v, want 3000 true", got, ok)
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
		RepoID:  domaintypes.ModRepoID("repo-abc"),
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
		if r.URL.Path != "/v1/runs/run-123/repos/repo-abc/diffs" {
			t.Errorf("expected path /v1/runs/run-123/repos/repo-abc/diffs, got %s", r.URL.Path)
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
		RunID:   domaintypes.RunID("run-123"),
		RepoID:  domaintypes.ModRepoID("repo-abc"),
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run-123/repos/repo-abc/diffs" {
			t.Errorf("expected path /v1/runs/run-123/repos/repo-abc/diffs, got %s", r.URL.Path)
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
		RunID:   domaintypes.RunID("run-123"),
		RepoID:  domaintypes.ModRepoID("repo-abc"),
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
