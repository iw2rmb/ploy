package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"time"

	// Docker Engine v29 SDK modules (moby). These replace the deprecated
	// github.com/docker/docker imports with supported Engine v29 equivalents.
	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// dockerClientAPI abstracts the core moby client methods used by DockerContainerRuntime.
// This interface enables dependency injection for testing without requiring a live
// Docker daemon. It mirrors the moby Engine v29 SDK method signatures exactly.
type dockerClientAPI interface {
	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

// dockerImageAPI abstracts image operations (pull & inspect) for conditional image fetching.
type dockerImageAPI interface {
	ImagePull(ctx context.Context, refStr string, options client.ImagePullOptions) (client.ImagePullResponse, error)
	ImageInspect(ctx context.Context, imageID string, inspectOpts ...client.ImageInspectOption) (client.ImageInspectResult, error)
}

// dockerStatsAPI abstracts container stats retrieval, used by gate resource telemetry.
type dockerStatsAPI interface {
	ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
}

// DockerContainerRuntime executes containers using the local Docker daemon.
// It uses the moby Engine v29 SDK (github.com/moby/moby/client) for all
// Docker operations (create, start, wait, logs, remove, image pull).
type DockerContainerRuntime struct {
	// client abstracts core Docker container lifecycle calls.
	client dockerClientAPI
	// images handles image pull/inspect (nil when PullImage is false).
	images dockerImageAPI
	// stats handles container resource stats (nil-safe; used only by gate telemetry).
	stats dockerStatsAPI
	opts  DockerContainerRuntimeOptions
}

// NewDockerContainerRuntime constructs a Docker-backed container runtime.
// It uses client.FromEnv to read DOCKER_HOST and related environment variables,
// and WithAPIVersionNegotiation to auto-negotiate API version with the daemon.
// These two options ensure environment semantics are honoured identically to
// the prior github.com/docker/docker client construction.
func NewDockerContainerRuntime(opts DockerContainerRuntimeOptions) (ContainerRuntime, error) {
	// Use moby client with FromEnv for DOCKER_HOST, DOCKER_TLS_VERIFY, DOCKER_CERT_PATH,
	// and WithAPIVersionNegotiation to auto-negotiate API version with the daemon.
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("step: configure docker runtime: %w", err)
	}
	return &DockerContainerRuntime{client: cli, images: cli, stats: cli, opts: opts}, nil
}

// newDockerContainerRuntimeWithClient constructs a DockerContainerRuntime with
// an injected dockerClientAPI. Used for testing with fake Docker clients.
func newDockerContainerRuntimeWithClient(cli dockerClientAPI, opts DockerContainerRuntimeOptions) *DockerContainerRuntime {
	rt := &DockerContainerRuntime{client: cli, opts: opts}
	if img, ok := cli.(dockerImageAPI); ok {
		rt.images = img
	}
	if s, ok := cli.(dockerStatsAPI); ok {
		rt.stats = s
	}
	return rt
}

// Create prepares a container using the moby client ContainerCreate API.
// It configures the container image, command, environment, mounts, resource
// limits, network mode, and storage options based on the provided ContainerSpec.
//
// Engine v29 lifecycle semantics:
//   - ContainerCreate uses client.ContainerCreateOptions struct (not positional args).
//   - HostConfig.AutoRemove is explicitly set to false to ensure logs can be
//     retrieved after container exit — critical for workflow artifact collection.
//   - Mounts use mount.TypeBind for host path mounts; volume mounts not used.
//   - Resource limits (NanoCPUs, Memory) are set via HostConfig.Resources.
//   - NetworkMode is set via HostConfig.NetworkMode when configured.
//   - StorageOpt["size"] sets disk quota when supported by the storage driver.
func (r *DockerContainerRuntime) Create(ctx context.Context, spec ContainerSpec) (handle ContainerHandle, err error) {
	// Guard against unexpected panics inside the Docker SDK JSON/request path.
	// Panics here must not terminate the node process.
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("step: create container panic: %v\n%s", p, string(debug.Stack()))
			handle = ""
		}
	}()

	if r == nil || r.client == nil {
		return "", errors.New("step: docker runtime not configured")
	}
	if strings.TrimSpace(spec.Image) == "" {
		return "", errors.New("step: container image required")
	}
	// Pull image before creation if configured.
	if r.opts.PullImage {
		if err := r.ensureImageAvailable(ctx, spec.Image); err != nil {
			return "", err
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
		hostCfg.NanoCPUs = spec.LimitNanoCPUs
		hostCfg.Memory = spec.LimitMemoryBytes
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
		return "", fmt.Errorf("step: create container: %w", err)
	}
	return ContainerHandle(created.ID), nil
}

