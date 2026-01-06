package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestRunController_uploadConfiguredArtifacts verifies artifact path resolution and upload.
func TestRunController_uploadConfiguredArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		artifactPaths interface{}
		createFiles   []string
		wantUpload    bool
	}{
		{
			name:          "valid paths trigger upload",
			artifactPaths: []string{"file1.txt", "dir1/file2.txt"},
			createFiles:   []string{"file1.txt", "dir1/file2.txt"},
			wantUpload:    true,
		},
		{
			name:          "JSON array format",
			artifactPaths: []any{"file1.txt"},
			createFiles:   []string{"file1.txt"},
			wantUpload:    true,
		},
		{
			name:          "missing paths skip upload",
			artifactPaths: []string{"missing.txt"},
			createFiles:   []string{},
			wantUpload:    false,
		},
		{
			name:          "empty paths skip upload",
			artifactPaths: []string{"", "  "},
			createFiles:   []string{},
			wantUpload:    false,
		},
		{
			name:          "nil artifact_paths skip upload",
			artifactPaths: nil,
			createFiles:   []string{},
			wantUpload:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uploadCalled := false

			// Mock server (job-scoped endpoint).
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Job-scoped artifact endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
				if r.URL.Path == "/v1/runs/test-run-123/jobs/test-job-id/artifact" {
					uploadCalled = true
					// Verify name matches manifest OptionString("artifact_name").
					var payload map[string]any
					_ = json.NewDecoder(r.Body).Decode(&payload)
					if name, ok := payload["name"].(string); !ok || name != "test-artifact" {
						t.Errorf("artifact name = %v, want test-artifact", payload["name"])
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]string{"artifact_bundle_id": "test-id", "cid": "test-cid"})
				}
			}))
			defer server.Close()

			// Create workspace with test files.
			workspace, err := os.MkdirTemp("", "ploy-test-workspace-*")
			if err != nil {
				t.Fatalf("failed to create workspace: %v", err)
			}
			defer func() { _ = os.RemoveAll(workspace) }()

			for _, f := range tt.createFiles {
				fullPath := filepath.Join(workspace, f)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte("test"), 0644); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
			}

			// Initialize test infrastructure.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			// Build StartRunRequest with artifact_paths in Options for parseRunOptions.
			opts := map[string]any{}
			if tt.artifactPaths != nil {
				opts["artifact_paths"] = tt.artifactPaths
			}
			opts["artifact_name"] = "test-artifact"

			// Parse options into typed RunOptions.
			typedOpts := parseRunOptions(opts)

			req := StartRunRequest{
				RunID:        types.RunID("test-run-123"),
				JobID:        "test-job-id",
				TypedOptions: typedOpts,
			}

			manifest := contracts.StepManifest{
				Image:   "test-image",
				Command: []string{"test"},
				Options: map[string]interface{}{
					"job_id":        "test-job",
					"artifact_name": "test-artifact",
				},
			}

			// Execute upload with typed RunOptions.
			ctx := context.Background()
			controller.uploadConfiguredArtifacts(ctx, req, typedOpts, manifest, workspace)

			// Verify upload call.
			if uploadCalled != tt.wantUpload {
				t.Errorf("uploadCalled = %v, want %v", uploadCalled, tt.wantUpload)
			}
		})
	}
}

// TestRunController_uploadOutDir verifies /out directory bundling and upload.
func TestRunController_uploadOutDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		createFiles []string
		wantUpload  bool
		wantErr     bool
		emptyOutDir bool
		nilOutDir   bool
	}{
		{
			name:        "directory with files triggers upload",
			createFiles: []string{"result.txt", "subdir/output.log"},
			wantUpload:  true,
			wantErr:     false,
		},
		{
			name:        "empty directory skips upload",
			createFiles: []string{},
			wantUpload:  false,
			wantErr:     false,
		},
		{
			name:        "empty outDir string skips upload",
			emptyOutDir: true,
			wantUpload:  false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uploadCalled := false

			// Mock server (job-scoped endpoint).
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Job-scoped artifact endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
				if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/artifact" {
					uploadCalled = true
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]string{"artifact_bundle_id": "test-id", "cid": "test-cid"})
				}
			}))
			defer server.Close()

			// Create /out directory with test files.
			outDir := ""
			if !tt.emptyOutDir {
				var err error
				outDir, err = os.MkdirTemp("", "ploy-test-out-*")
				if err != nil {
					t.Fatalf("failed to create out dir: %v", err)
				}
				defer func() { _ = os.RemoveAll(outDir) }()

				for _, f := range tt.createFiles {
					fullPath := filepath.Join(outDir, f)
					if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
						t.Fatalf("failed to create dir: %v", err)
					}
					if err := os.WriteFile(fullPath, []byte("test output"), 0644); err != nil {
						t.Fatalf("failed to create file: %v", err)
					}
				}
			}

			// Initialize test infrastructure.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			// Execute upload.
			ctx := context.Background()
			err := controller.uploadOutDir(ctx, "test-run", "test-stage", outDir)

			// Verify error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("uploadOutDir() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify upload call.
			if uploadCalled != tt.wantUpload {
				t.Errorf("uploadCalled = %v, want %v", uploadCalled, tt.wantUpload)
			}
		})
	}
}

