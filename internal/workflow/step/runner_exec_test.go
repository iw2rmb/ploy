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

// TestRunner_Run_TimingCapture verifies that all phase timings (hydration,
// execution, build gate, diff, publish) are accurately measured and that
// total duration reflects the sum of all individual phase durations.
func TestRunner_Run_TimingCapture(t *testing.T) {
	hydrationDelay := 10 * time.Millisecond
	gateDelay := 5 * time.Millisecond

	runner := Runner{
		Workspace: &testWorkspaceHydrator{
			hydrateFn: func(ctx context.Context, manifest contracts.StepManifest, workspace string) error {
				time.Sleep(hydrationDelay)
				return nil
			},
		},
		Gate: &testGateExecutor{
			executeFn: func(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
				time.Sleep(gateDelay)
				return &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{
						{Tool: "test", Passed: true},
					},
				}, nil
			},
		},
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
		Gate: &contracts.StepGateSpec{
			Enabled: true,
		},
	}

	req := Request{
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
	}

	result, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	// Verify timing measurements are reasonable
	if time.Duration(result.Timings.HydrationDuration) < hydrationDelay {
		t.Errorf("Run() HydrationDuration = %v, expected >= %v", result.Timings.HydrationDuration, hydrationDelay)
	}

	if time.Duration(result.Timings.BuildGateDuration) < gateDelay {
		t.Errorf("Run() BuildGateDuration = %v, expected >= %v", result.Timings.BuildGateDuration, gateDelay)
	}

	// Total duration should be sum of all stages (with some tolerance)
	minExpected := result.Timings.HydrationDuration +
		result.Timings.ExecutionDuration +
		result.Timings.BuildGateDuration +
		result.Timings.DiffDuration +
		result.Timings.PublishDuration

	if result.Timings.TotalDuration < minExpected {
		t.Errorf("Run() TotalDuration = %v, expected >= %v", result.Timings.TotalDuration, minExpected)
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
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
		OutDir:    "/tmp/test-out",
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
		Manifest:  manifest,
		Workspace: "/tmp/test-workspace",
		OutDir:    "/tmp/test-out",
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
