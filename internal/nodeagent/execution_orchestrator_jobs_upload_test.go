package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
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
		artifactPaths []string
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
			// Uploaders are lazily initialized by ensureUploaders() when needed.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			typedOpts := RunOptions{
				Artifacts: ArtifactOptions{
					Paths: tt.artifactPaths,
					Name:  "test-artifact",
				},
			}

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
			controller.uploadConfiguredArtifacts(ctx, req, typedOpts, manifest, workspace, "")

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
			var uploadedBundle []byte

			// Mock server (job-scoped endpoint).
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Job-scoped artifact endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
				if r.URL.Path == "/v1/runs/test-run/jobs/test-stage/artifact" {
					uploadCalled = true
					var payload struct {
						Bundle []byte `json:"bundle"`
					}
					_ = json.NewDecoder(r.Body).Decode(&payload)
					uploadedBundle = payload.Bundle
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
			// Uploaders are lazily initialized by ensureUploaders() when needed.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
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
			if tt.wantUpload && len(tt.createFiles) > 0 {
				headers := tarHeadersFromBundle(t, uploadedBundle)
				if _, ok := headers["out/result.txt"]; !ok {
					t.Fatalf("expected /out upload to include out/result.txt, got headers=%v", keys(headers))
				}
				if _, ok := headers["out/subdir/output.log"]; !ok {
					t.Fatalf("expected /out upload to include out/subdir/output.log, got headers=%v", keys(headers))
				}
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
			// Uploaders are lazily initialized by ensureUploaders() when needed.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			// Execute upload with job_id.
			// v1 uses capitalized job status values: Success, Fail, Cancelled.
			ctx := context.Background()
			var exitCode int32 = 0
			stats := types.NewRunStatsBuilder().ExitCode(0).MustBuild()
			err := controller.uploadStatus(ctx, "test-run", JobStatusSuccess.String(), &exitCode, stats, "test-job-id")

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
			// Uploaders are lazily initialized by ensureUploaders() when needed.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
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

// Note: uploadDiff and associated mig diff metadata tests were removed along
// with legacy HEAD-based diff generation. Mods now use baseline-aware
// GenerateBetween semantics via dedicated helpers.

// TestIsValidArtifactPath verifies path traversal prevention for artifact paths.
// This is a security test ensuring malicious paths like "../../etc/passwd" are rejected.
func TestIsValidArtifactPath(t *testing.T) {
	t.Parallel()

	// Use a realistic workspace path for testing.
	workspace := "/workspace"

	tests := []struct {
		name         string
		artifactPath string
		workspace    string
		wantValid    bool
	}{
		// Valid paths — should be accepted.
		{
			name:         "simple relative path",
			artifactPath: "file.txt",
			workspace:    workspace,
			wantValid:    true,
		},
		{
			name:         "nested relative path",
			artifactPath: "dir/subdir/file.txt",
			workspace:    workspace,
			wantValid:    true,
		},
		{
			name:         "path with safe relative component",
			artifactPath: "dir/../file.txt",
			workspace:    workspace,
			wantValid:    true, // Resolves to /workspace/file.txt, still inside workspace
		},
		{
			name:         "path with dot",
			artifactPath: "./file.txt",
			workspace:    workspace,
			wantValid:    true,
		},

		// Invalid paths — should be rejected.
		{
			name:         "path traversal with ../../",
			artifactPath: "../../etc/passwd",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "path traversal with multiple ..",
			artifactPath: "../../../var/log/secret.log",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "path traversal at start",
			artifactPath: "../sibling/file.txt",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "absolute path unix style",
			artifactPath: "/etc/passwd",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "absolute path to sensitive file",
			artifactPath: "/root/.ssh/id_rsa",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "empty path",
			artifactPath: "",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "whitespace only path",
			artifactPath: "   ",
			workspace:    workspace,
			wantValid:    false,
		},
		{
			name:         "nested traversal escaping workspace",
			artifactPath: "subdir/../../secrets.txt",
			workspace:    workspace,
			wantValid:    false, // Resolves to /secrets.txt, outside workspace
		},
		{
			name:         "deep nested traversal",
			artifactPath: "a/b/c/../../../../etc/hosts",
			workspace:    workspace,
			wantValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isValidArtifactPath(tt.artifactPath, tt.workspace)
			if got != tt.wantValid {
				t.Errorf("isValidArtifactPath(%q, %q) = %v, want %v",
					tt.artifactPath, tt.workspace, got, tt.wantValid)
			}
		})
	}
}

// TestRunController_uploadConfiguredArtifacts_PathTraversal verifies that
// path traversal attempts in artifact_paths are rejected and do not trigger uploads.
// Security: prevents uploads of files outside the workspace via traversal/absolute paths.
func TestRunController_uploadConfiguredArtifacts_PathTraversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		artifactPaths []string
		createFiles   []string // Files to create relative to workspace
		wantUpload    bool     // Whether upload should be called at all
		description   string   // Security context
	}{
		{
			name:          "path traversal ../../etc/passwd rejected",
			artifactPaths: []string{"../../etc/passwd"},
			createFiles:   []string{}, // Nothing to create — the path would be outside workspace
			wantUpload:    false,
			description:   "Malicious path attempting to exfiltrate /etc/passwd should be rejected",
		},
		{
			name:          "mixed valid and traversal paths",
			artifactPaths: []string{"valid.txt", "../../etc/passwd", "another_valid.txt"},
			createFiles:   []string{"valid.txt", "another_valid.txt"},
			wantUpload:    true, // Valid paths should still be uploaded
			description:   "Valid paths should be uploaded even when mixed with invalid paths",
		},
		{
			name:          "absolute path /etc/hosts rejected",
			artifactPaths: []string{"/etc/hosts"},
			createFiles:   []string{},
			wantUpload:    false,
			description:   "Absolute paths should be rejected to prevent arbitrary file access",
		},
		{
			name:          "all paths are traversal attempts",
			artifactPaths: []string{"../secret", "../../etc/shadow", "../../../root/.bashrc"},
			createFiles:   []string{},
			wantUpload:    false,
			description:   "When all paths are invalid, no upload should occur",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uploadCalled := false

			// Mock server (job-scoped endpoint).
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Job-scoped artifact endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
				if r.URL.Path == "/v1/runs/test-run-traversal/jobs/test-job-traversal/artifact" {
					uploadCalled = true
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"artifact_bundle_id": "test-id",
						"cid":                "test-cid",
					})
				}
			}))
			defer server.Close()

			// Create workspace with test files.
			workspace, err := os.MkdirTemp("", "ploy-test-traversal-*")
			if err != nil {
				t.Fatalf("failed to create workspace: %v", err)
			}
			defer func() { _ = os.RemoveAll(workspace) }()

			// Create files that should be uploadable.
			for _, f := range tt.createFiles {
				fullPath := filepath.Join(workspace, f)
				if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				if err := os.WriteFile(fullPath, []byte("test content"), 0644); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
			}

			// Initialize test infrastructure.
			// Uploaders are lazily initialized by ensureUploaders() when needed.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			controller := &runController{cfg: cfg}

			typedOpts := RunOptions{
				Artifacts: ArtifactOptions{
					Paths: tt.artifactPaths,
					Name:  "test-artifact",
				},
			}

			req := StartRunRequest{
				RunID:        types.RunID("test-run-traversal"),
				JobID:        "test-job-traversal",
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
			controller.uploadConfiguredArtifacts(ctx, req, typedOpts, manifest, workspace, "")

			// Verify upload call matches expectation.
			if uploadCalled != tt.wantUpload {
				t.Errorf("[%s] uploadCalled = %v, want %v", tt.description, uploadCalled, tt.wantUpload)
			}
		})
	}
}

