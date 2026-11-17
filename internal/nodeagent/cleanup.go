package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const cleanupHostMountInfo = "/run/ploy/host-mountinfo"

type cleanupDockerClient interface {
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ImageList(context.Context, client.ImageListOptions) (client.ImageListResult, error)
	ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error)
	ImageRemove(context.Context, string, client.ImageRemoveOptions) (client.ImageRemoveResult, error)
	BuildCachePrune(context.Context, client.BuildCachePruneOptions) (client.BuildCachePruneResult, error)
	VolumePrune(context.Context, client.VolumePruneOptions) (client.VolumePruneResult, error)
}

type cleanupBuildCacheRecord struct {
	ID    string
	InUse *bool
}

// cleanupNode requires the exclusive maintenance lock for its complete pass.
func (r *runController) cleanupNode(ctx context.Context) error {
	ageText := os.Getenv("PLOY_NODE_CLEANUP_AGE")
	if ageText == "" {
		ageText = "24h"
	}
	age, err := parseCleanupAge(ageText)
	if err != nil {
		return err
	}
	docker, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("create cleanup Docker client: %w", err)
	}
	defer docker.Close()
	tasks, err := cleanupContainerdIDs(ctx, "tasks")
	if err != nil {
		return err
	}
	leases, err := cleanupContainerdIDs(ctx, "leases")
	if err != nil {
		return err
	}
	containers, err := inspectCleanupContainers(ctx, docker)
	if err != nil {
		return err
	}
	if err := checkCleanupActivity(ctx, containers, tasks, leases, func(ctx context.Context) ([]cleanupBuildCacheRecord, error) {
		return cleanupBuildCache(ctx, docker)
	}); err != nil {
		return err
	}
	if err := pruneCleanupDocker(ctx, docker, containers, time.Now(), age, ageText, os.Getenv("PLOY_NODE_PRUNE_DOCKER_VOLUMES") == "true"); err != nil {
		return err
	}
	if err := r.pruneCleanupImages(ctx, docker, containers, time.Now(), age); err != nil {
		return err
	}
	if err := pruneCleanupGitClones(ctx, age, time.Now()); err != nil {
		return err
	}
	return r.pruneExpiredRunData(ctx, docker, cleanupHostMountInfo)
}

func parseCleanupAge(value string) (time.Duration, error) {
	if len(value) < 2 {
		return 0, fmt.Errorf("cleanup age must use h or m suffix: %q", value)
	}
	unit := time.Minute
	switch value[len(value)-1] {
	case 'h':
		unit = time.Hour
	case 'm':
	default:
		return 0, fmt.Errorf("cleanup age must use h or m suffix: %q", value)
	}
	for _, digit := range value[:len(value)-1] {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("invalid cleanup age: %q", value)
		}
	}
	count, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil || count <= 0 || count > int64((1<<63-1)/unit) {
		return 0, fmt.Errorf("invalid cleanup age: %q", value)
	}
	return time.Duration(count) * unit, nil
}

func cleanupContainerdIDs(ctx context.Context, kind string) (map[string]bool, error) {
	out, err := exec.CommandContext(ctx, "/usr/bin/ctr", "--address", "/run/containerd/containerd.sock", "--namespace", "moby", kind, "list", "-q").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect containerd %s: %w", kind, err)
	}
	ids := make(map[string]bool)
	for _, id := range strings.Fields(string(out)) {
		ids[id] = true
	}
	return ids, nil
}

func inspectCleanupContainers(ctx context.Context, docker cleanupDockerClient) ([]container.InspectResponse, error) {
	listed, err := docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list cleanup containers: %w", err)
	}
	containers := make([]container.InspectResponse, 0, len(listed.Items))
	for _, item := range listed.Items {
		inspected, err := docker.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if err != nil {
			return nil, fmt.Errorf("inspect cleanup container %s: %w", item.ID, err)
		}
		c := inspected.Container
		if c.ID == "" || c.ID != item.ID || c.State == nil || c.Config == nil {
			return nil, fmt.Errorf("incomplete cleanup container inspection: %s", item.ID)
		}
		var fields struct {
			Mounts []json.RawMessage
			State  *struct {
				Running, Restarting, Paused *bool
				Pid                         *int
			}
		}
		if err := json.Unmarshal(inspected.Raw, &fields); err != nil || fields.Mounts == nil || fields.State == nil || fields.State.Running == nil || fields.State.Restarting == nil || fields.State.Paused == nil || fields.State.Pid == nil {
			return nil, fmt.Errorf("incomplete cleanup container mounts or state: %s", item.ID)
		}
		containers = append(containers, c)
	}
	return containers, nil
}

