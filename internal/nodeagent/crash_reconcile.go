package nodeagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const crashTerminalReconcileWindow = 120 * time.Second

type crashReconcileDockerClient interface {
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
}

type recoveredRunningContainer struct {
	ContainerID string
	RunID       types.RunID
	JobID       types.JobID
}

type recoveredTerminalContainer struct {
	ContainerID string
	RunID       types.RunID
	JobID       types.JobID
	FinishedAt  time.Time
}

type startupCrashSnapshot struct {
	Running        []recoveredRunningContainer
	RecentTerminal []recoveredTerminalContainer
}

type startupCrashReconciler struct {
	docker         crashReconcileDockerClient
	now            func() time.Time
	terminalWindow time.Duration
}

func newStartupCrashReconciler() (*startupCrashReconciler, error) {
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &startupCrashReconciler{
		docker:         dockerClient,
		now:            time.Now,
		terminalWindow: crashTerminalReconcileWindow,
	}, nil
}

func (r *startupCrashReconciler) Discover(ctx context.Context) (startupCrashSnapshot, error) {
	if r == nil || r.docker == nil {
		return startupCrashSnapshot{}, errors.New("startup crash reconciler not configured")
	}

	nowFn := r.now
	if nowFn == nil {
		nowFn = time.Now
	}
	window := r.terminalWindow
	if window <= 0 {
		window = crashTerminalReconcileWindow
	}
	cutoff := nowFn().Add(-window)

	listed, err := r.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return startupCrashSnapshot{}, fmt.Errorf("list containers: %w", err)
	}

	snapshot := startupCrashSnapshot{
		Running:        make([]recoveredRunningContainer, 0),
		RecentTerminal: make([]recoveredTerminalContainer, 0),
	}

	for _, summary := range listed.Items {
		runID, jobID, ok := ployContainerIdentity(summary.Labels)
		if !ok {
			continue
		}

		inspect, err := r.docker.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if err != nil {
			return startupCrashSnapshot{}, fmt.Errorf("inspect container %s: %w", summary.ID, err)
		}
		if inspect.Container.State == nil {
			continue
		}

		state := inspect.Container.State
		if state.Running {
			snapshot.Running = append(snapshot.Running, recoveredRunningContainer{
				ContainerID: summary.ID,
				RunID:       runID,
				JobID:       jobID,
			})
			continue
		}

		if !isTerminalContainerStatus(string(state.Status)) {
			continue
		}
		finishedAt, ok := parseDockerFinishedAt(state.FinishedAt)
		if !ok {
			continue
		}
		if finishedAt.Before(cutoff) {
			continue
		}

		snapshot.RecentTerminal = append(snapshot.RecentTerminal, recoveredTerminalContainer{
			ContainerID: summary.ID,
			RunID:       runID,
			JobID:       jobID,
			FinishedAt:  finishedAt,
		})
	}

	sort.SliceStable(snapshot.Running, func(i, j int) bool {
		return snapshot.Running[i].ContainerID < snapshot.Running[j].ContainerID
	})
	sort.SliceStable(snapshot.RecentTerminal, func(i, j int) bool {
		return snapshot.RecentTerminal[i].ContainerID < snapshot.RecentTerminal[j].ContainerID
	})

	return snapshot, nil
}

type recoveredContainerTerminal struct {
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}

func (r *startupCrashReconciler) WaitRecoveredContainer(ctx context.Context, containerID string) (recoveredContainerTerminal, error) {
	if r == nil || r.docker == nil {
		return recoveredContainerTerminal{}, errors.New("startup crash reconciler not configured")
	}
	waitResult := r.docker.ContainerWait(ctx, containerID, client.ContainerWaitOptions{
		Condition: containertypes.WaitConditionNotRunning,
	})
	select {
	case waitErr := <-waitResult.Error:
		if waitErr != nil {
			return recoveredContainerTerminal{}, fmt.Errorf("wait container %s: %w", containerID, waitErr)
		}
	case <-waitResult.Result:
	}

	inspect, err := r.docker.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return recoveredContainerTerminal{}, fmt.Errorf("inspect container %s after wait: %w", containerID, err)
	}
	if inspect.Container.State == nil {
		return recoveredContainerTerminal{}, fmt.Errorf("container %s has no state after wait", containerID)
	}
	state := inspect.Container.State
	startedAt, _ := parseDockerTimestamp(state.StartedAt)
	finishedAt, _ := parseDockerTimestamp(state.FinishedAt)
	return recoveredContainerTerminal{
		ExitCode:   state.ExitCode,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
	}, nil
}

func (r *startupCrashReconciler) ReadContainerLogs(ctx context.Context, containerID string) ([]byte, error) {
	if r == nil || r.docker == nil {
		return nil, errors.New("startup crash reconciler not configured")
	}
	reader, err := r.docker.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("read container logs %s: %w", containerID, err)
	}
	defer func() { _ = reader.Close() }()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, reader); err != nil {
		raw, rawErr := io.ReadAll(reader)
		if rawErr != nil {
			return nil, fmt.Errorf("demux container logs %s: %w", containerID, err)
		}
		return raw, nil
	}
	return append(stdoutBuf.Bytes(), stderrBuf.Bytes()...), nil
}

func ployContainerIdentity(labels map[string]string) (types.RunID, types.JobID, bool) {
	if len(labels) == 0 {
		return "", "", false
	}
	runID := types.RunID(strings.TrimSpace(labels[types.LabelRunID]))
	jobID := types.JobID(strings.TrimSpace(labels[types.LabelJobID]))
	if runID.IsZero() || jobID.IsZero() {
		return "", "", false
	}
	return runID, jobID, true
}

func isTerminalContainerStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "exited", "dead":
		return true
	default:
		return false
	}
}

func parseDockerFinishedAt(value string) (time.Time, bool) {
	return parseDockerTimestamp(value)
}

func parseDockerTimestamp(value string) (time.Time, bool) {
	s := strings.TrimSpace(value)
	if s == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || parsed.IsZero() {
		return time.Time{}, false
	}
	return parsed, true
}