// Start launches the container using the moby client ContainerStart API.
// The moby v29 SDK returns (ContainerStartResult, error) but ContainerStartResult
// is currently empty; we discard it and return only the error status.
//
// Engine v29 lifecycle semantics:
//   - ContainerStart is async: returns immediately after the container process starts.
//   - Uses client.ContainerStartOptions (empty struct for default behavior).
//   - The container may still be initializing when Start returns successfully.
//   - Use Wait to block until the container actually exits.
func (r *DockerContainerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerStartOptions instead of
	// container.StartOptions; empty struct for default behavior.
	_, err := r.client.ContainerStart(ctx, string(handle), client.ContainerStartOptions{})
	return err
}

// Wait waits for container termination using the moby client ContainerWait API.
// The moby v29 SDK returns a ContainerWaitResult struct with Result and Error
// channels instead of returning two separate channels.
//
// Engine v29 lifecycle semantics:
//   - ContainerWait blocks until the container reaches the specified condition.
//   - Uses WaitConditionNotRunning to wait until container has completely stopped
//     (not just exited), ensuring all cleanup has occurred before proceeding.
//   - Returns ContainerWaitResult with Result and Error channels (not two channels).
//   - Exit code is obtained from container.WaitResponse.StatusCode.
//   - Timestamps (StartedAt, FinishedAt) are extracted via ContainerInspect
//     from ContainerInspectResult.Container.State fields.
//   - Context cancellation propagates to abort waiting if ctx is done.
func (r *DockerContainerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	if r == nil || r.client == nil {
		return ContainerResult{}, errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerWaitOptions with Condition field
	// instead of container.WaitCondition as a positional parameter. The result
	// is a struct with Result and Error channels.
	waitResult := r.client.ContainerWait(ctx, string(handle), client.ContainerWaitOptions{
		Condition: container.WaitConditionNotRunning,
	})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				r.forceRemoveOnWaitCancel(handle)
			}
			return ContainerResult{}, fmt.Errorf("step: wait container: %w", err)
		}
	case status := <-waitResult.Result:
		res := ContainerResult{ExitCode: int(status.StatusCode)}
		// Inspect container to extract start/finish timestamps.
		// Moby v29 SDK returns ContainerInspectResult with Container field.
		inspect, err := r.client.ContainerInspect(ctx, string(handle), client.ContainerInspectOptions{})
		if err == nil && inspect.Container.State != nil {
			res.StartedAt = parseDockerTime(inspect.Container.State.StartedAt)
			res.CompletedAt = parseDockerTime(inspect.Container.State.FinishedAt)
		}
		return res, nil
	}
	if ctx.Err() != nil {
		r.forceRemoveOnWaitCancel(handle)
	}
	return ContainerResult{}, errors.New("step: container wait interrupted")
}

func (r *DockerContainerRuntime) forceRemoveOnWaitCancel(handle ContainerHandle) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.client.ContainerRemove(cleanupCtx, string(handle), client.ContainerRemoveOptions{Force: true})
	if err != nil && !isContainerNotFound(err) {
		// Best effort cleanup: cancellation must return promptly even when remove fails.
		// Errors are intentionally ignored to preserve Wait() behavior contract.
		return
	}
}

func isContainerNotFound(err error) bool {
	if err == nil {
		return false
	}
	return cerrdefs.IsNotFound(err)
}

