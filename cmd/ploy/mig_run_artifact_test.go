package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// TestDownloadRunArtifactsCreatesManifest verifies that artifacts are downloaded
// and a manifest.json file is created with correct metadata.
func TestDownloadRunArtifactsCreatesManifest(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	stageID := domaintypes.NewJobID()
	// Create a mock server that handles run status and artifact downloads.
	artifactContent := []byte("test artifact content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.HasSuffix(r.URL.Path, "/status"):
			// Run status endpoint — return RunSummary directly (canonical response shape).
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(migsapi.RunSummary{
				RunID: runID,
				State: migsapi.RunStateSucceeded,
				Stages: map[domaintypes.JobID]migsapi.StageStatus{
					stageID: {
						State: migsapi.StageStateSucceeded,
						Artifacts: map[string]string{
							"artifact1": "cid-abc123",
						},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/v1/artifacts") && r.URL.Query().Get("cid") != "":
			// Artifact lookup by CID.
			listing := struct {
				Artifacts []struct {
					ID, CID, Digest, Name string
					Size                  int64
				} `json:"artifacts"`
			}{
				Artifacts: []struct {
					ID, CID, Digest, Name string
					Size                  int64
				}{
					{ID: "art-id-1", CID: "cid-abc123", Digest: "sha256:abcdef1234567890", Name: "artifact1", Size: int64(len(artifactContent))},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(listing)
		case strings.Contains(r.URL.Path, "/v1/artifacts/art-id-1"):
			// Artifact download endpoint.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(artifactContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	baseURL, _ := url.Parse(server.URL)
	out := &bytes.Buffer{}

	ctx := context.Background()
	err := downloadRunArtifacts(ctx, baseURL, server.Client(), runID.String(), tmpDir, out)
	if err != nil {
		t.Fatalf("downloadRunArtifacts failed: %v", err)
	}

	// Verify manifest.json was created.
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatalf("manifest.json not created")
	}

	// Parse manifest and verify contents.
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		Artifacts []struct {
			Stage  string `json:"stage"`
			Name   string `json:"name"`
			CID    string `json:"cid"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
			Path   string `json:"path"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact in manifest, got %d", len(manifest.Artifacts))
	}
	item := manifest.Artifacts[0]
	if item.Stage != stageID.String() {
		t.Errorf("expected %s, got %s", stageID.String(), item.Stage)
	}
	if item.Name != "artifact1" {
		t.Errorf("expected artifact1, got %s", item.Name)
	}
	if item.CID != "cid-abc123" {
		t.Errorf("expected cid-abc123, got %s", item.CID)
	}

	// Verify artifact file was downloaded.
	artifactData, err := os.ReadFile(item.Path)
	if err != nil {
		t.Fatalf("read artifact file: %v", err)
	}
	if string(artifactData) != string(artifactContent) {
		t.Errorf("artifact content mismatch: got %q, want %q", artifactData, artifactContent)
	}

	// Verify output message.
	if !strings.Contains(out.String(), "Downloaded 1 artifacts") {
		t.Errorf("expected download message in output, got: %s", out.String())
	}
}

// TestDownloadRunArtifactsMultipleStages verifies that artifacts from multiple
// stages are all downloaded correctly.
func TestDownloadRunArtifactsMultipleStages(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	planStageID := domaintypes.NewJobID()
	execStageID := domaintypes.NewJobID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.HasSuffix(r.URL.Path, "/status"):
			// Run status with multiple stages — return RunSummary directly.
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(migsapi.RunSummary{
				RunID: runID,
				State: migsapi.RunStateSucceeded,
				Stages: map[domaintypes.JobID]migsapi.StageStatus{
					planStageID: {
						State: migsapi.StageStateSucceeded,
						Artifacts: map[string]string{
							"plan.json": "cid-plan",
						},
					},
					execStageID: {
						State: migsapi.StageStateSucceeded,
						Artifacts: map[string]string{
							"output.log": "cid-exec",
						},
					},
				},
			})
		case strings.Contains(r.URL.Path, "/v1/artifacts") && r.URL.Query().Get("cid") == "cid-plan":
			listing := struct {
				Artifacts []struct {
					ID, CID, Digest, Name string
					Size                  int64
				} `json:"artifacts"`
			}{
				Artifacts: []struct {
					ID, CID, Digest, Name string
					Size                  int64
				}{
					{ID: "art-plan", CID: "cid-plan", Digest: "sha256:plan123", Name: "plan.json", Size: 10},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(listing)
		case strings.Contains(r.URL.Path, "/v1/artifacts") && r.URL.Query().Get("cid") == "cid-exec":
			listing := struct {
				Artifacts []struct {
					ID, CID, Digest, Name string
					Size                  int64
				} `json:"artifacts"`
			}{
				Artifacts: []struct {
					ID, CID, Digest, Name string
					Size                  int64
				}{
					{ID: "art-exec", CID: "cid-exec", Digest: "sha256:exec456", Name: "output.log", Size: 20},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(listing)
		case strings.Contains(r.URL.Path, "/v1/artifacts/art-plan"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("plan data"))
		case strings.Contains(r.URL.Path, "/v1/artifacts/art-exec"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("exec output"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	baseURL, _ := url.Parse(server.URL)
	out := &bytes.Buffer{}

	ctx := context.Background()
	err := downloadRunArtifacts(ctx, baseURL, server.Client(), runID.String(), tmpDir, out)
	if err != nil {
		t.Fatalf("downloadRunArtifacts failed: %v", err)
	}

	// Verify manifest contains both artifacts.
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	data, _ := os.ReadFile(manifestPath)
	var manifest struct {
		Artifacts []struct {
			Stage string `json:"stage"`
			Name  string `json:"name"`
		} `json:"artifacts"`
	}
	_ = json.Unmarshal(data, &manifest)
	if len(manifest.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(manifest.Artifacts))
	}

	// Verify output message shows correct count.
	if !strings.Contains(out.String(), "Downloaded 2 artifacts") {
		t.Errorf("expected 2 artifacts downloaded, got: %s", out.String())
	}
}

// TestDownloadRunArtifactsErrorHandling verifies error handling for various
// failure scenarios (status fetch failure, artifact not found, etc.).
func TestDownloadRunArtifactsErrorHandling(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()
	stageID := domaintypes.NewJobID()

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErrText string
	}{
		{
			name: "run status 404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.HasSuffix(r.URL.Path, "/status") {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"run not found"}`))
				}
			},
			wantErrText: "404",
		},
		{
			name: "artifact CID not found",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.HasSuffix(r.URL.Path, "/status") {
					// Return RunSummary directly.
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(migsapi.RunSummary{
						RunID: runID,
						Stages: map[domaintypes.JobID]migsapi.StageStatus{
							stageID: {Artifacts: map[string]string{"art": "missing-cid"}},
						},
					})
				} else if strings.Contains(r.URL.Path, "/v1/artifacts") {
					// Return empty artifact list.
					_, _ = w.Write([]byte(`{"artifacts":[]}`))
				}
			},
			wantErrText: "no artifact found for CID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			tmpDir := t.TempDir()
			baseURL, _ := url.Parse(server.URL)
			out := &bytes.Buffer{}

			ctx := context.Background()
			err := downloadRunArtifacts(ctx, baseURL, server.Client(), runID.String(), tmpDir, out)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrText)
			}
		})
	}
}

// TestBuildArtifactFilename verifies filename generation with various inputs.
func TestBuildArtifactFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		stage         string
		artName       string
		cid           string
		digest        string
		wantSubstring string
	}{
		{
			name:          "with digest",
			stage:         "plan",
			artName:       "output.json",
			cid:           "cid123",
			digest:        "sha256:abcdef1234567890abcdef",
			wantSubstring: "sha256-abcdef1234567",
		},
		{
			name:          "without digest uses cid",
			stage:         "exec",
			artName:       "logs.txt",
			cid:           "cid-xyz",
			digest:        "",
			wantSubstring: "cid-xyz_exec_logs.txt",
		},
		{
			name:          "sanitize slashes in name",
			stage:         "build",
			artName:       "dir/file.bin",
			cid:           "cid1",
			digest:        "",
			wantSubstring: "dir_file.bin",
		},
		{
			name:          "sanitize backslashes in name",
			stage:         "test",
			artName:       "win\\path\\file.dat",
			cid:           "cid2",
			digest:        "",
			wantSubstring: "win_path_file.dat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := buildArtifactFilename(tt.stage, tt.artName, tt.cid, tt.digest)
			if !strings.Contains(result, tt.wantSubstring) {
				t.Errorf("buildArtifactFilename(%q, %q, %q, %q) = %q, want substring %q",
					tt.stage, tt.artName, tt.cid, tt.digest, result, tt.wantSubstring)
			}
			// Verify .bin extension is always present.
			if !strings.HasSuffix(result, ".bin") {
				t.Errorf("expected .bin suffix, got %q", result)
			}
		})
	}
}

// TestFetchMRURL verifies MR URL extraction from run metadata.
func TestFetchMRURL(t *testing.T) {
	t.Parallel()
	runID := domaintypes.NewRunID()

	tests := []struct {
		name       string
		metadata   map[string]string
		wantURL    string
		wantErrNil bool
	}{
		{
			name:       "mr_url present",
			metadata:   map[string]string{"mr_url": "https://gitlab.com/org/repo/-/merge_requests/42"},
			wantURL:    "https://gitlab.com/org/repo/-/merge_requests/42",
			wantErrNil: true,
		},
		{
			name:       "mr_url missing",
			metadata:   map[string]string{"other_key": "value"},
			wantURL:    "",
			wantErrNil: true,
		},
		{
			name:       "nil metadata",
			metadata:   nil,
			wantURL:    "",
			wantErrNil: true,
		},
		{
			name:       "mr_url with whitespace trimmed",
			metadata:   map[string]string{"mr_url": "  https://example.com/mr/1  "},
			wantURL:    "https://example.com/mr/1",
			wantErrNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Return RunSummary directly — the canonical response shape.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(migsapi.RunSummary{
					RunID:    runID,
					Metadata: tt.metadata,
				})
			}))
			defer server.Close()

			baseURL, _ := url.Parse(server.URL)
			ctx := context.Background()
			gotURL, err := fetchMRURL(ctx, baseURL, server.Client(), runID.String())
			if (err == nil) != tt.wantErrNil {
				t.Errorf("fetchMRURL error = %v, wantErrNil %v", err, tt.wantErrNil)
			}
			if gotURL != tt.wantURL {
				t.Errorf("fetchMRURL = %q, want %q", gotURL, tt.wantURL)
			}
		})
	}
}

