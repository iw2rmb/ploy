package nodeagent

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestRunMigContainerJobNonzeroExitReportsDerivedStatsError(t *testing.T) {
	hydrationDuration := 25 * time.Millisecond
	testCases := []struct {
		name      string
		stdout    string
		stderr    string
		wantError string
	}{
		{
			name: "amata json provider failure",
			stdout: `{"kind":"step_finished","step":{"status":"failed","error":{"message":"step failed","details":{"provider_error":{"message":"step provider failed"}}}}}` + "\n" +
				`{"kind":"run_finished","failure":{"message":"run failed","details":{"provider_error":{"message":"exceeded retry limit, last status: 429 Too Many Requests, request id: req_123"}}}}` + "\n",
			stderr:    "stderr fallback\n",
			wantError: "exceeded retry limit, last status: 429 Too Many Requests, request id: req_123",
		},
		{
			name:      "stderr fallback",
			stderr:    "\nfirst stderr line\n\nfinal stderr line\n",
			wantError: "final stderr line",
		},
		{
			name:      "exit code fallback",
			wantError: "job mig failed with exit code 1",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cacheHome := t.TempDir()
			t.Setenv("PLOYD_CACHE_HOME", cacheHome)

			runID := types.NewRunID()
			jobID := types.NewJobID()
			paths := jobDirectories(runID, jobID)
			if err := ensureJobDirectories(paths); err != nil {
				t.Fatalf("ensure job directories: %v", err)
			}
			artifactLogs, err := newArtifactLogWriter(nil, paths)
			if err != nil {
				t.Fatalf("new artifact log writer: %v", err)
			}
			defer func() { _ = artifactLogs.Close() }()

			server, cap := newStatusCaptureServer(t, jobID.String())
			controller := newTestController(t, newAgentConfig(server.URL))
			req := StartRunRequest{
				RunID:   runID,
				RepoID:  types.NewRepoID(),
				JobID:   jobID,
				JobType: types.JobTypeMig,
			}
			execCtx := executionContext{
				runner: step.Runner{
					Containers: &mockContainerRuntime{
						waitFn: func(context.Context, step.ContainerHandle) (step.ContainerResult, error) {
							return step.ContainerResult{ExitCode: 1, ContainerID: "container-1"}, nil
						},
						logsFn: func(context.Context, step.ContainerHandle) ([]byte, error) {
							return []byte(tc.stdout + tc.stderr), nil
						},
						streamLogsFn: func(_ context.Context, _ step.ContainerHandle, stdout io.Writer, stderr io.Writer) error {
							if _, err := io.WriteString(stdout, tc.stdout); err != nil {
								return err
							}
							if _, err := io.WriteString(stderr, tc.stderr); err != nil {
								return err
							}
							return nil
						},
					},
					LogWriter: artifactLogs,
				},
			}
			manifest := contracts.StepManifest{
				ID:      "mig-step",
				Image:   "example/mig:latest",
				Command: []string{"amata", "run"},
			}
			workspace := t.TempDir()
			initRepoWithFile(t, workspace, "README.md", "base\n")
			mounts, err := jobMounts(paths, runID, req.JobType)
			if err != nil {
				t.Fatalf("jobMounts() error = %v", err)
			}

			_, err = controller.runMigContainerJob(
				context.Background(),
				req,
				manifest,
				execCtx,
				workspace,
				time.Now().Add(-50*time.Millisecond),
				hydrationDuration,
				paths,
				mounts,
			)
			if err != nil {
				t.Fatalf("runMigContainerJob() error = %v", err)
			}
			if closeErr := artifactLogs.Close(); closeErr != nil {
				t.Fatalf("close artifact logs: %v", closeErr)
			}

			if cap.Status != types.JobStatusFail.String() {
				t.Fatalf("status = %q, want %q", cap.Status, types.JobStatusFail.String())
			}
			if got := cap.Stats["error"]; got != tc.wantError {
				t.Fatalf("stats.error = %#v, want %q", got, tc.wantError)
			}
			timings, ok := cap.Stats["timings"].(map[string]any)
			if !ok {
				t.Fatalf("stats.timings = %#v, want object", cap.Stats["timings"])
			}
			if got := timings["hydration_duration_ms"]; got != float64(hydrationDuration.Milliseconds()) {
				t.Fatalf("hydration_duration_ms = %#v, want %d", got, hydrationDuration.Milliseconds())
			}
			if got, ok := timings["total_duration_ms"].(float64); !ok || got < float64(hydrationDuration.Milliseconds()) {
				t.Fatalf("total_duration_ms = %#v, want at least %d", timings["total_duration_ms"], hydrationDuration.Milliseconds())
			}
		})
	}
}
