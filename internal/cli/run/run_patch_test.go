package run

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunPatch_RepoIDLatestToFile(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	diffID1 := domaintypes.DiffID("550e8400-e29b-41d4-a716-4466554400a1")
	diffID2 := domaintypes.DiffID("550e8400-e29b-41d4-a716-4466554400a2")
	expectedPatch := []byte("raw-gzip-patch-bytes")

	var listCalled bool
	var downloadCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs"
		if r.URL.Path != wantPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}

		if r.URL.Query().Get("download") == "true" {
			downloadCalled = true
			if got := r.URL.Query().Get("diff_id"); got != diffID2.String() {
				t.Fatalf("download diff_id=%q, want %q", got, diffID2.String())
			}
			if got := r.URL.Query().Get("accumulated"); got != "true" {
				t.Fatalf("download accumulated=%q, want true", got)
			}
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(expectedPatch)
			return
		}

		listCalled = true
		resp := map[string]any{
			"diffs": []map[string]any{
				{"id": diffID1.String(), "job_id": domaintypes.NewJobID().String(), "created_at": "2026-01-01T00:00:00Z", "gzipped_size": 12},
				{"id": diffID2.String(), "job_id": domaintypes.NewJobID().String(), "created_at": "2026-01-01T00:00:01Z", "gzipped_size": 15},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	outPath := filepath.Join(t.TempDir(), "patch.patch.gz")
	var stderr bytes.Buffer
	err := RunPatch(context.Background(), PatchOptions{
		RunID:      runID.String(),
		RepoID:     repoID.String(),
		OutputPath: outPath,
		Output:     &stderr,
	})
	if err != nil {
		t.Fatalf("RunPatch error: %v", err)
	}
	if !listCalled {
		t.Fatal("expected list endpoint call")
	}
	if !downloadCalled {
		t.Fatal("expected download endpoint call")
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(got) != string(expectedPatch) {
		t.Fatalf("saved patch mismatch: got %q want %q", string(got), string(expectedPatch))
	}
}

func TestRunPatch_DiffIDNotFound(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	missingDiff := "550e8400-e29b-41d4-a716-4466554400ff"

	var downloadCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs"
		if r.URL.Path != wantPath {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("download") == "true" {
			downloadCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		resp := map[string]any{
			"diffs": []map[string]any{
				{"id": "550e8400-e29b-41d4-a716-4466554400aa", "job_id": domaintypes.NewJobID().String(), "created_at": "2026-01-01T00:00:00Z", "gzipped_size": 10},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var stderr bytes.Buffer
	err := RunPatch(context.Background(), PatchOptions{
		RunID:  runID.String(),
		RepoID: repoID.String(),
		DiffID: missingDiff,
		Output: &stderr,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
	if downloadCalled {
		t.Fatal("download endpoint should not be called when diff is missing from listing")
	}
}

func TestRunPatch_ResolveRepoViaRepoURL(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	diffID := domaintypes.DiffID("550e8400-e29b-41d4-a716-4466554400bb")
	patchBytes := []byte("gzip-bytes-from-server")

	var pullCalled bool
	var listCalled bool
	var downloadCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+runID.String()+"/pull":
			pullCalled = true
			resp := map[string]any{
				"run_id":          runID.String(),
				"repo_id":         repoID.String(),
				"repo_target_ref": "migs/target",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			if r.URL.Query().Get("download") == "true" {
				downloadCalled = true
				if got := r.URL.Query().Get("diff_id"); got != diffID.String() {
					t.Fatalf("download diff_id=%q, want %q", got, diffID.String())
				}
				if got := r.URL.Query().Get("accumulated"); got != "true" {
					t.Fatalf("download accumulated=%q, want true", got)
				}
				_, _ = w.Write(patchBytes)
				return
			}
			listCalled = true
			resp := map[string]any{
				"diffs": []map[string]any{
					{"id": diffID.String(), "job_id": domaintypes.NewJobID().String(), "created_at": "2026-01-01T00:00:00Z", "gzipped_size": 10},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	outPath := filepath.Join(t.TempDir(), "resolved.patch.gz")
	var stderr bytes.Buffer
	err := RunPatch(context.Background(), PatchOptions{
		RunID:      runID.String(),
		RepoURL:    "https://github.com/example/repo.git",
		OutputPath: outPath,
		Output:     &stderr,
	})
	if err != nil {
		t.Fatalf("RunPatch error: %v", err)
	}
	if !pullCalled {
		t.Fatal("expected run pull resolution endpoint call")
	}
	if !listCalled {
		t.Fatal("expected diff listing call")
	}
	if !downloadCalled {
		t.Fatal("expected diff download call")
	}
}

func TestRunPatch_AutoResolveSingleRunRepo(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	diffID := domaintypes.DiffID("550e8400-e29b-41d4-a716-4466554400cc")
	patchBytes := []byte("single-repo-gzip")

	var listReposCalled bool
	var listDiffsCalled bool
	var downloadCalled bool
	var pullCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+runID.String()+"/pull":
			pullCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos":
			listReposCalled = true
			resp := map[string]any{
				"repos": []map[string]any{
					{
						"run_id":      runID.String(),
						"repo_id":     repoID.String(),
						"repo_url":    "https://github.com/example/repo.git",
						"base_ref":    "main",
						"target_ref":  "feature",
						"status":      "success",
						"attempt":     1,
						"created_at":  "2026-01-01T00:00:00Z",
						"started_at":  "2026-01-01T00:00:01Z",
						"finished_at": "2026-01-01T00:00:02Z",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs":
			if r.URL.Query().Get("download") == "true" {
				downloadCalled = true
				if got := r.URL.Query().Get("diff_id"); got != diffID.String() {
					t.Fatalf("download diff_id=%q, want %q", got, diffID.String())
				}
				if got := r.URL.Query().Get("accumulated"); got != "true" {
					t.Fatalf("download accumulated=%q, want true", got)
				}
				_, _ = w.Write(patchBytes)
				return
			}
			listDiffsCalled = true
			resp := map[string]any{
				"diffs": []map[string]any{
					{"id": diffID.String(), "job_id": domaintypes.NewJobID().String(), "created_at": "2026-01-01T00:00:00Z", "gzipped_size": 10},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	outPath := filepath.Join(t.TempDir(), "single.patch.gz")
	var stderr bytes.Buffer
	err := RunPatch(context.Background(), PatchOptions{
		RunID:      runID.String(),
		OutputPath: outPath,
		Output:     &stderr,
	})
	if err != nil {
		t.Fatalf("RunPatch error: %v", err)
	}
	if !listReposCalled {
		t.Fatal("expected run repos list call")
	}
	if !listDiffsCalled {
		t.Fatal("expected diff listing call")
	}
	if !downloadCalled {
		t.Fatal("expected diff download call")
	}
	if pullCalled {
		t.Fatal("did not expect run pull resolution call for single repo auto-resolution")
	}
}

func TestRunPatch_MultiRepoRequiresSelector(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID1 := domaintypes.NewMigRepoID()
	repoID2 := domaintypes.NewMigRepoID()

	var listReposCalled bool
	var listDiffsCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID.String()+"/repos":
			listReposCalled = true
			resp := map[string]any{
				"repos": []map[string]any{
					{
						"run_id":     runID.String(),
						"repo_id":    repoID1.String(),
						"repo_url":   "https://github.com/example/repo-1.git",
						"base_ref":   "main",
						"target_ref": "feature-1",
						"status":     "success",
						"attempt":    1,
						"created_at": "2026-01-01T00:00:00Z",
					},
					{
						"run_id":     runID.String(),
						"repo_id":    repoID2.String(),
						"repo_url":   "https://github.com/example/repo-2.git",
						"base_ref":   "main",
						"target_ref": "feature-2",
						"status":     "success",
						"attempt":    1,
						"created_at": "2026-01-01T00:00:01Z",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		case strings.HasSuffix(r.URL.Path, "/diffs"):
			listDiffsCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var stderr bytes.Buffer
	err := RunPatch(context.Background(), PatchOptions{
		RunID:  runID.String(),
		Output: &stderr,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "multiple repos found in run") {
		t.Fatalf("expected multi-repo selector error, got %v", err)
	}
	if !listReposCalled {
		t.Fatal("expected run repos list call")
	}
	if listDiffsCalled {
		t.Fatal("did not expect diff listing call when repo selection is ambiguous")
	}
}

func TestRunPatch_RepoIDAndRepoURLMutuallyExclusive(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)

	var stderr bytes.Buffer
	err := RunPatch(context.Background(), PatchOptions{
		RunID:   runID.String(),
		RepoID:  repoID.String(),
		RepoURL: "https://github.com/example/repo.git",
		Output:  &stderr,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive error, got %v", err)
	}
}
