package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// testRunID and testJobID are fixed IDs used across upload tests to match mock servers.
const (
	testUploadRunID = "test-run-123"
	testUploadJobID = "test-job-id"
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

			env := newUploadTestEnv(t, testUploadRunID, testUploadJobID)

			workspace := t.TempDir()
			populateTestFiles(t, workspace, tt.createFiles, "test")

			outDir := ""
			if len(tt.outDirFiles) > 0 {
				outDir = t.TempDir()
				populateTestFiles(t, outDir, tt.outDirFiles, "test")
			}

			opts := RunOptions{Artifacts: ArtifactOptions{Paths: tt.artifactPaths, Name: "test-artifact"}}
			req := StartRunRequest{RunID: testUploadRunID, JobID: testUploadJobID}

			env.Controller.uploadConfiguredArtifacts(context.Background(), req, opts, contracts.StepManifest{}, workspace, outDir)

			assertUpload(t, env.Calls, tt.wantUpload, "test-artifact", tt.wantHeaders)
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
			bundleName:  "mig-out",
			wantHeaders: []string{"out/result.txt", "out/subdir/output.log"},
		},
		{
			name:       "empty directory skips upload",
			bundleName: "mig-out",
			wantUpload: false,
		},
		{
			name:        "empty outDir string skips upload",
			emptyOutDir: true,
			bundleName:  "mig-out",
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

			env := newUploadTestEnv(t, "test-run", "test-stage")

			outDir := ""
			if !tt.emptyOutDir {
				outDir = t.TempDir()
				populateTestFiles(t, outDir, tt.createFiles, "test output")
			}

			err := env.Controller.uploadOutDirBundle(context.Background(), "test-run", "test-stage", outDir, tt.bundleName)

			checkErr(t, tt.wantErr, err)
			assertUpload(t, env.Calls, tt.wantUpload, tt.bundleName, tt.wantHeaders)
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
			controller := newTestController(t, newAgentConfig(server.URL))

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

			env := newUploadTestEnv(t, "test-run", "test-stage", withArtifactStatus(tt.serverStatus))

			phase := &types.RunStatsGatePhase{}
			env.Controller.uploadGateLogsArtifact("test-run", "test-stage", tt.logsText, tt.artifactSuffix, phase)

			assertGatePhaseIDs(t, phase, tt.wantArtifactID)
			assertUpload(t, env.Calls, true, tt.wantArtifactName, nil)
		})
	}
}

func TestRunController_uploadGateReportArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tool            string
		language        string
		outDirFiles     map[string]string // path -> content
		wantUploadCount int
		wantNames       []string
		wantLinks       []wantReportLink
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
			wantLinks: []wantReportLink{
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

			env := newUploadTestEnv(t, "test-run", "test-gate")

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

			env.Controller.uploadGateReportArtifacts(context.Background(), "test-run", "test-gate", workspace, meta)

			if len(*env.Calls) != tt.wantUploadCount {
				t.Fatalf("upload call count = %d, want %d", len(*env.Calls), tt.wantUploadCount)
			}
			if tt.wantUploadCount == 0 {
				if len(meta.ReportLinks) != 0 {
					t.Fatalf("expected no report links, got %+v", meta.ReportLinks)
				}
				return
			}

			assertArtifactNames(t, env.Calls, tt.wantNames)
			assertReportLinks(t, meta.ReportLinks, tt.wantLinks)
		})
	}
}

// Note: uploadDiff and associated mig diff metadata tests were removed along
// with legacy HEAD-based diff generation. Migs now use baseline-aware
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
			name:       "runtime error reports error",
			runErr:     errors.New("runtime failed"),
			wantStatus: types.JobStatusError.String(),
		},
		{
			name:       "exit code one reports fail",
			exitCode:   1,
			wantStatus: types.JobStatusFail.String(),
		},
		{
			name:       "exit code above one reports error",
			exitCode:   2,
			wantStatus: types.JobStatusError.String(),
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
			controller := newTestController(t, newAgentConfig(server.URL))

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

func TestEncodeHookConditionResult_EmitsStructuredJSON(t *testing.T) {
	t.Parallel()

	raw := encodeHookConditionResult(&contracts.HookRuntimeDecision{
		HookHash:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HookShouldRun: true,
	})

	var decoded struct {
		Evaluated bool   `json:"evaluated"`
		ShouldRun bool   `json:"should_run"`
		Hash      string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal condition result: %v", err)
	}
	if !decoded.Evaluated {
		t.Fatalf("evaluated=%v, want true", decoded.Evaluated)
	}
	if !decoded.ShouldRun {
		t.Fatalf("should_run=%v, want true", decoded.ShouldRun)
	}
	if decoded.Hash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("hash=%q, want hook hash", decoded.Hash)
	}
}

func TestEncodeHookCommandIdentity_EmitsStructuredJSON(t *testing.T) {
	t.Parallel()

	raw := encodeHookCommandIdentityList("aa11bb22", []hook.Step{{
		Name:    "security-scan",
		Image:   "hook:latest",
		Command: []string{"scan", "--sbom", "/in/sbom.spdx.json", "--out", "/out/sbom.spdx.json"},
	}})

	var decoded struct {
		Source string `json:"source"`
		Steps  []struct {
			Name    string   `json:"name"`
			Image   string   `json:"image"`
			Command []string `json:"command"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("unmarshal command identity: %v", err)
	}
	if decoded.Source != "aa11bb22" {
		t.Fatalf("source=%q, want aa11bb22", decoded.Source)
	}
	if len(decoded.Steps) != 1 {
		t.Fatalf("steps len=%d, want 1", len(decoded.Steps))
	}
	if decoded.Steps[0].Name != "security-scan" {
		t.Fatalf("steps[0].name=%q, want security-scan", decoded.Steps[0].Name)
	}
	if decoded.Steps[0].Image != "hook:latest" {
		t.Fatalf("steps[0].image=%q, want hook:latest", decoded.Steps[0].Image)
	}
	if len(decoded.Steps[0].Command) != 5 {
		t.Fatalf("steps[0].command len=%d, want 5", len(decoded.Steps[0].Command))
	}
}
