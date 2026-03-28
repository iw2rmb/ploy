package nodeagent

import (
	"context"
	"net/http"
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

			server, calls := newArtifactUploadServer(t, "test-run-123", "test-job-id")
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
			req := newStartRunRequest(
				withRunID("test-run-123"),
				withJobID("test-job-id"),
				withRunOptions(typedOpts),
			)
			manifest := contracts.StepManifest{
				Image:   "test-image",
				Command: []string{"test"},
				Options: map[string]interface{}{
					"job_id":        "test-job",
					"artifact_name": "test-artifact",
				},
			}

			controller.uploadConfiguredArtifacts(context.Background(), req, typedOpts, manifest, workspace, outDir)

			assertUploadOccurred(t, calls, tt.wantUpload)
			if tt.wantUpload {
				assertArtifactName(t, calls, 0, "test-artifact")
			}
			if len(tt.wantHeaders) > 0 {
				assertTarContains(t, (*calls)[0].Bundle, tt.wantHeaders)
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

			server, calls := newArtifactUploadServer(t, "test-run", "test-stage")
			controller := newTestController(t, newTestConfig(server.URL))

			outDir := ""
			if !tt.emptyOutDir {
				outDir = t.TempDir()
				populateTestFiles(t, outDir, tt.createFiles, "test output")
			}

			bundleName := tt.bundleName
			if bundleName == "" {
				bundleName = "mig-out"
			}
			err := controller.uploadOutDirBundle(context.Background(), "test-run", "test-stage", outDir, bundleName)

			checkErr(t, tt.wantErr, err)
			assertUploadOccurred(t, calls, tt.wantUpload)
			if tt.bundleName != "" && len(*calls) > 0 {
				assertArtifactName(t, calls, 0, tt.bundleName)
			}
			if len(tt.wantHeaders) > 0 {
				assertTarContains(t, (*calls)[0].Bundle, tt.wantHeaders)
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

			server, _ := newStatusCaptureServer(t, "test-job-id", withStatusHTTPCode(tt.serverStatus))
			controller := newTestController(t, newTestConfig(server.URL))

			var exitCode int32 = 0
			stats := types.NewRunStatsBuilder().ExitCode(0).MustBuild()
			err := controller.uploadStatus(context.Background(), "test-run", types.JobStatusSuccess.String(), &exitCode, stats, "test-job-id")

			checkErr(t, tt.wantErr, err)
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

			server, calls := newArtifactUploadServer(t, "test-run", "test-stage", withArtifactStatus(tt.serverStatus))
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
				assertArtifactName(t, calls, 0, tt.wantArtifactName)
			} else {
				if phase.LogsArtifactID != "" {
					t.Error("LogsArtifactID should not be set on upload failure")
				}
			}
		})
	}
}

