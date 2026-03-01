package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"syscall"

	"github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// minDockerFreeBytes is the minimum free disk space (1 GiB) required in the
// Docker root directory before claiming new work. Below this threshold the
// pre-claim cleanup removes stopped Ploy containers to reclaim space.
const minDockerFreeBytes int64 = 1 << 30

type claimCleanupDockerClient interface {
	Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type freeBytesFunc func(path string) (int64, error)

type dockerPreClaimCleanup struct {
	docker    claimCleanupDockerClient
	freeBytes freeBytesFunc
}

func newDockerPreClaimCleanup() (preClaimCleanupFunc, error) {
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	cleanup := &dockerPreClaimCleanup{
		docker:    dockerClient,
		freeBytes: dockerRootFreeBytes,
	}
	return cleanup.EnsureCapacity, nil
}

func (c *dockerPreClaimCleanup) EnsureCapacity(ctx context.Context) (bool, error) {
	if c == nil || c.docker == nil || c.freeBytes == nil {
		return false, errors.New("pre-claim cleanup not configured")
	}

	info, err := c.docker.Info(ctx, client.InfoOptions{})
	if err != nil {
		return false, fmt.Errorf("docker info: %w", err)
	}
	dockerRoot := strings.TrimSpace(info.Info.DockerRootDir)
	if dockerRoot == "" {
		return false, errors.New("docker info: empty docker root dir")
	}

	free, err := c.freeBytes(dockerRoot)
	if err != nil {
		return false, fmt.Errorf("free bytes for docker root %q: %w", dockerRoot, err)
	}
	if free >= minDockerFreeBytes {
		return true, nil
	}
	slog.Warn(
		"pre-claim disk guard detected low docker-root capacity",
		"docker_root", dockerRoot,
		"free_bytes", free,
		"threshold_bytes", minDockerFreeBytes,
	)

	listed, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return false, fmt.Errorf("list containers: %w", err)
	}
	eligible := eligibleCleanupContainers(listed.Items)
	removed := 0

	for _, summary := range eligible {
		if free >= minDockerFreeBytes {
			return true, nil
		}
		freeBefore := free
		if _, err := c.docker.ContainerRemove(ctx, summary.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return false, fmt.Errorf("remove container %s: %w", summary.ID, err)
		}
		removed++

		free, err = c.freeBytes(dockerRoot)
		if err != nil {
			return false, fmt.Errorf("free bytes for docker root %q after removing %s: %w", dockerRoot, summary.ID, err)
		}
		slog.Info(
			"pre-claim disk cleanup removed container",
			"docker_root", dockerRoot,
			"container_id", summary.ID,
			"created", summary.Created,
			"free_bytes_before", freeBefore,
			"free_bytes_after", free,
			"threshold_bytes", minDockerFreeBytes,
		)
	}

	if free >= minDockerFreeBytes {
		slog.Info(
			"pre-claim disk cleanup restored capacity",
			"docker_root", dockerRoot,
			"free_bytes", free,
			"threshold_bytes", minDockerFreeBytes,
			"removed_containers", removed,
		)
		return true, nil
	}
	slog.Warn(
		"pre-claim disk cleanup exhausted eligible containers",
		"docker_root", dockerRoot,
		"free_bytes", free,
		"threshold_bytes", minDockerFreeBytes,
		"removed_containers", removed,
		"eligible_containers", len(eligible),
	)
	return false, nil
}

func eligibleCleanupContainers(containers []containertypes.Summary) []containertypes.Summary {
	eligible := make([]containertypes.Summary, 0, len(containers))
	for _, summary := range containers {
		if !isPloyManaged(summary.Labels) {
			continue
		}
		if summary.State == containertypes.StateRunning {
			continue
		}
		eligible = append(eligible, summary)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Created == eligible[j].Created {
			return eligible[i].ID < eligible[j].ID
		}
		return eligible[i].Created < eligible[j].Created
	})

	return eligible
}

func isPloyManaged(labels map[string]string) bool {
	if len(labels) == 0 {
		return false
	}
	if strings.TrimSpace(labels[types.LabelRunID]) != "" {
		return true
	}
	return strings.TrimSpace(labels[types.LabelJobID]) != ""
}

func dockerRootFreeBytes(path string) (int64, error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0, err
	}

	free := uint64(stats.Bavail) * uint64(stats.Bsize)
	if free > math.MaxInt64 {
		return math.MaxInt64, nil
	}
	return int64(free), nil
}
