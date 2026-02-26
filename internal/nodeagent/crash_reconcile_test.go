package nodeagent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type fakeCrashReconcileDockerClient struct {
	listResult client.ContainerListResult
	listErr    error

	inspectByID    map[string]client.ContainerInspectResult
	inspectErrByID map[string]error
}

func (f *fakeCrashReconcileDockerClient) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	if f.listErr != nil {
		return client.ContainerListResult{}, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeCrashReconcileDockerClient) ContainerInspect(_ context.Context, containerID string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if err, ok := f.inspectErrByID[containerID]; ok && err != nil {
		return client.ContainerInspectResult{}, err
	}
	if inspect, ok := f.inspectByID[containerID]; ok {
		return inspect, nil
	}
	return client.ContainerInspectResult{}, errors.New("missing inspect result")
}

func TestCrashReconcile_StartupRunsBeforeFirstClaim_Contract(t *testing.T) {
	t.Skip("phase 0 contract: enable when startup reconciliation wiring (phase 5) is implemented")
}

func TestCrashReconcile_ClassifiesByRuntimeState_Contract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 14, 0, 0, 0, time.UTC)
	fakeDocker := &fakeCrashReconcileDockerClient{
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{
				ID:     "running-2",
				State:  containertypes.ContainerState("exited"), // Summary state must not decide classification.
				Labels: map[string]string{types.LabelRunID: "run-r2", types.LabelJobID: "job-r2"},
			},
			{
				ID:     "terminal-1",
				State:  containertypes.StateRunning, // Summary state must not decide classification.
				Labels: map[string]string{types.LabelRunID: "run-t1", types.LabelJobID: "job-t1"},
			},
			{
				ID:     "terminal-0",
				State:  containertypes.StateRunning,
				Labels: map[string]string{types.LabelRunID: "run-t0", types.LabelJobID: "job-t0"},
			},
			{
				ID:     "created-0",
				State:  containertypes.StateRunning,
				Labels: map[string]string{types.LabelRunID: "run-c0", types.LabelJobID: "job-c0"},
			},
			{
				ID:     "missing-job",
				State:  containertypes.StateRunning,
				Labels: map[string]string{types.LabelRunID: "run-missing"},
			},
			{
				ID:     "unmanaged",
				State:  containertypes.StateRunning,
				Labels: map[string]string{},
			},
		}},
		inspectByID: map[string]client.ContainerInspectResult{
			"running-2": inspectWithState(true, containertypes.StateRunning, ""),
			"terminal-1": inspectWithState(
				false,
				containertypes.ContainerState("exited"),
				now.Add(-30*time.Second).Format(time.RFC3339Nano),
			),
			"terminal-0": inspectWithState(
				false,
				containertypes.ContainerState("dead"),
				now.Add(-45*time.Second).Format(time.RFC3339Nano),
			),
			"created-0": inspectWithState(false, containertypes.ContainerState("created"), ""),
		},
	}

	reconciler := &startupCrashReconciler{
		docker:         fakeDocker,
		now:            func() time.Time { return now },
		terminalWindow: 120 * time.Second,
	}
	got, err := reconciler.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	wantRunning := []recoveredRunningContainer{
		{ContainerID: "running-2", RunID: types.RunID("run-r2"), JobID: types.JobID("job-r2")},
	}
	if !reflect.DeepEqual(got.Running, wantRunning) {
		t.Fatalf("Running = %#v, want %#v", got.Running, wantRunning)
	}

	wantTerminalIDs := []string{"terminal-0", "terminal-1"}
	gotTerminalIDs := make([]string, 0, len(got.RecentTerminal))
	for _, c := range got.RecentTerminal {
		gotTerminalIDs = append(gotTerminalIDs, c.ContainerID)
	}
	if !reflect.DeepEqual(gotTerminalIDs, wantTerminalIDs) {
		t.Fatalf("RecentTerminal IDs = %v, want %v", gotTerminalIDs, wantTerminalIDs)
	}
	if got.RecentTerminal[0].JobID != types.JobID("job-t0") || got.RecentTerminal[1].JobID != types.JobID("job-t1") {
		t.Fatalf("RecentTerminal job IDs = [%s %s], want [job-t0 job-t1]", got.RecentTerminal[0].JobID, got.RecentTerminal[1].JobID)
	}
}

