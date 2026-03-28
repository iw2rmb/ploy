package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestRunController_uploadConfiguredArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		artifactPaths []string
		createFiles   []string
		wantUpload    bool
		outDirFiles   []string
		wantHeaders   []string
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
			wantUpload:    false,
		},
		{
			name:          "empty paths skip upload",
			artifactPaths: []string{"", "  "},
			wantUpload:    false,
		},
		{
			name:          "nil artifact_paths skip upload",
			artifactPaths: nil,
			wantUpload:    false,
		},
		{
			name:          "path traversal ../../etc/passwd rejected",
			artifactPaths: []string{"../../etc/passwd"},
			wantUpload:    false,
		},
		{
			name:          "mixed valid and traversal paths",
			artifactPaths: []string{"valid.txt", "../../etc/passwd", "another_valid.txt"},
			createFiles:   []string{"valid.txt", "another_valid.txt"},
			wantUpload:    true,
		},
		{
			name:          "absolute path /etc/hosts rejected",
			artifactPaths: []string{"/etc/hosts"},
			wantUpload:    false,
		},
		{
			name:          "all paths are traversal attempts",
			artifactPaths: []string{"../secret", "../../etc/shadow", "../../../root/.bashrc"},
			wantUpload:    false,
		},
		{
			name:          "resolves /out path deterministically",
			artifactPaths: []string{"/out/gate-profile-candidate.json"},
			wantUpload:    true,
			outDirFiles:   []string{"gate-profile-candidate.json"},
			wantHeaders:   []string{"out/gate-profile-candidate.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, cap := newArtifactUploadServer(t, "test-run-123", "test-job-id")
			controller := newTestController(t, newTestConfig(server.URL))

			workspace := t.TempDir()
			populateTestFiles(t, workspace, tt.createFiles, "test")

			outDir := ""
			if len(tt.outDirFiles) > 0 {
				outDir = t.TempDir()
				populateTestFiles(t, outDir, tt.outDirFiles, "test")
			}

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

			controller.uploadConfiguredArtifacts(context.Background(), req, typedOpts, manifest, workspace, outDir)

			if cap.Called != tt.wantUpload {
				t.Errorf("uploadCalled = %v, want %v", cap.Called, tt.wantUpload)
			}
			if tt.wantUpload && cap.Name != "test-artifact" {
				t.Errorf("artifact name = %q, want %q", cap.Name, "test-artifact")
			}
			if len(tt.wantHeaders) > 0 {
				headers := tarHeadersFromBundle(t, cap.Bundle)
				for _, h := range tt.wantHeaders {
					if _, ok := headers[h]; !ok {
						t.Fatalf("expected header %q, got %v", h, keys(headers))
					}
				}
			}
		})
	}
}

func TestRunController_uploadOutDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		createFiles []string
		wantUpload  bool
		wantErr     bool
		emptyOutDir bool
		bundleName  string
		wantHeaders []string
	}{
		{
			name:        "directory with files triggers upload",
			createFiles: []string{"result.txt", "subdir/output.log"},
			wantUpload:  true,
			wantHeaders: []string{"out/result.txt", "out/subdir/output.log"},
		},
		{
			name:       "empty directory skips upload",
			wantUpload: false,
		},
		{
			name:        "empty outDir string skips upload",
			emptyOutDir: true,
			wantUpload:  false,
		},
		{
			name:        "custom bundle name",
			createFiles: []string{"nested/artifact.txt"},
			wantUpload:  true,
			bundleName:  "build-gate-out",
			wantHeaders: []string{"out/nested/artifact.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, cap := newArtifactUploadServer(t, "test-run", "test-stage")
			controller := newTestController(t, newTestConfig(server.URL))

			outDir := ""
			if !tt.emptyOutDir {
				outDir = t.TempDir()
				populateTestFiles(t, outDir, tt.createFiles, "test output")
			}

			ctx := context.Background()
			var err error
			if tt.bundleName != "" {
				err = controller.uploadOutDirBundle(ctx, "test-run", "test-stage", outDir, tt.bundleName)
			} else {
				err = controller.uploadOutDir(ctx, "test-run", "test-stage", outDir)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("upload error = %v, wantErr %v", err, tt.wantErr)
			}
			if cap.Called != tt.wantUpload {
				t.Errorf("uploadCalled = %v, want %v", cap.Called, tt.wantUpload)
			}
			if tt.bundleName != "" && cap.Name != tt.bundleName {
				t.Errorf("artifact name = %q, want %q", cap.Name, tt.bundleName)
			}
			if len(tt.wantHeaders) > 0 {
				headers := tarHeadersFromBundle(t, cap.Bundle)
				for _, h := range tt.wantHeaders {
					if _, ok := headers[h]; !ok {
						t.Fatalf("expected header %q, got %v", h, keys(headers))
					}
				}
			}
		})
	}
}

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

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/jobs/test-job-id/complete" {
					w.WriteHeader(tt.serverStatus)
				}
			}))
			defer server.Close()

			controller := newTestController(t, newTestConfig(server.URL))

			ctx := context.Background()
			var exitCode int32 = 0
			stats := types.NewRunStatsBuilder().ExitCode(0).MustBuild()
			err := controller.uploadStatus(ctx, "test-run", types.JobStatusSuccess.String(), &exitCode, stats, "test-job-id")

			if (err != nil) != tt.wantErr {
				t.Errorf("uploadStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

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

			server, cap := newArtifactUploadServer(t, "test-run", "test-stage", withArtifactStatus(tt.serverStatus))
			controller := newTestController(t, newTestConfig(server.URL))

			phase := &types.RunStatsGatePhase{}
			controller.uploadGateLogsArtifact("test-run", "test-stage", tt.logsText, tt.artifactSuffix, phase)

			if tt.wantArtifactID {
				if phase.LogsArtifactID == "" {
					t.Error("LogsArtifactID not set in gate phase")
				}
				if phase.LogsBundleCID == "" {
					t.Error("LogsBundleCID not set in gate phase")
				}
				if cap.Name != tt.wantArtifactName {
					t.Errorf("artifact name = %q, want %q", cap.Name, tt.wantArtifactName)
				}
			} else {
				if phase.LogsArtifactID != "" {
					t.Error("LogsArtifactID should not be set on upload failure")
				}
			}
		})
	}
}

