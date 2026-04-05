package step

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type testStreamingContainerRuntime struct {
	testContainerRuntime
	streamLogsFn func(ctx context.Context, handle ContainerHandle, stdout, stderr io.Writer) error
}

func (m *testStreamingContainerRuntime) StreamLogs(ctx context.Context, handle ContainerHandle, stdout, stderr io.Writer) error {
	if m.streamLogsFn != nil {
		return m.streamLogsFn(ctx, handle, stdout, stderr)
	}
	return nil
}

func TestDockerGateExecutor_StreamsLogsToExecutionWriter(t *testing.T) {
	t.Parallel()

	var live strings.Builder
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
			_, _ = stdout.Write([]byte("live gate line\n"))
			return nil
		},
	}

	executor := NewDockerGateExecutor(rt)
	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{Enabled: true}

	meta, err := executor.Execute(WithExecutionLogWriter(context.Background(), &live), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if strings.Contains(live.String(), "live gate line") == false {
		t.Fatalf("expected live writer to receive streamed logs, got %q", live.String())
	}
	if strings.Contains(meta.LogsText, "live gate line") == false {
		t.Fatalf("expected metadata logs to include streamed logs, got %q", meta.LogsText)
	}
	if rt.logsCalled {
		t.Fatalf("expected one-shot Logs() fallback not to be used when StreamLogs succeeds")
	}
}
