package nodeagent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

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

func TestRunController_computeRepoSHAOut(t *testing.T) {
	t.Parallel()

	controller := &runController{}
	ctx := context.Background()

	t.Run("missing repo_sha_in returns error", func(t *testing.T) {
		t.Parallel()

		repoDir := t.TempDir()
		initRepoWithFile(t, repoDir, "main.txt", "base\n")

		_, err := controller.computeRepoSHAOut(ctx, StartRunRequest{
			RunID: "run-1",
			JobID: "job-1",
		}, repoDir, "")
		if err == nil {
			t.Fatal("computeRepoSHAOut() error = nil, want non-nil")
		}
	})

	t.Run("invalid repo_sha_in returns error", func(t *testing.T) {
		t.Parallel()

		repoDir := t.TempDir()
		initRepoWithFile(t, repoDir, "main.txt", "base\n")

		_, err := controller.computeRepoSHAOut(ctx, StartRunRequest{
			RunID:     "run-2",
			JobID:     "job-2",
			RepoSHAIn: types.CommitSHA("not-a-sha"),
		}, repoDir, "")
		if err == nil {
			t.Fatal("computeRepoSHAOut() error = nil, want non-nil")
		}
	})

	t.Run("valid repo_sha_in returns hash", func(t *testing.T) {
		t.Parallel()

		repoDir := t.TempDir()
		initRepoWithFile(t, repoDir, "main.txt", "base\n")
		headSHA := gitrepo.RevParse(t, repoDir, "HEAD")

		repoSHAOut, err := controller.computeRepoSHAOut(ctx, StartRunRequest{
			RunID:     "run-3",
			JobID:     "job-3",
			RepoSHAIn: types.CommitSHA(headSHA),
		}, repoDir, "")
		if err != nil {
			t.Fatalf("computeRepoSHAOut() error = %v", err)
		}
		if !strings.EqualFold(repoSHAOut, headSHA) {
			t.Fatalf("repo_sha_out = %q, want %q", repoSHAOut, headSHA)
		}
	})
}

type staticDiffGenerator struct {
	diff []byte
	err  error
}

func (g staticDiffGenerator) Generate(context.Context, string) ([]byte, error) {
	return g.diff, g.err
}

func TestRunController_uploadJobDiffWritesInspectabilityCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		diff       []byte
		stalePatch bool
		wantUpload bool
		wantPatch  string
	}{
		{
			name:       "non-empty diff writes patch and uploads",
			diff:       []byte("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n"),
			wantUpload: true,
			wantPatch:  "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n",
		},
		{
			name:       "empty diff removes stale patch and skips upload",
			stalePatch: true,
			wantUpload: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, calls := newDiffUploadServer(t, "test-run", "test-job")
			controller := newTestController(t, newAgentConfig(server.URL))
			patchPath := filepath.Join(t.TempDir(), "diff.patch")
			if tt.stalePatch {
				if err := os.WriteFile(patchPath, []byte("stale"), 0o644); err != nil {
					t.Fatalf("write stale patch: %v", err)
				}
			}

			uploaded, err := controller.uploadJobDiff(
				context.Background(),
				"test-run",
				"test-job",
				staticDiffGenerator{diff: tt.diff},
				t.TempDir(),
				step.Result{ExitCode: 0},
				types.DiffJobTypeMig,
				patchPath,
			)
			if err != nil {
				t.Fatalf("uploadJobDiff() error = %v", err)
			}
			if uploaded != tt.wantUpload {
				t.Fatalf("uploaded = %v, want %v", uploaded, tt.wantUpload)
			}
			if got := len(*calls) > 0; got != tt.wantUpload {
				t.Fatalf("diff upload occurred = %v, want %v", got, tt.wantUpload)
			}
			data, readErr := os.ReadFile(patchPath)
			if tt.wantPatch == "" {
				if !os.IsNotExist(readErr) {
					t.Fatalf("diff.patch should not exist, readErr=%v data=%q", readErr, data)
				}
				return
			}
			if readErr != nil {
				t.Fatalf("read diff.patch: %v", readErr)
			}
			if string(data) != tt.wantPatch {
				t.Fatalf("diff.patch = %q, want %q", data, tt.wantPatch)
			}
		})
	}
}

func TestRunController_reportTerminalStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		runErr     error
		jobType    types.JobType
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
			jobType:    types.JobTypeMig,
			exitCode:   1,
			wantStatus: types.JobStatusFail.String(),
		},
		{
			name:       "pre_gate exit code one reports fail",
			jobType:    types.JobTypePreGate,
			exitCode:   1,
			wantStatus: types.JobStatusFail.String(),
		},
		{
			name:       "mig exit code one reports fail",
			jobType:    types.JobTypeMig,
			exitCode:   1,
			wantStatus: types.JobStatusFail.String(),
		},
		{
			name:       "exit code above one reports error",
			jobType:    types.JobTypeMig,
			exitCode:   2,
			wantStatus: types.JobStatusError.String(),
		},
		{
			name:       "zero exit code reports success",
			jobType:    types.JobTypeMig,
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
			req := StartRunRequest{RunID: "test-run", JobID: "test-job", JobType: tt.jobType}

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

func TestRunController_reportTerminalStatus_PreservesSharedArtifactsOnTerminalSuccess(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	shareDir := runSharedArtifactsDir(runID)
	if err := os.MkdirAll(shareDir, 0o755); err != nil {
		t.Fatalf("mkdir share dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shareDir, "java.classpath"), []byte("/repo/.m2/a.jar\n"), 0o644); err != nil {
		t.Fatalf("write share classpath: %v", err)
	}

	server, _ := newStatusCaptureServer(t, "test-job")
	controller := newTestController(t, newAgentConfig(server.URL))
	req := StartRunRequest{
		RunID:   runID,
		RepoID:  repoID,
		JobID:   "test-job",
		JobType: types.JobTypeMig,
		NextID:  nil,
	}
	stats := types.NewRunStatsBuilder().ExitCode(0).MustBuild()

	controller.reportTerminalStatus(
		context.Background(),
		req,
		nil,
		step.Result{ExitCode: 0},
		stats,
		"",
		100*1e6,
	)

	if _, err := os.Stat(filepath.Join(shareDir, "java.classpath")); err != nil {
		t.Fatalf("shared artifact should remain on terminal success, stat err=%v", err)
	}
}
