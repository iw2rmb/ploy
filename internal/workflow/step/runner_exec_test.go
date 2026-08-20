package step

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRunner_Run_CapturesContainerExecutionDuration(t *testing.T) {
	executionDelay := 10 * time.Millisecond
	runner := Runner{Containers: &testContainerRuntime{
		waitFn: func(context.Context, ContainerHandle) (ContainerResult, error) {
			time.Sleep(executionDelay)
			return ContainerResult{ExitCode: 0}, nil
		},
	}}
	req := Request{
		RunID:     types.RunID("run-123"),
		JobID:     types.JobID("job-123"),
		Manifest:  contracts.StepManifest{ID: "test-step", Image: "test:latest"},
		Workspace: t.TempDir(),
		JobMounts: newTestMigJobMounts(t),
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if time.Duration(result.Timings.ExecutionDuration) < executionDelay {
		t.Fatalf("ExecutionDuration = %v, want at least %v", result.Timings.ExecutionDuration, executionDelay)
	}
}

func TestRunner_Run_DoesNotRemoveContainerAfterCompletion(t *testing.T) {
	rt := &testContainerRuntime{
		logsFn: func(ctx context.Context, handle ContainerHandle) ([]byte, error) {
			return []byte("test logs"), nil
		},
	}
	var logBuf bytes.Buffer
	runner := Runner{
		Containers: rt,
		LogWriter:  &logBuf,
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{
			{
				Name:        "source",
				MountPath:   "/workspace",
				Mode:        contracts.StepInputModeReadOnly,
				SnapshotCID: types.CID("bafytest123"),
			},
		},
	}
	req := Request{
		RunID:     types.RunID("run-123"),
		JobID:     types.JobID("job-123"),
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
		JobMounts: newTestMigJobMounts(t),
	}

	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if !rt.createCalled || !rt.startCalled || !rt.waitCalled || !rt.logsCalled {
		t.Fatalf("expected create/start/wait/logs to be called; got %+v", rt)
	}
	if rt.removeCalled {
		t.Fatalf("expected Remove not to be called")
	}
	if got := logBuf.String(); got == "" {
		t.Fatalf("expected log output to be written")
	}
}

func TestRunner_Run_StreamsLogsLiveWhenSupported(t *testing.T) {
	rt := &testStreamingContainerRuntime{
		testContainerRuntime: testContainerRuntime{
			waitFn: func(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
				return ContainerResult{ExitCode: 0}, nil
			},
			logsFn: func(ctx context.Context, handle ContainerHandle) ([]byte, error) {
				return []byte("fallback logs should not be used"), nil
			},
		},
		streamLogsFn: func(ctx context.Context, handle ContainerHandle, stdout, stderr io.Writer) error {
			_, _ = stdout.Write([]byte("live runner line\n"))
			return nil
		},
	}

	var logBuf bytes.Buffer
	runner := Runner{
		Containers: rt,
		LogWriter:  &logBuf,
	}

	manifest := contracts.StepManifest{
		ID:    types.StepID("test-step"),
		Name:  "Test Step",
		Image: "test:latest",
		Inputs: []contracts.StepInput{{
			Name:        "source",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadOnly,
			SnapshotCID: types.CID("bafytest123"),
		}},
	}
	req := Request{
		RunID:     types.RunID("run-123"),
		JobID:     types.JobID("job-123"),
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
		JobMounts: newTestMigJobMounts(t),
	}

	if _, err := runner.Run(context.Background(), req); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !strings.Contains(logBuf.String(), "live runner line") {
		t.Fatalf("expected live streamed logs in output, got %q", logBuf.String())
	}
	if rt.logsCalled {
		t.Fatalf("expected one-shot Logs() fallback not to be used when StreamLogs succeeds")
	}
}