func TestRunController_uploadConfiguredArtifacts_ResolvesOutPathDeterministically(t *testing.T) {
	t.Parallel()

	uploadCalled := false
	var uploadedBundle []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/runs/test-run-out/jobs/test-job-out/artifact" {
			uploadCalled = true
			var payload struct {
				Bundle []byte `json:"bundle"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			uploadedBundle = payload.Bundle
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"artifact_bundle_id": "test-id",
				"cid":                "test-cid",
			})
		}
	}))
	defer server.Close()

	workspace := t.TempDir()
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "prep-profile-candidate.json"), []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	controller := &runController{cfg: cfg}

	typedOpts := RunOptions{
		Artifacts: ArtifactOptions{
			Paths: []string{"/out/prep-profile-candidate.json"},
			Name:  "test-artifact",
		},
	}
	req := StartRunRequest{
		RunID:        "test-run-out",
		JobID:        "test-job-out",
		TypedOptions: typedOpts,
	}
	manifest := contracts.StepManifest{
		Image:   "test-image",
		Command: []string{"test"},
	}

	controller.uploadConfiguredArtifacts(context.Background(), req, typedOpts, manifest, workspace, outDir)
	if !uploadCalled {
		t.Fatal("expected upload to be called for /out artifact path")
	}

	headers := tarHeadersFromBundle(t, uploadedBundle)
	if _, ok := headers["out/prep-profile-candidate.json"]; !ok {
		t.Fatalf("expected header out/prep-profile-candidate.json, got %v", keys(headers))
	}
}

func tarHeadersFromBundle(t *testing.T, bundle []byte) map[string]struct{} {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	headers := map[string]struct{}{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		headers[hdr.Name] = struct{}{}
	}
	return headers
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
