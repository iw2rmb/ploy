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

// DockerContainerRuntimeOptions configures the Docker-backed container runtime.
type DockerContainerRuntimeOptions struct {
	// Network defines the docker network name. Defaults to bridge when empty.
	Network string
	// PullImage controls whether the runtime pulls images before execution.
	PullImage bool
}

// DockerContainerRuntime executes step containers using the local Docker daemon.
type DockerContainerRuntime struct {
	client *client.Client
	opts   DockerContainerRuntimeOptions
}

// NewDockerContainerRuntime constructs a container runtime backed by Docker.
func NewDockerContainerRuntime(opts DockerContainerRuntimeOptions) (*DockerContainerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("step: configure docker runtime: %w", err)
	}
	return &DockerContainerRuntime{
		client: cli,
		opts:   opts,
	}, nil
}

// Create prepares the container according to the provided spec.
func (r *DockerContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error) {
	if r == nil || r.client == nil {
		return ContainerHandle{}, errors.New("step: docker runtime not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(spec.Image) == "" {
		return ContainerHandle{}, errors.New("step: container image required")
	}

	if r.opts.PullImage {
		if err := r.pullImage(ctx, spec.Image); err != nil {
			return ContainerHandle{}, err
		}
	}

	config := &container.Config{
		Image:      spec.Image,
		Cmd:        append([]string{}, spec.Command...),
		WorkingDir: spec.WorkingDir,
		Env:        flattenEnv(spec.Env),
		Tty:        false,
	}
	hostConfig := &container.HostConfig{
		AutoRemove: false,
		Mounts:     convertMounts(spec.Mounts),
	}
	if network := strings.TrimSpace(r.opts.Network); network != "" {
		hostConfig.NetworkMode = container.NetworkMode(network)
	}

	created, err := r.client.ContainerCreate(ctx, config, hostConfig, nil, nil, "")
	if err != nil {
		return ContainerHandle{}, fmt.Errorf("step: create container: %w", err)
	}
	return ContainerHandle{ID: created.ID}, nil
}

// Start launches the prepared container.
func (r *DockerContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return r.client.ContainerStart(ctx, handle.ID, container.StartOptions{})
}

// Wait waits for the container to exit and returns execution metadata.
func (r *DockerContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	if r == nil || r.client == nil {
		return ContainerResult{}, errors.New("step: docker runtime not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	statusCh, errCh := r.client.ContainerWait(ctx, handle.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return ContainerResult{}, fmt.Errorf("step: wait container: %w", err)
		}
	case status := <-statusCh:
		result := ContainerResult{
			ExitCode: int(status.StatusCode),
		}
		if status.Error != nil && status.Error.Message != "" {
			return result, fmt.Errorf("step: container error: %s", status.Error.Message)
		}
		inspect, err := r.client.ContainerInspect(ctx, handle.ID)
		if err == nil && inspect.ContainerJSONBase != nil && inspect.State != nil {
			result.StartedAt = parseDockerTime(inspect.State.StartedAt)
			result.CompletedAt = parseDockerTime(inspect.State.FinishedAt)
		}
		return result, nil
	}
	return ContainerResult{}, errors.New("step: container wait interrupted")
}

// Logs returns the combined stdout/stderr output for the container.
func (r *DockerContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("step: docker runtime not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reader, err := r.client.ContainerLogs(ctx, handle.ID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: false,
		Details:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("step: fetch container logs: %w", err)
	}
	defer func() {
		_ = reader.Close()
	}()
	return io.ReadAll(reader)
}

func (r *DockerContainerRuntime) pullImage(ctx context.Context, imageRef string) error {
	reader, err := r.client.ImagePull(ctx, imageRef, imageapi.PullOptions{})
	if err != nil {
		return fmt.Errorf("step: pull image %s: %w", imageRef, err)
	}
	defer func() {
		_ = reader.Close()
	}()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, fmt.Sprintf("%s=%s", strings.TrimSpace(key), value))
	}
	return out
}

func convertMounts(mounts []ContainerMount) []mount.Mount {
	if len(mounts) == 0 {
		return nil
	}
	result := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	return result
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