func TestCrashReconcile_UsesFinishedAtCutoff_Contract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 15, 0, 0, 0, time.UTC)
	cutoff := now.Add(-120 * time.Second)
	fakeDocker := &fakeCrashReconcileDockerClient{
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{
				ID:      "boundary",
				Created: 1, // Created is intentionally old; cutoff must use FinishedAt.
				Labels:  map[string]string{types.LabelRunID: "run-b", types.LabelJobID: "job-b"},
			},
			{
				ID:      "fresh-created-stale-finished",
				Created: now.Unix(), // Created is intentionally recent; stale FinishedAt must be skipped.
				Labels:  map[string]string{types.LabelRunID: "run-s", types.LabelJobID: "job-s"},
			},
			{
				ID:      "inside-window",
				Created: 2,
				Labels:  map[string]string{types.LabelRunID: "run-i", types.LabelJobID: "job-i"},
			},
		}},
		inspectByID: map[string]client.ContainerInspectResult{
			"boundary": inspectWithState(false, containertypes.ContainerState("exited"), cutoff.Format(time.RFC3339Nano)),
			"fresh-created-stale-finished": inspectWithState(
				false,
				containertypes.ContainerState("exited"),
				cutoff.Add(-1*time.Nanosecond).Format(time.RFC3339Nano),
			),
			"inside-window": inspectWithState(
				false,
				containertypes.ContainerState("dead"),
				cutoff.Add(1*time.Nanosecond).Format(time.RFC3339Nano),
			),
		},
	}

	reconciler := &startupCrashReconciler{
		docker:         fakeDocker,
		now:            func() time.Time { return now },
		terminalWindow: 120 * time.Second,
	}
	got, err := reconciler.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	gotTerminalIDs := make([]string, 0, len(got.RecentTerminal))
	for _, c := range got.RecentTerminal {
		gotTerminalIDs = append(gotTerminalIDs, c.ContainerID)
	}
	wantTerminalIDs := []string{"boundary", "inside-window"}
	if !reflect.DeepEqual(gotTerminalIDs, wantTerminalIDs) {
		t.Fatalf("RecentTerminal IDs = %v, want %v", gotTerminalIDs, wantTerminalIDs)
	}
}

func TestCrashReconcile_SkipsStaleTerminalContainers_Contract(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 26, 16, 0, 0, 0, time.UTC)
	cutoff := now.Add(-120 * time.Second)
	fakeDocker := &fakeCrashReconcileDockerClient{
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{ID: "stale", Labels: map[string]string{types.LabelRunID: "run-stale", types.LabelJobID: "job-stale"}},
			{ID: "empty-finished", Labels: map[string]string{types.LabelRunID: "run-empty", types.LabelJobID: "job-empty"}},
			{ID: "bad-finished", Labels: map[string]string{types.LabelRunID: "run-bad", types.LabelJobID: "job-bad"}},
			{ID: "zero-finished", Labels: map[string]string{types.LabelRunID: "run-zero", types.LabelJobID: "job-zero"}},
			{ID: "no-state", Labels: map[string]string{types.LabelRunID: "run-ns", types.LabelJobID: "job-ns"}},
		}},
		inspectByID: map[string]client.ContainerInspectResult{
			"stale":          inspectWithState(false, containertypes.ContainerState("exited"), cutoff.Add(-1*time.Second).Format(time.RFC3339Nano)),
			"empty-finished": inspectWithState(false, containertypes.ContainerState("exited"), ""),
			"bad-finished":   inspectWithState(false, containertypes.ContainerState("dead"), "not-a-time"),
			"zero-finished":  inspectWithState(false, containertypes.ContainerState("dead"), time.Time{}.Format(time.RFC3339Nano)),
			"no-state":       {Container: containertypes.InspectResponse{}},
		},
	}

	reconciler := &startupCrashReconciler{
		docker:         fakeDocker,
		now:            func() time.Time { return now },
		terminalWindow: 120 * time.Second,
	}
	got, err := reconciler.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(got.RecentTerminal) != 0 {
		t.Fatalf("RecentTerminal len = %d, want 0", len(got.RecentTerminal))
	}
}

func inspectWithState(running bool, status containertypes.ContainerState, finishedAt string) client.ContainerInspectResult {
	return client.ContainerInspectResult{
		Container: containertypes.InspectResponse{
			State: &containertypes.State{
				Running:    running,
				Status:     status,
				FinishedAt: finishedAt,
			},
		},
	}
}
