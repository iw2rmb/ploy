package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// minDockerFreeBytes is the minimum free disk space (1 GiB) required in the
// Docker root directory before claiming new work. Below this threshold the
// pre-claim cleanup removes sticky /share volumes, sticky workspaces, then
// stopped Ploy containers to reclaim space.
const minDockerFreeBytes int64 = 1 << 30

type claimCleanupDockerClient interface {
	Info(ctx context.Context, options client.InfoOptions) (client.SystemInfoResult, error)
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

type freeBytesFunc func(path string) (int64, error)

type dockerPreClaimCleanup struct {
	docker        claimCleanupDockerClient
	freeBytes     freeBytesFunc
	workspaceRoot string
}

func newDockerPreClaimCleanup() (preClaimCleanupFunc, error) {
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	cleanup := &dockerPreClaimCleanup{
		docker:        dockerClient,
		freeBytes:     dockerRootFreeBytes,
		workspaceRoot: runCacheRootDir(),
	}
	return cleanup.EnsureCapacity, nil
}

func (c *dockerPreClaimCleanup) EnsureCapacity(ctx context.Context) (bool, error) {
	if c == nil || c.docker == nil || c.freeBytes == nil {
		return false, errors.New("pre-claim cleanup not configured")
	}
	workspaceRoot := strings.TrimSpace(c.workspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = runCacheRootDir()
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return false, fmt.Errorf("ensure workspace root %q: %w", workspaceRoot, err)
	}

	info, err := c.docker.Info(ctx, client.InfoOptions{})
	if err != nil {
		return false, fmt.Errorf("docker info: %w", err)
	}
	dockerRoot := strings.TrimSpace(info.Info.DockerRootDir)
	if dockerRoot == "" {
		return false, errors.New("docker info: empty docker root dir")
	}

	capacity, err := c.readCapacity(dockerRoot, workspaceRoot)
	if err != nil {
		return false, err
	}
	if capacity.enough() {
		return true, nil
	}
	slog.Warn(
		"pre-claim disk guard detected low capacity",
		"docker_root", dockerRoot,
		"workspace_root", workspaceRoot,
		"docker_free_bytes", capacity.dockerFreeBytes,
		"workspace_free_bytes", capacity.workspaceFreeBytes,
		"threshold_bytes", minDockerFreeBytes,
	)

	stickyShares, err := listStickyShareVolumes(workspaceRoot)
	if err != nil {
		return false, fmt.Errorf("list sticky share volumes: %w", err)
	}
	removedShares := 0
	for _, share := range stickyShares {
		if capacity.enough() {
			return true, nil
		}
		capacityBefore := capacity
		if err := os.RemoveAll(share.path); err != nil {
			return false, fmt.Errorf("remove sticky share volume %q: %w", share.path, err)
		}
		removedShares++
		capacity, err = c.readCapacity(dockerRoot, workspaceRoot)
		if err != nil {
			return false, err
		}
		slog.Info(
			"pre-claim disk cleanup removed sticky share volume",
			"share_path", share.path,
			"share_mod_time", share.modTime,
			"docker_free_bytes_before", capacityBefore.dockerFreeBytes,
			"docker_free_bytes_after", capacity.dockerFreeBytes,
			"workspace_free_bytes_before", capacityBefore.workspaceFreeBytes,
			"workspace_free_bytes_after", capacity.workspaceFreeBytes,
			"threshold_bytes", minDockerFreeBytes,
		)
	}
	if capacity.enough() {
		slog.Info(
			"pre-claim disk cleanup restored capacity via share-volume eviction",
			"docker_root", dockerRoot,
			"workspace_root", workspaceRoot,
			"docker_free_bytes", capacity.dockerFreeBytes,
			"workspace_free_bytes", capacity.workspaceFreeBytes,
			"threshold_bytes", minDockerFreeBytes,
			"removed_share_volumes", removedShares,
		)
		return true, nil
	}

	stickyWorkspaces, err := listStickyWorkspaces(workspaceRoot)
	if err != nil {
		return false, fmt.Errorf("list sticky workspaces: %w", err)
	}
	removedWorkspaces := 0
	for _, ws := range stickyWorkspaces {
		if capacity.enough() {
			return true, nil
		}
		capacityBefore := capacity
		if err := os.RemoveAll(ws.path); err != nil {
			return false, fmt.Errorf("remove sticky workspace %q: %w", ws.path, err)
		}
		removedWorkspaces++
		capacity, err = c.readCapacity(dockerRoot, workspaceRoot)
		if err != nil {
			return false, err
		}
		slog.Info(
			"pre-claim disk cleanup removed sticky workspace",
			"workspace_path", ws.path,
			"workspace_mod_time", ws.modTime,
			"docker_free_bytes_before", capacityBefore.dockerFreeBytes,
			"docker_free_bytes_after", capacity.dockerFreeBytes,
			"workspace_free_bytes_before", capacityBefore.workspaceFreeBytes,
			"workspace_free_bytes_after", capacity.workspaceFreeBytes,
			"threshold_bytes", minDockerFreeBytes,
		)
	}
	if capacity.enough() {
		slog.Info(
			"pre-claim disk cleanup restored capacity via workspace eviction",
			"docker_root", dockerRoot,
			"workspace_root", workspaceRoot,
			"docker_free_bytes", capacity.dockerFreeBytes,
			"workspace_free_bytes", capacity.workspaceFreeBytes,
			"threshold_bytes", minDockerFreeBytes,
			"removed_workspaces", removedWorkspaces,
			"removed_share_volumes", removedShares,
		)
		return true, nil
	}

	listed, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return false, fmt.Errorf("list containers: %w", err)
	}
	eligible := eligibleCleanupContainers(listed.Items)
	removed := 0

	for _, summary := range eligible {
		if capacity.enough() {
			return true, nil
		}
		capacityBefore := capacity
		if _, err := c.docker.ContainerRemove(ctx, summary.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
			return false, fmt.Errorf("remove container %s: %w", summary.ID, err)
		}
		removed++

		capacity, err = c.readCapacity(dockerRoot, workspaceRoot)
		if err != nil {
			return false, err
		}
		slog.Info(
			"pre-claim disk cleanup removed container",
			"docker_root", dockerRoot,
			"container_id", summary.ID,
			"created", summary.Created,
			"docker_free_bytes_before", capacityBefore.dockerFreeBytes,
			"docker_free_bytes_after", capacity.dockerFreeBytes,
			"workspace_free_bytes_before", capacityBefore.workspaceFreeBytes,
			"workspace_free_bytes_after", capacity.workspaceFreeBytes,
			"threshold_bytes", minDockerFreeBytes,
		)
	}

	if capacity.enough() {
		slog.Info(
			"pre-claim disk cleanup restored capacity",
			"docker_root", dockerRoot,
			"workspace_root", workspaceRoot,
			"docker_free_bytes", capacity.dockerFreeBytes,
			"workspace_free_bytes", capacity.workspaceFreeBytes,
			"threshold_bytes", minDockerFreeBytes,
			"removed_containers", removed,
			"removed_workspaces", removedWorkspaces,
			"removed_share_volumes", removedShares,
		)
		return true, nil
	}
	slog.Warn(
		"pre-claim disk cleanup exhausted eligible resources",
		"docker_root", dockerRoot,
		"workspace_root", workspaceRoot,
		"docker_free_bytes", capacity.dockerFreeBytes,
		"workspace_free_bytes", capacity.workspaceFreeBytes,
		"threshold_bytes", minDockerFreeBytes,
		"removed_containers", removed,
		"removed_workspaces", removedWorkspaces,
		"removed_share_volumes", removedShares,
		"eligible_share_volumes", len(stickyShares),
		"eligible_workspaces", len(stickyWorkspaces),
		"eligible_containers", len(eligible),
	)
	return false, nil
}

