package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/client"
)

const crashTerminalReconcileWindow = 120 * time.Second

type crashReconcileDockerClient interface {
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
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
	s := strings.TrimSpace(value)
	if s == "" {
		return time.Time{}, false
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || finishedAt.IsZero() {
		return time.Time{}, false
	}
	return finishedAt, true
}
