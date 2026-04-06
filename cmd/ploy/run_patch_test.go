package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

func TestRunPatchRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing run id",
			args:    []string{"run", "patch"},
			wantErr: "run-id required",
		},
		{
			name:    "extra positional argument",
			args:    []string{"run", "patch", "run-1", "extra"},
			wantErr: "unexpected argument: extra",
		},
		{
			name:    "unknown flag",
			args:    []string{"run", "patch", "--unknown", "run-1"},
			wantErr: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := executeCmd(tt.args, &buf)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunPatchUsageHelp(t *testing.T) {
	var buf bytes.Buffer
	printRunPatchUsage(&buf)

	out := buf.String()
	if !strings.Contains(out, "Usage: ploy run patch") {
		t.Fatalf("usage line missing: %q", out)
	}
	if !strings.Contains(out, "--repo-id") {
		t.Fatalf("expected --repo-id in usage: %q", out)
	}
	if !strings.Contains(out, "--diff-id") {
		t.Fatalf("expected --diff-id in usage: %q", out)
	}
	if !strings.Contains(out, ".patch.gz") {
		t.Fatalf("expected .patch.gz mention in usage: %q", out)
	}
}

func TestHandleRunPatch_RepoIDLatestToFile(t *testing.T) {
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
	err := handleRunPatch([]string{"--repo-id", repoID.String(), "--output", outPath, runID.String()}, &stderr)
	if err != nil {
		t.Fatalf("handleRunPatch error: %v", err)
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

func TestHandleRunPatch_DiffIDNotFound(t *testing.T) {
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
	err := handleRunPatch([]string{"--repo-id", repoID.String(), "--diff-id", missingDiff, runID.String()}, &stderr)
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

func TestHandleRunPatch_ResolveRepoViaOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git command not found, skipping test")
	}

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

	repoDir := gitrepo.SetupWithRemote(t, "https://github.com/example/repo.git")
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "resolved.patch.gz")
	var stderr bytes.Buffer
	err = handleRunPatch([]string{"--output", outPath, runID.String()}, &stderr)
	if err != nil {
		t.Fatalf("handleRunPatch error: %v", err)
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