// TestRunController_uploadStatus verifies status upload with retry logic.
func TestRunController_uploadStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		serverStatus int
		wantErr      bool
	}{
		{
			name:         "successful upload (200 OK)",
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "successful upload (204 No Content)",
			serverStatus: http.StatusNoContent,
			wantErr:      false,
		},
		{
			name:         "client error (400 Bad Request)",
			serverStatus: http.StatusBadRequest,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Mock server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/jobs/test-job-id/complete" {
					w.WriteHeader(tt.serverStatus)
				}
			}))
			defer server.Close()

			// Initialize test infrastructure.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			// Execute upload with job_id.
			// v1 uses capitalized job status values: Success, Fail, Cancelled.
			ctx := context.Background()
			var exitCode int32 = 0
			stats := types.NewRunStatsBuilder().ExitCode(0).MustBuild()
			err := controller.uploadStatus(ctx, "test-run", "Success", &exitCode, stats, 1000, "test-job-id")

			// Verify error expectation.
			if (err != nil) != tt.wantErr {
				t.Errorf("uploadStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRunController_uploadGateLogsArtifact verifies gate log artifact upload with ID attachment.
func TestRunController_uploadGateLogsArtifact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		logsText         string
		artifactSuffix   string
		wantArtifactName string
		serverStatus     int
		wantArtifactID   bool
	}{
		{
			name:             "final gate logs",
			logsText:         "gate logs content",
			artifactSuffix:   "",
			wantArtifactName: "build-gate.log",
			serverStatus:     http.StatusCreated,
			wantArtifactID:   true,
		},
		{
			name:             "pre-gate logs",
			logsText:         "pre-gate logs content",
			artifactSuffix:   "pre",
			wantArtifactName: "build-gate-pre.log",
			serverStatus:     http.StatusCreated,
			wantArtifactID:   true,
		},
		{
			name:             "re-gate logs",
			logsText:         "re-gate logs content",
			artifactSuffix:   "re1",
			wantArtifactName: "build-gate-re1.log",
			serverStatus:     http.StatusCreated,
			wantArtifactID:   true,
		},
		{
			name:             "upload failure does not set IDs",
			logsText:         "logs content",
			artifactSuffix:   "",
			wantArtifactName: "build-gate.log",
			serverStatus:     http.StatusInternalServerError,
			wantArtifactID:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Mock server (job-scoped endpoint).
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Job-scoped artifact endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
				if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/artifact" {
					w.WriteHeader(tt.serverStatus)
					if tt.serverStatus == http.StatusCreated {
						_ = json.NewEncoder(w).Encode(map[string]string{
							"artifact_bundle_id": "test-artifact-id",
							"cid":                "test-cid",
						})
					}
				}
			}))
			defer server.Close()

			// Initialize test infrastructure.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			phase := &types.RunStatsGatePhase{}

			// Execute upload.
			controller.uploadGateLogsArtifact("test-run", "test-stage", tt.logsText, tt.artifactSuffix, phase)

			// Verify artifact ID attachment.
			if tt.wantArtifactID {
				if phase.LogsArtifactID == "" {
					t.Error("LogsArtifactID not set in gate phase")
				}
				if phase.LogsBundleCID == "" {
					t.Error("LogsBundleCID not set in gate phase")
				}
			} else {
				if phase.LogsArtifactID != "" {
					t.Error("LogsArtifactID should not be set on upload failure")
				}
			}
		})
	}
}

// Note: uploadDiff and associated mod diff metadata tests were removed along
// with legacy HEAD-based diff generation. Mods now use baseline-aware
// GenerateBetween semantics via dedicated helpers.
