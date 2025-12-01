package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// TestRunController_uploadDiff verifies diff generation and upload to both diff and artifact endpoints.
func TestRunController_uploadDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		diffContent    string
		wantDiffUpload bool
		wantArtUpload  bool
	}{
		{
			name:           "non-empty diff triggers uploads",
			diffContent:    "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n",
			wantDiffUpload: true,
			wantArtUpload:  true,
		},
		{
			name:           "empty diff skips uploads",
			diffContent:    "",
			wantDiffUpload: false,
			wantArtUpload:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			diffUploadCalled := false
			artUploadCalled := false

			// Mock server that handles both diff and artifact endpoints (job-scoped).
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/diff" {
					diffUploadCalled = true
					w.WriteHeader(http.StatusCreated)
				} else if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/artifact" {
					artUploadCalled = true
					w.WriteHeader(http.StatusCreated)
					// Verify artifact name is "diff" for diff bundle.
					var payload map[string]any
					_ = json.NewDecoder(r.Body).Decode(&payload)
					if name, ok := payload["name"].(string); !ok || name != "diff" {
						t.Errorf("artifact name = %v, want diff", payload["name"])
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"artifact_bundle_id": "test-id", "cid": "test-cid"})
				} else {
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			// Create a temporary workspace with a test file.
			workspace, err := os.MkdirTemp("", "ploy-test-workspace-*")
			if err != nil {
				t.Fatalf("failed to create workspace: %v", err)
			}
			defer os.RemoveAll(workspace)

			// Initialize test infrastructure.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node",
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			// Create a mock diff generator.
			diffGen := &mockDiffGenerator{diffContent: tt.diffContent}

			// Build test result.
			result := step.Result{
				ExitCode: 0,
				Timings: step.StageTiming{
					HydrationDuration: 100 * time.Millisecond,
					ExecutionDuration: 200 * time.Millisecond,
					BuildGateDuration: 50 * time.Millisecond,
					DiffDuration:      10 * time.Millisecond,
					TotalDuration:     360 * time.Millisecond,
				},
			}

			// Execute upload.
			ctx := context.Background()
			controller.uploadDiff(ctx, "test-run", "test-stage", diffGen, workspace, result)

			// Verify expected upload calls.
			if diffUploadCalled != tt.wantDiffUpload {
				t.Errorf("diffUploadCalled = %v, want %v", diffUploadCalled, tt.wantDiffUpload)
			}
			if artUploadCalled != tt.wantArtUpload {
				t.Errorf("artUploadCalled = %v, want %v", artUploadCalled, tt.wantArtUpload)
			}
		})
	}
}

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
			defer os.RemoveAll(workspace)

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

			req := StartRunRequest{
				RunID: types.RunID("test-run-123"),
				JobID: "test-job-id",
				Options: map[string]interface{}{
					"artifact_paths": tt.artifactPaths,
				},
			}

			manifest := contracts.StepManifest{
				Image:   "test-image",
				Command: []string{"test"},
				Options: map[string]interface{}{
					"job_id":        "test-job",
					"artifact_name": "test-artifact",
				},
			}

			// Execute upload.
			ctx := context.Background()
			controller.uploadConfiguredArtifacts(ctx, req, manifest, workspace)

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
				defer os.RemoveAll(outDir)

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
				if r.URL.Path == "/v1/nodes/test-node/complete" {
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
			ctx := context.Background()
			var exitCode int32 = 0
			stats := map[string]interface{}{"exit_code": 0}
			err := controller.uploadStatus(ctx, "test-run", "succeeded", &exitCode, stats, 1000, "test-job-id")

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

			// Build gate stats map to receive artifact IDs.
			gateStats := map[string]any{}

			// Execute upload.
			controller.uploadGateLogsArtifact("test-run", "test-stage", tt.logsText, tt.artifactSuffix, gateStats)

			// Verify artifact ID attachment.
			if tt.wantArtifactID {
				if _, ok := gateStats["logs_artifact_id"]; !ok {
					t.Error("logs_artifact_id not set in gateStats")
				}
				if _, ok := gateStats["logs_bundle_cid"]; !ok {
					t.Error("logs_bundle_cid not set in gateStats")
				}
			} else {
				if _, ok := gateStats["logs_artifact_id"]; ok {
					t.Error("logs_artifact_id should not be set on upload failure")
				}
			}
		})
	}
}

// TestRunController_uploadDiff_ModTypeMetadata verifies that mod step diffs
// are tagged with mod_type="mod" to distinguish from other diff types.
func TestRunController_uploadDiff_ModTypeMetadata(t *testing.T) {
	t.Parallel()

	var capturedSummary types.DiffSummary

	// Mock server that captures the diff summary for validation (job-scoped endpoints).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/diff" {
			var payload struct {
				Patch   string            `json:"patch"`
				Summary types.DiffSummary `json:"summary"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			capturedSummary = payload.Summary
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"diff_id": "test-diff-id"})
		} else if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/artifact" {
			// Artifact endpoint (diff artifact bundle)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"artifact_bundle_id": "test-id", "cid": "test-cid"})
		}
	}))
	defer server.Close()

	// Create workspace.
	workspace, err := os.MkdirTemp("", "ploy-test-workspace-*")
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	defer os.RemoveAll(workspace)

	// Initialize test infrastructure.
	cfg := Config{
		ServerURL: server.URL,
		NodeID:    "test-node",
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}

	controller := &runController{cfg: cfg}

	// Create a mock diff generator with sample diff.
	diffGen := &mockDiffGenerator{
		diffContent: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n",
	}

	// Build test result.
	result := step.Result{
		ExitCode: 0,
		Timings: step.StageTiming{
			HydrationDuration: 100 * time.Millisecond,
			ExecutionDuration: 200 * time.Millisecond,
			BuildGateDuration: 50 * time.Millisecond,
			DiffDuration:      10 * time.Millisecond,
			TotalDuration:     360 * time.Millisecond,
		},
	}

	// Execute upload.
	ctx := context.Background()
	controller.uploadDiff(ctx, "test-run", "test-stage", diffGen, workspace, result)

	// Verify that mod_type is set to "mod".
	modType, ok := capturedSummary["mod_type"].(string)
	if !ok {
		t.Errorf("mod_type field missing or not a string in summary: %+v", capturedSummary)
	}
	if modType != "mod" {
		t.Errorf("mod_type = %q, want \"mod\"", modType)
	}

	// Verify other fields are present.
	if _, ok := capturedSummary["exit_code"]; !ok {
		t.Errorf("exit_code field missing in summary")
	}
	if _, ok := capturedSummary["timings"]; !ok {
		t.Errorf("timings field missing in summary")
	}
}

// mockDiffGenerator is a test helper that returns pre-configured diff content.
type mockDiffGenerator struct {
	diffContent string
	err         error
}

// Generate implements step.DiffGenerator interface.
func (m *mockDiffGenerator) Generate(ctx context.Context, workspace string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return []byte(m.diffContent), nil
}

// GenerateBetween implements step.DiffGenerator interface.
func (m *mockDiffGenerator) GenerateBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return []byte(m.diffContent), nil
}