func TestRunController_uploadGateReportArtifacts(t *testing.T) {
	t.Parallel()

	type wantLink struct {
		Type string
		Path string
	}

	tests := []struct {
		name            string
		tool            string
		language        string
		outDirFiles     map[string]string // path -> content
		wantUploadCount int
		wantNames       []string
		wantLinks       []wantLink
	}{
		{
			name:     "gradle gate uploads junit xml and html report",
			tool:     "gradle",
			language: "java",
			outDirFiles: map[string]string{
				"gradle-test-results/TEST-Example.xml": "<testsuite/>",
				"gradle-test-report/index.html":        "<html/>",
			},
			wantUploadCount: 2,
			wantNames: []string{
				"build-gate-gradle-junit-xml",
				"build-gate-gradle-html-report",
			},
			wantLinks: []wantLink{
				{Type: contracts.BuildGateReportTypeGradleJUnitXML, Path: "/out/gradle-test-results"},
				{Type: contracts.BuildGateReportTypeGradleHTML, Path: "/out/gradle-test-report"},
			},
		},
		{
			name:            "non-gradle gate produces no uploads",
			tool:            "maven",
			language:        "java",
			wantUploadCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, calls := newArtifactUploadServer(t, "test-run", "test-gate")
			controller := newTestController(t, newTestConfig(server.URL))

			workspace := t.TempDir()
			if len(tt.outDirFiles) > 0 {
				outDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
				for path, content := range tt.outDirFiles {
					populateTestFiles(t, outDir, []string{path}, content)
				}
			}

			meta := &contracts.BuildGateStageMetadata{
				Detected: &contracts.StackExpectation{
					Language: tt.language,
					Tool:     tt.tool,
				},
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: tt.language,
					Tool:     tt.tool,
					Passed:   true,
				}},
			}

			controller.uploadGateReportArtifacts(context.Background(), "test-run", "test-gate", workspace, meta)

			if len(*calls) != tt.wantUploadCount {
				t.Fatalf("upload call count = %d, want %d", len(*calls), tt.wantUploadCount)
			}

			if tt.wantUploadCount == 0 {
				if len(meta.ReportLinks) != 0 {
					t.Fatalf("expected no report links, got %+v", meta.ReportLinks)
				}
				return
			}

			// Verify artifact names.
			seen := make(map[string]bool, len(tt.wantNames))
			for _, c := range *calls {
				seen[c.Name] = true
			}
			for _, name := range tt.wantNames {
				if !seen[name] {
					t.Fatalf("expected artifact upload name %q", name)
				}
			}

			// Verify report links.
			if len(meta.ReportLinks) != len(tt.wantLinks) {
				t.Fatalf("report_links count = %d, want %d", len(meta.ReportLinks), len(tt.wantLinks))
			}
			linkByType := make(map[string]contracts.BuildGateReportLink, len(meta.ReportLinks))
			for _, link := range meta.ReportLinks {
				linkByType[link.Type] = link
			}
			for _, wl := range tt.wantLinks {
				link, ok := linkByType[wl.Type]
				if !ok {
					t.Fatalf("missing report link type %q in %+v", wl.Type, meta.ReportLinks)
				}
				if link.Path != wl.Path {
					t.Fatalf("%s link path = %q, want %q", wl.Type, link.Path, wl.Path)
				}
				if link.ArtifactID == "" || link.BundleCID == "" || link.URL == "" || link.DownloadURL == "" {
					t.Fatalf("expected populated link fields, got %+v", link)
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

	workspace := "/workspace"

	tests := []struct {
		name         string
		artifactPath string
		workspace    string
		wantValid    bool
	}{
		// Valid paths.
		{name: "simple relative path", artifactPath: "file.txt", workspace: workspace, wantValid: true},
		{name: "nested relative path", artifactPath: "dir/subdir/file.txt", workspace: workspace, wantValid: true},
		{name: "path with safe relative component", artifactPath: "dir/../file.txt", workspace: workspace, wantValid: true},
		{name: "path with dot", artifactPath: "./file.txt", workspace: workspace, wantValid: true},

		// Invalid paths.
		{name: "path traversal with ../../", artifactPath: "../../etc/passwd", workspace: workspace, wantValid: false},
		{name: "path traversal with multiple ..", artifactPath: "../../../var/log/secret.log", workspace: workspace, wantValid: false},
		{name: "path traversal at start", artifactPath: "../sibling/file.txt", workspace: workspace, wantValid: false},
		{name: "absolute path unix style", artifactPath: "/etc/passwd", workspace: workspace, wantValid: false},
		{name: "absolute path to sensitive file", artifactPath: "/root/.ssh/id_rsa", workspace: workspace, wantValid: false},
		{name: "empty path", artifactPath: "", workspace: workspace, wantValid: false},
		{name: "whitespace only path", artifactPath: "   ", workspace: workspace, wantValid: false},
		{name: "nested traversal escaping workspace", artifactPath: "subdir/../../secrets.txt", workspace: workspace, wantValid: false},
		{name: "deep nested traversal", artifactPath: "a/b/c/../../../../etc/hosts", workspace: workspace, wantValid: false},
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

func TestRunController_reportTerminalStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		runErr     error
		exitCode   int
		wantStatus string
	}{
		{
			name:       "runtime error reports cancelled",
			runErr:     context.Canceled,
			wantStatus: types.JobStatusCancelled.String(),
		},
		{
			name:       "nonzero exit code reports fail",
			exitCode:   1,
			wantStatus: types.JobStatusFail.String(),
		},
		{
			name:       "zero exit code reports success",
			exitCode:   0,
			wantStatus: types.JobStatusSuccess.String(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, cap := newStatusCaptureServer(t, "test-job")
			controller := newTestController(t, newTestConfig(server.URL))

			stats := types.NewRunStatsBuilder().ExitCode(tt.exitCode).MustBuild()
			result := step.Result{ExitCode: tt.exitCode}
			req := StartRunRequest{RunID: "test-run", JobID: "test-job"}

			controller.reportTerminalStatus(
				context.Background(), req, tt.runErr, result,
				stats, "", 100*1e6, // 100ms
			)

			if cap.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", cap.Status, tt.wantStatus)
			}
		})
	}
}
