package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	// Docker Engine v29 SDK modules (moby). These replace the deprecated
	// github.com/docker/docker imports with supported Engine v29 equivalents.
	// See ROADMAP.md "Migrate workflow runtime packages to moby client and types".
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// DockerContainerRuntime executes containers using the local Docker daemon.
// It uses the moby Engine v29 SDK (github.com/moby/moby/client) for all
// Docker operations (create, start, wait, logs, remove, image pull).
type DockerContainerRuntime struct {
	client *client.Client
	opts   DockerContainerRuntimeOptions
}

// NewDockerContainerRuntime constructs a Docker-backed container runtime.
// It uses client.FromEnv to read DOCKER_HOST and related environment variables,
// and WithAPIVersionNegotiation to auto-negotiate API version with the daemon.
func NewDockerContainerRuntime(opts DockerContainerRuntimeOptions) (ContainerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("step: configure docker runtime: %w", err)
	}
	return &DockerContainerRuntime{client: cli, opts: opts}, nil
}

// Create prepares a container using the moby client ContainerCreate API.
// It configures the container image, command, environment, mounts, resource
// limits, network mode, and storage options based on the provided ContainerSpec.
func (r *DockerContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	if r == nil || r.client == nil {
		return ContainerHandle{}, errors.New("step: docker runtime not configured")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return ContainerHandle{}, errors.New("step: container image required")
	}
	// Pull image before creation if configured.
	if r.opts.PullImage {
		if err := r.pullImage(ctx, spec.Image); err != nil {
			return ContainerHandle{}, err
		}
	}
	// Build container configuration with image, command, workdir, env, and labels.
	config := &container.Config{
		Image:      spec.Image,
		Cmd:        append([]string{}, spec.Command...),
		WorkingDir: spec.WorkingDir,
		Env:        flattenEnv(spec.Env),
		Labels:     spec.Labels,
	}
	// Build host configuration with mounts and resource constraints.
	// Disable AutoRemove to ensure we can reliably fetch logs after the
	// container exits. We handle explicit deletion in the runner based on
	// manifest retention settings.
	hostCfg := &container.HostConfig{
		AutoRemove: false,
		Mounts:     convertMounts(spec.Mounts),
	}
	// Apply optional resource limits when provided (0 => unlimited).
	if spec.LimitNanoCPUs > 0 || spec.LimitMemoryBytes > 0 {
		hostCfg.Resources.NanoCPUs = spec.LimitNanoCPUs
		hostCfg.Resources.Memory = spec.LimitMemoryBytes
	}
	// Optional: attach container to a specific Docker network when configured.
	if strings.TrimSpace(r.opts.Network) != "" {
		hostCfg.NetworkMode = container.NetworkMode(strings.TrimSpace(r.opts.Network))
	}
	// Optional disk size limit (driver dependent; e.g., overlay2 with xfs project quota).
	if strings.TrimSpace(spec.StorageSizeOpt) != "" {
		if hostCfg.StorageOpt == nil {
			hostCfg.StorageOpt = map[string]string{}
		}
		hostCfg.StorageOpt["size"] = strings.TrimSpace(spec.StorageSizeOpt)
	}
	// Moby Engine v29 SDK uses client.ContainerCreateOptions struct instead of
	// positional parameters. Config, HostConfig, NetworkingConfig, Platform, Name.
	created, err := r.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     config,
		HostConfig: hostCfg,
		// NetworkingConfig and Platform left nil for default behavior.
		// Name left empty for auto-generated container name.
	})
	if err != nil {
		return ContainerHandle{}, fmt.Errorf("step: create container: %w", err)
	}
	return ContainerHandle{ID: created.ID}, nil
}

// Start launches the container using the moby client ContainerStart API.
// The moby v29 SDK returns (ContainerStartResult, error) but ContainerStartResult
// is currently empty; we discard it and return only the error status.
func (r *DockerContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerStartOptions instead of
	// container.StartOptions; empty struct for default behavior.
	_, err := r.client.ContainerStart(ctx, handle.ID, client.ContainerStartOptions{})
	return err
}