func TestRunController_uploadGateReportArtifacts_Gradle(t *testing.T) {
	t.Parallel()

	server, calls := newArtifactUploadServerMulti(t, "test-run", "test-gate")
	controller := newTestController(t, newTestConfig(server.URL))

	workspace := t.TempDir()
	outDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	populateTestFiles(t, outDir, []string{"gradle-test-results/TEST-Example.xml"}, `<testsuite/>`)
	populateTestFiles(t, outDir, []string{"gradle-test-report/index.html"}, `<html/>`)

	meta := &contracts.BuildGateStageMetadata{
		Detected: &contracts.StackExpectation{
			Language: "java",
			Tool:     "gradle",
			Release:  "17",
		},
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: "java",
			Tool:     "gradle",
			Passed:   true,
		}},
	}

	controller.uploadGateReportArtifacts(context.Background(), "test-run", "test-gate", workspace, meta)

	if len(*calls) != 2 {
		t.Fatalf("upload call count = %d, want 2", len(*calls))
	}
	if len(meta.ReportLinks) != 2 {
		t.Fatalf("report_links count = %d, want 2", len(meta.ReportLinks))
	}

	wantNames := map[string]bool{
		"build-gate-gradle-junit-xml":   false,
		"build-gate-gradle-html-report": false,
	}
	for _, c := range *calls {
		if _, ok := wantNames[c.Name]; !ok {
			t.Fatalf("unexpected artifact name %q", c.Name)
		}
		wantNames[c.Name] = true
		headers := tarHeadersFromBundle(t, c.Bundle)
		if c.Name == "build-gate-gradle-junit-xml" {
			if _, ok := headers["out/gradle-test-results/TEST-Example.xml"]; !ok {
				t.Fatalf("junit bundle missing expected file, headers=%v", keys(headers))
			}
		}
		if c.Name == "build-gate-gradle-html-report" {
			if _, ok := headers["out/gradle-test-report/index.html"]; !ok {
				t.Fatalf("html bundle missing expected file, headers=%v", keys(headers))
			}
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Fatalf("expected artifact upload name %q", name)
		}
	}

	foundJUnit := false
	foundHTML := false
	for _, link := range meta.ReportLinks {
		if link.Type == contracts.BuildGateReportTypeGradleJUnitXML {
			foundJUnit = true
			if link.Path != "/out/gradle-test-results" {
				t.Fatalf("junit link path = %q, want /out/gradle-test-results", link.Path)
			}
		}
		if link.Type == contracts.BuildGateReportTypeGradleHTML {
			foundHTML = true
			if link.Path != "/out/gradle-test-report" {
				t.Fatalf("html link path = %q, want /out/gradle-test-report", link.Path)
			}
		}
		if link.ArtifactID == "" || link.BundleCID == "" || link.URL == "" || link.DownloadURL == "" {
			t.Fatalf("expected populated link fields, got %+v", link)
		}
	}
	if !foundJUnit || !foundHTML {
		t.Fatalf("missing expected report link types: %+v", meta.ReportLinks)
	}
}

func TestRunController_uploadGateReportArtifacts_NonGradleIgnored(t *testing.T) {
	t.Parallel()

	server, cap := newArtifactUploadServer(t, "test-run", "test-gate")
	controller := newTestController(t, newTestConfig(server.URL))

	meta := &contracts.BuildGateStageMetadata{
		Detected: &contracts.StackExpectation{
			Language: "java",
			Tool:     "maven",
		},
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Tool:   "maven",
			Passed: true,
		}},
	}

	controller.uploadGateReportArtifacts(context.Background(), "test-run", "test-gate", t.TempDir(), meta)

	if cap.Called {
		t.Fatal("expected no upload for non-gradle gate")
	}
	if len(meta.ReportLinks) != 0 {
		t.Fatalf("expected no report links for non-gradle gate, got %+v", meta.ReportLinks)
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