// Logs returns combined stdout/stderr from the container using the moby client
// ContainerLogs API. The method demultiplexes Docker's multiplexed stream format
// into separate stdout and stderr buffers, then concatenates them for callers
// that expect plain text output.
//
// Engine v29 log streaming semantics:
//   - ContainerLogs uses client.ContainerLogsOptions (not container.LogsOptions).
//   - Returns client.ContainerLogsResult (io.ReadCloser) with multiplexed stream.
//   - When TTY is disabled (default for workflow containers), Docker multiplexes
//     stdout and stderr into a single stream using an 8-byte header per frame:
//     [STREAM_TYPE, 0, 0, 0, SIZE_BE32] + payload.
//   - STREAM_TYPE: 0=stdin (unused), 1=stdout, 2=stderr.
//   - SIZE_BE32: big-endian uint32 length of the payload.
//
// Demuxing with moby stdcopy:
//   - The moby SDK provides stdcopy.StdCopy at github.com/moby/moby/api/pkg/stdcopy.
//   - The old path github.com/docker/docker/pkg/stdcopy is deprecated in Engine v29.
//   - stdcopy.StdCopy reads multiplexed frames and writes to separate stdout/stderr
//     io.Writers, preserving content order within each stream.
//   - We combine stdout then stderr for workflow artifact collection, matching
//     the format expected by downstream log processors.
//
// Fallback behaviour:
//   - On demux errors (e.g., corrupted stream, TTY mode), we attempt to read
//     raw bytes to avoid total data loss.
//   - This handles edge cases where the container was started with TTY enabled
//     (non-multiplexed) or the stream is malformed.
func (r *DockerContainerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerLogsOptions (ShowStdout, ShowStderr).
	// Returns client.ContainerLogsResult which embeds io.ReadCloser.
	reader, err := r.client.ContainerLogs(ctx, string(handle), client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("step: fetch container logs: %w", err)
	}
	defer func() { _ = reader.Close() }()

	// Demultiplex Docker's multiplexed stream format using stdcopy.StdCopy.
	// This separates stdout and stderr frames based on the 8-byte header prefix.
	// Import path: github.com/moby/moby/api/pkg/stdcopy (the deprecated
	// github.com/docker/docker/pkg/stdcopy should not be used with Engine v29).
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, reader); err != nil {
		// Fall back to raw bytes on demux errors (TTY mode, corruption).
		// The reader may be partially consumed, so ReadAll gets remaining data.
		raw, readErr := io.ReadAll(reader)
		return raw, readErr
	}

	// Combine stdout then stderr for consumers expecting plain text.
	// This ordering matches workflow artifact collection semantics where
	// stdout (build output) precedes stderr (warnings/errors).
	return append(stdoutBuf.Bytes(), stderrBuf.Bytes()...), nil
}

// Remove deletes the container using the moby client ContainerRemove API.
// Force remove is used to ensure cleanup even if some resources linger.
//
// Engine v29 lifecycle semantics:
//   - ContainerRemove uses client.ContainerRemoveOptions (not container.RemoveOptions).
//   - Force=true ensures removal even if container is still running or has
//     lingering resources, preventing orphaned containers from accumulating.
//   - Returns (ContainerRemoveResult, error); result is empty, we discard it.
//   - Removal is idempotent: removing an already-removed container may return
//     an error (404) but does not leave the system in an inconsistent state.
func (r *DockerContainerRuntime) Remove(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	// Moby Engine v29 SDK uses client.ContainerRemoveOptions instead of
	// container.RemoveOptions; same field names (Force, RemoveVolumes, RemoveLinks).
	// Returns (ContainerRemoveResult, error); result is empty, discard it.
	_, err := r.client.ContainerRemove(ctx, string(handle), client.ContainerRemoveOptions{Force: true})
	return err
}

// ensureImageAvailable checks whether the image exists locally and pulls it from
// a registry only when it is missing.
//
// This avoids failing local development runs when tags (e.g. gate-gradle:jdk11)
// are built locally and not published to a registry.
func (r *DockerContainerRuntime) ensureImageAvailable(ctx context.Context, imageRef string) error {
	_, err := r.images.ImageInspect(ctx, imageRef)
	if err == nil {
		return nil
	}
	if cerrdefs.IsNotFound(err) {
		return r.pullImage(ctx, imageRef)
	}
	return fmt.Errorf("step: inspect image %s: %w", imageRef, err)
}

// pullImage pulls the specified image using the moby client ImagePull API.
// The moby v29 SDK uses client.ImagePullOptions and returns ImagePullResponse.
func (r *DockerContainerRuntime) pullImage(ctx context.Context, imageRef string) error {
	registryAuth, err := r.registryAuthForImage(imageRef)
	if err != nil {
		return fmt.Errorf("step: pull image %s: %w", imageRef, err)
	}

	// Moby Engine v29 SDK uses client.ImagePullOptions instead of
	// imageapi.PullOptions; same field names (RegistryAuth, PrivilegeFunc, etc.).
	reader, err := r.images.ImagePull(ctx, imageRef, client.ImagePullOptions{
		RegistryAuth: registryAuth,
	})
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