// TestFetchMRURLErrorHandling verifies error handling when status fetch fails.
func TestFetchMRURLErrorHandling(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)
	ctx := context.Background()
	_, err := fetchMRURL(ctx, baseURL, server.Client(), domaintypes.NewRunID().String())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not contain status code 500", err.Error())
	}
}

// TestDownloadRunArtifactsZeroArtifacts writes an empty manifest when no artifacts exist.
func TestDownloadRunArtifactsZeroArtifacts(t *testing.T) {
	t.Parallel()
	stageID := domaintypes.NewJobID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/runs/") && strings.HasSuffix(r.URL.Path, "/status") {
			// Return RunSummary directly — the canonical response shape.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(migsapi.RunSummary{Stages: map[domaintypes.JobID]migsapi.StageStatus{
				stageID: {Artifacts: map[string]string{}},
			}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	baseURL, _ := url.Parse(server.URL)
	out := &bytes.Buffer{}
	ctx := context.Background()
	if err := downloadRunArtifacts(ctx, baseURL, server.Client(), domaintypes.NewRunID().String(), tmpDir, out); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		Artifacts []any `json:"artifacts"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(manifest.Artifacts) != 0 {
		t.Fatalf("expected 0 artifacts, got %d", len(manifest.Artifacts))
	}
	if !strings.Contains(out.String(), "Downloaded 0 artifacts") {
		t.Fatalf("expected \"Downloaded 0 artifacts\" in output, got: %s", out.String())
	}
}
