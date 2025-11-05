package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	imageapi "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// DockerContainerRuntime executes containers using the local Docker daemon.
type DockerContainerRuntime struct {
	client *client.Client
	opts   DockerContainerRuntimeOptions
}

// NewDockerContainerRuntime constructs a Docker-backed container runtime.
func NewDockerContainerRuntime(opts DockerContainerRuntimeOptions) (ContainerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("step: configure docker runtime: %w", err)
	}
	return &DockerContainerRuntime{client: cli, opts: opts}, nil
}

// Create prepares a container.
func (r *DockerContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	if r == nil || r.client == nil {
		return ContainerHandle{}, errors.New("step: docker runtime not configured")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return ContainerHandle{}, errors.New("step: container image required")
	}
	if r.opts.PullImage {
		if err := r.pullImage(ctx, spec.Image); err != nil {
			return ContainerHandle{}, err
		}
	}
	config := &container.Config{Image: spec.Image, Cmd: append([]string{}, spec.Command...), WorkingDir: spec.WorkingDir, Env: flattenEnv(spec.Env), Labels: spec.Labels}
	// Disable AutoRemove to ensure we can reliably fetch logs after the
	// container exits. We handle explicit deletion in the runner based on
	// manifest retention settings.
	hostCfg := &container.HostConfig{AutoRemove: false, Mounts: convertMounts(spec.Mounts)}
	// Apply optional resource limits when provided (0 => unlimited).
	if spec.LimitNanoCPUs > 0 || spec.LimitMemoryBytes > 0 {
		hostCfg.Resources.NanoCPUs = spec.LimitNanoCPUs
		hostCfg.Resources.Memory = spec.LimitMemoryBytes
	}
	created, err := r.client.ContainerCreate(ctx, config, hostCfg, nil, nil, "")
	if err != nil {
		return ContainerHandle{}, fmt.Errorf("step: create container: %w", err)
	}
	return ContainerHandle{ID: created.ID}, nil
}

// Start launches the container.
func (r *DockerContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	return r.client.ContainerStart(ctx, handle.ID, container.StartOptions{})
}

// Wait waits for termination.
func (r *DockerContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	if r == nil || r.client == nil {
		return ContainerResult{}, errors.New("step: docker runtime not configured")
	}
	statusCh, errCh := r.client.ContainerWait(ctx, handle.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return ContainerResult{}, fmt.Errorf("step: wait container: %w", err)
		}
	case status := <-statusCh:
		res := ContainerResult{ExitCode: int(status.StatusCode)}
		inspect, err := r.client.ContainerInspect(ctx, handle.ID)
		if err == nil && inspect.ContainerJSONBase != nil && inspect.State != nil {
			res.StartedAt = parseDockerTime(inspect.State.StartedAt)
			res.CompletedAt = parseDockerTime(inspect.State.FinishedAt)
		}
		return res, nil
	}
	return ContainerResult{}, errors.New("step: container wait interrupted")
}

// Logs returns combined stdout/stderr.
func (r *DockerContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("step: docker runtime not configured")
	}
	reader, err := r.client.ContainerLogs(ctx, handle.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("step: fetch container logs: %w", err)
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

// Remove deletes the container.
func (r *DockerContainerRuntime) Remove(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	// Force remove to ensure cleanup even if some resources linger.
	return r.client.ContainerRemove(ctx, handle.ID, container.RemoveOptions{Force: true})
}

func (r *DockerContainerRuntime) pullImage(ctx context.Context, imageRef string) error {
	reader, err := r.client.ImagePull(ctx, imageRef, imageapi.PullOptions{})
	if err != nil {
		return fmt.Errorf("step: pull image %s: %w", imageRef, err)
	}
	defer func() { _ = reader.Close() }()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

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

func convertMounts(mounts []ContainerMount) []mount.Mount {
	if len(mounts) == 0 {
		return nil
	}
	res := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		res = append(res, mount.Mount{Type: mount.TypeBind, Source: m.Source, Target: m.Target, ReadOnly: m.ReadOnly})
	}
	return res
}

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