type diskCapacity struct {
	dockerFreeBytes    int64
	workspaceFreeBytes int64
}

func (d diskCapacity) enough() bool {
	return d.dockerFreeBytes >= minDockerFreeBytes && d.workspaceFreeBytes >= minDockerFreeBytes
}

func (c *dockerPreClaimCleanup) readCapacity(dockerRoot, workspaceRoot string) (diskCapacity, error) {
	dockerFree, err := c.freeBytes(dockerRoot)
	if err != nil {
		return diskCapacity{}, fmt.Errorf("free bytes for docker root %q: %w", dockerRoot, err)
	}
	workspaceFree, err := c.freeBytes(workspaceRoot)
	if err != nil {
		return diskCapacity{}, fmt.Errorf("free bytes for workspace root %q: %w", workspaceRoot, err)
	}
	return diskCapacity{
		dockerFreeBytes:    dockerFree,
		workspaceFreeBytes: workspaceFree,
	}, nil
}

type stickyWorkspaceCandidate struct {
	path    string
	modTime time.Time
}

func listStickyWorkspaces(workspaceRoot string) ([]stickyWorkspaceCandidate, error) {
	return listStickyRunRepoDirs(workspaceRoot, "workspace")
}

func listStickyShareVolumes(workspaceRoot string) ([]stickyWorkspaceCandidate, error) {
	return listStickyRunRepoDirs(workspaceRoot, "share")
}

func listStickyRunRepoDirs(workspaceRoot, leaf string) ([]stickyWorkspaceCandidate, error) {
	pattern := filepath.Join(workspaceRoot, "*", "repos", "*", leaf)
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %q: %w", pattern, err)
	}
	candidates := make([]stickyWorkspaceCandidate, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat sticky dir %q: %w", p, err)
		}
		if !info.IsDir() {
			continue
		}
		candidates = append(candidates, stickyWorkspaceCandidate{
			path:    p,
			modTime: info.ModTime().UTC(),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path < candidates[j].path
		}
		return candidates[i].modTime.Before(candidates[j].modTime)
	})
	return candidates, nil
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