func checkCleanupActivity(ctx context.Context, containers []container.InspectResponse, tasks, leases map[string]bool, buildCache func(context.Context) ([]cleanupBuildCacheRecord, error)) error {
	containerIDs := make(map[string]bool, len(containers))
	for _, c := range containers {
		containerIDs[c.ID] = true
		_, job := c.Config.Labels[types.LabelJobID]
		if !job && c.Config.Labels[types.LabelJobResource] != "true" {
			continue
		}
		s := c.State
		if (s.Status != container.StateExited && s.Status != container.StateDead) || s.Running || s.Restarting || s.Paused || s.Pid != 0 || tasks[c.ID] {
			return fmt.Errorf("unreconciled job container: %s", c.ID)
		}
	}
	unknown := make(map[string]bool)
	for id := range leases {
		if !containerIDs[id] {
			unknown[id] = true
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	items, err := buildCache(ctx)
	if err != nil {
		return err
	}
	if items == nil {
		return fmt.Errorf("build cache inspection returned no items")
	}
	inactive, active := make(map[string]bool), make(map[string]bool)
	for _, item := range items {
		if item.ID == "" || item.InUse == nil {
			return fmt.Errorf("malformed build cache item")
		}
		target := inactive
		if *item.InUse {
			target = active
		}
		for _, id := range []string{item.ID, item.ID + "-view", item.ID + "-variants"} {
			target[id] = true
		}
	}
	for id := range unknown {
		if !inactive[id] || active[id] {
			return fmt.Errorf("unaccounted containerd lease: %s", id)
		}
	}
	return nil
}

func cleanupBuildCache(ctx context.Context, docker *client.Client) ([]cleanupBuildCacheRecord, error) {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return docker.Dialer()(ctx)
	}}
	defer transport.CloseIdleConnections()
	// The SDK drops field presence. Missing InUse cannot prove a lease is inactive.
	type usage struct {
		BuildCacheUsage *struct{ Items []cleanupBuildCacheRecord }
	}
	response, err := httpx.DoJSON[usage](ctx, &http.Client{Transport: transport, Timeout: 30 * time.Second}, http.MethodGet,
		"http://docker/v1.52/system/df?type=build-cache&verbose=1", nil, http.StatusOK, "inspect cleanup build cache")
	if err != nil {
		return nil, err
	}
	if response.BuildCacheUsage == nil {
		return nil, fmt.Errorf("build cache usage is unavailable")
	}
	return response.BuildCacheUsage.Items, nil
}

func pruneCleanupDocker(ctx context.Context, docker cleanupDockerClient, containers []container.InspectResponse, now time.Time, age time.Duration, ageText string, volumes bool) error {
	for _, c := range containers {
		created, err := time.Parse(time.RFC3339Nano, c.Created)
		if err != nil {
			return fmt.Errorf("parse container %s creation time: %w", c.ID, err)
		}
		if c.State.Status != container.StateCreated && c.State.Status != container.StateExited && c.State.Status != container.StateDead {
			continue
		}
		if !created.Before(now.Add(-age)) {
			continue
		}
		if _, err := docker.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{RemoveVolumes: true}); err != nil {
			return fmt.Errorf("remove expired container %s: %w", c.ID, err)
		}
	}
	if _, err := docker.BuildCachePrune(ctx, client.BuildCachePruneOptions{All: true, Filters: make(client.Filters).Add("until", ageText)}); err != nil {
		return fmt.Errorf("prune cleanup build cache: %w", err)
	}
	if volumes {
		if _, err := docker.VolumePrune(ctx, client.VolumePruneOptions{}); err != nil {
			return fmt.Errorf("prune cleanup volumes: %w", err)
		}
	}
	return nil
}