// Wait waits for container termination using the moby client ContainerWait API.
// The moby v29 SDK returns a ContainerWaitResult struct with Result and Error
// channels instead of returning two separate channels.
func (r *DockerContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	if r == nil || r.client == nil {
		return ContainerResult{}, errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerWaitOptions with Condition field
	// instead of container.WaitCondition as a positional parameter. The result
	// is a struct with Result and Error channels.
	waitResult := r.client.ContainerWait(ctx, handle.ID, client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			return ContainerResult{}, fmt.Errorf("step: wait container: %w", err)
		}
	case status := <-waitResult.Result:
		res := ContainerResult{ExitCode: int(status.StatusCode)}
		// Inspect container to extract start/finish timestamps.
		// Moby v29 SDK returns ContainerInspectResult with Container field.
		inspect, err := r.client.ContainerInspect(ctx, handle.ID, client.ContainerInspectOptions{})
		if err == nil && inspect.Container.State != nil {
			res.StartedAt = parseDockerTime(inspect.Container.State.StartedAt)
			res.CompletedAt = parseDockerTime(inspect.Container.State.FinishedAt)
		}
		return res, nil
	}
	return ContainerResult{}, errors.New("step: container wait interrupted")
}

// Logs returns combined stdout/stderr from the container using the moby client
// ContainerLogs API. Docker returns a multiplexed stream when TTY is not enabled;
// we demultiplex using stdcopy.StdCopy and combine stdout+stderr.
func (r *DockerContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerLogsOptions instead of
	// container.LogsOptions; same field names (ShowStdout, ShowStderr).
	reader, err := r.client.ContainerLogs(ctx, handle.ID, client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("step: fetch container logs: %w", err)
	}
	defer func() { _ = reader.Close() }()
	// Docker returns a multiplexed stream when TTY is not enabled. Demultiplex
	// into combined stdout+stderr for consumers that expect plain text.
	// stdcopy is now at github.com/moby/moby/api/pkg/stdcopy.
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, reader); err != nil {
		// Fall back to raw bytes on demux errors to avoid losing logs entirely.
		raw, _ := io.ReadAll(reader)
		return raw, nil
	}
	return append(stdoutBuf.Bytes(), stderrBuf.Bytes()...), nil
}

// Remove deletes the container using the moby client ContainerRemove API.
// Force remove is used to ensure cleanup even if some resources linger.
func (r *DockerContainerRuntime) Remove(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerRemoveOptions instead of
	// container.RemoveOptions; same field names (Force, RemoveVolumes, RemoveLinks).
	// Returns (ContainerRemoveResult, error); result is empty, discard it.
	_, err := r.client.ContainerRemove(ctx, handle.ID, client.ContainerRemoveOptions{Force: true})
	return err
}

// pullImage pulls the specified image using the moby client ImagePull API.
// The moby v29 SDK uses client.ImagePullOptions and returns ImagePullResponse.
func (r *DockerContainerRuntime) pullImage(ctx context.Context, imageRef string) error {
	// Moby Engine v29 SDK uses client.ImagePullOptions instead of
	// imageapi.PullOptions; same field names (RegistryAuth, PrivilegeFunc, etc.).
	reader, err := r.client.ImagePull(ctx, imageRef, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("step: pull image %s: %w", imageRef, err)
	}
	defer func() { _ = reader.Close() }()
	// Drain the response to ensure the pull completes before returning.
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

// flattenEnv converts a map[string]string environment to []string "K=V" format.
func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", strings.TrimSpace(k), v))
	}
	return out
}

// convertMounts converts ContainerMount slice to moby mount.Mount slice.
// All mounts are bind mounts (mount.TypeBind).
func convertMounts(mounts []ContainerMount) []mount.Mount {
	if len(mounts) == 0 {
		return nil
	}
	res := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		res = append(res, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	return res
}

// parseDockerTime parses Docker RFC3339Nano timestamp strings into time.Time.
func parseDockerTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return ts
}
