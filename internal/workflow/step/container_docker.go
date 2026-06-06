package step

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

// dockerClientAPI abstracts the core moby client methods used by containerRuntime.
// This interface enables dependency injection for testing without requiring a live
// Docker daemon.
type dockerClientAPI interface {
	ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

// dockerImageAPI abstracts image operations for Docker image fetching.
type dockerImageAPI interface {
	ImagePull(ctx context.Context, refStr string, options client.ImagePullOptions) (client.ImagePullResponse, error)
	ImageInspect(ctx context.Context, imageID string, inspectOpts ...client.ImageInspectOption) (client.ImageInspectResult, error)
}

// dockerStatsAPI abstracts container stats retrieval, used by gate resource telemetry.
type dockerStatsAPI interface {
	ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error)
}

// containerRuntime executes containers using the local Docker daemon.
type containerRuntime struct {
	client dockerClientAPI
	images dockerImageAPI
	stats  dockerStatsAPI
	opts   ContainerRuntimeOptions
}

// NewContainerRuntime constructs a Docker-backed container runtime.
// It uses client.FromEnv to read DOCKER_HOST and related environment variables,
// and WithAPIVersionNegotiation to auto-negotiate API version with the daemon.
func NewContainerRuntime(opts ContainerRuntimeOptions) (ContainerRuntime, error) {
	cli, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("step: configure docker runtime: %w", err)
	}
	return &containerRuntime{client: cli, images: cli, stats: cli, opts: opts}, nil
}

// newContainerRuntimeWithClient constructs a containerRuntime with
// an injected dockerClientAPI. Used for testing with fake Docker clients.
func newContainerRuntimeWithClient(cli dockerClientAPI, opts ContainerRuntimeOptions) *containerRuntime {
	rt := &containerRuntime{client: cli, opts: opts}
	if img, ok := cli.(dockerImageAPI); ok {
		rt.images = img
	}
	if s, ok := cli.(dockerStatsAPI); ok {
		rt.stats = s
	}
	return rt
}

// Create prepares a container based on the provided ContainerSpec.
// HostConfig.AutoRemove is set to false so logs remain retrievable after exit;
// explicit removal is owned by the caller (node-runtime disk-pressure flow).
func (r *containerRuntime) Create(ctx context.Context, spec ContainerSpec) (handle ContainerHandle, err error) {
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
	if r.opts.PullImage {
		if err := r.ensureImageAvailable(ctx, spec.Image); err != nil {
			return "", err
		}
	}
	config := &container.Config{
		Image:      spec.Image,
		Cmd:        append([]string{}, spec.Command...),
		WorkingDir: spec.WorkingDir,
		Env:        flattenEnv(spec.Env),
		Labels:     spec.Labels,
	}
	hostCfg := &container.HostConfig{
		AutoRemove: false,
		Mounts:     convertMounts(r.withDockerAuthMount(spec.Mounts)),
	}
	if spec.LimitNanoCPUs > 0 || spec.LimitMemoryBytes > 0 {
		hostCfg.NanoCPUs = spec.LimitNanoCPUs
		hostCfg.Memory = spec.LimitMemoryBytes
	}
	if strings.TrimSpace(r.opts.Network) != "" {
		hostCfg.NetworkMode = container.NetworkMode(strings.TrimSpace(r.opts.Network))
	}
	if strings.TrimSpace(spec.StorageSizeOpt) != "" {
		if hostCfg.StorageOpt == nil {
			hostCfg.StorageOpt = map[string]string{}
		}
		hostCfg.StorageOpt["size"] = strings.TrimSpace(spec.StorageSizeOpt)
	}
	created, err := r.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     config,
		HostConfig: hostCfg,
	})
	if err != nil {
		return "", fmt.Errorf("step: create container: %w", err)
	}
	return ContainerHandle(created.ID), nil
}

func (r *containerRuntime) withDockerAuthMount(mounts []ContainerMount) []ContainerMount {
	authConfigFile := strings.TrimSpace(r.opts.RegistryAuthConfigFile)
	if authConfigFile == "" ||
		!hasMountTarget(mounts, "/var/run/docker.sock") ||
		hasMountTarget(mounts, "/root/.docker") ||
		hasMountTarget(mounts, "/root/.docker/config.json") {
		return mounts
	}
	return append(append([]ContainerMount{}, mounts...), ContainerMount{
		Source:   filepath.Dir(authConfigFile),
		Target:   "/root/.docker",
		ReadOnly: true,
	})
}

func hasMountTarget(mounts []ContainerMount, target string) bool {
	for _, m := range mounts {
		if m.Target == target {
			return true
		}
	}
	return false
}

// Start launches the container. ContainerStart is async — the container may still
// be initializing when Start returns successfully. Use Wait to block until exit.
func (r *containerRuntime) Start(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	_, err := r.client.ContainerStart(ctx, string(handle), client.ContainerStartOptions{})
	return err
}

// Wait blocks until the container reaches WaitConditionNotRunning (fully stopped),
// then inspects the container to extract start/finish timestamps. On context
// cancellation the container is force-removed so callers don't leak resources.
func (r *containerRuntime) Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error) {
	if r == nil || r.client == nil {
		return ContainerResult{}, errors.New("step: docker runtime not configured")
	}
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
		res := ContainerResult{ExitCode: int(status.StatusCode), ContainerID: string(handle)}
		inspect, err := r.client.ContainerInspect(ctx, string(handle), client.ContainerInspectOptions{})
		if err == nil {
			if data, marshalErr := json.Marshal(inspect.Container); marshalErr == nil {
				res.InspectJSON = data
			}
			if inspect.Container.State != nil {
				res.StartedAt = parseDockerTime(inspect.Container.State.StartedAt)
				res.CompletedAt = parseDockerTime(inspect.Container.State.FinishedAt)
			}
		}
		return res, nil
	}
	if ctx.Err() != nil {
		r.forceRemoveOnWaitCancel(handle)
	}
	return ContainerResult{}, errors.New("step: container wait interrupted")
}

func (r *containerRuntime) forceRemoveOnWaitCancel(handle ContainerHandle) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.client.ContainerRemove(cleanupCtx, string(handle), client.ContainerRemoveOptions{Force: true})
	if err != nil && !isContainerNotFound(err) {
		// Best-effort cleanup: cancellation must return promptly even when remove fails.
		return
	}
}

func isContainerNotFound(err error) bool {
	if err == nil {
		return false
	}
	return cerrdefs.IsNotFound(err)
}

// Logs returns combined stdout/stderr from the container.
//
// When TTY is disabled (default for workflow containers), Docker multiplexes
// stdout/stderr into a single stream with an 8-byte header per frame; we use
// moby's stdcopy.StdCopy to demux into separate buffers, then concatenate
// stdout+stderr for callers that expect plain text.
//
// On demux errors (corrupted stream, or TTY mode where the stream is not
// multiplexed) we fall back to ReadAll on the remaining bytes to avoid total
// data loss.
func (r *containerRuntime) Logs(ctx context.Context, handle ContainerHandle) ([]byte, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("step: docker runtime not configured")
	}
	reader, err := r.client.ContainerLogs(ctx, string(handle), client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("step: fetch container logs: %w", err)
	}
	defer func() { _ = reader.Close() }()

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, reader); err != nil {
		raw, readErr := io.ReadAll(reader)
		return raw, readErr
	}

	return append(stdoutBuf.Bytes(), stderrBuf.Bytes()...), nil
}

// StreamLogs follows container logs and writes demultiplexed stdout/stderr into
// the provided writers. This is used for live job log uploads while a container
// is still running.
func (r *containerRuntime) StreamLogs(ctx context.Context, handle ContainerHandle, stdout, stderr io.Writer) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	reader, err := r.client.ContainerLogs(ctx, string(handle), client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return fmt.Errorf("step: stream container logs: %w", err)
	}
	defer func() { _ = reader.Close() }()

	if _, err := stdcopy.StdCopy(stdout, stderr, reader); err != nil {
		return err
	}
	return nil
}

// Remove deletes the container with Force=true. Removing an already-removed
// container may return a 404 error; the operation is idempotent in effect.
func (r *containerRuntime) Remove(ctx context.Context, handle ContainerHandle) error {
	if r == nil || r.client == nil {
		return errors.New("step: docker runtime not configured")
	}
	_, err := r.client.ContainerRemove(ctx, string(handle), client.ContainerRemoveOptions{Force: true})
	return err
}

// ensureImageAvailable refreshes the image before container creation. Job images
// commonly use mutable tags such as latest, so local presence is not enough.
func (r *containerRuntime) ensureImageAvailable(ctx context.Context, imageRef string) error {
	return r.pullImage(ctx, imageRef)
}

func (r *containerRuntime) pullImage(ctx context.Context, imageRef string) error {
	err := r.pullImageOnce(ctx, imageRef)
	if err == nil {
		return nil
	}
	if !isDockerAuthPullError(err) {
		return fmt.Errorf("step: pull image %s: %w", imageRef, err)
	}
	if strings.TrimSpace(r.opts.RegistryAuthRefreshSocket) == "" {
		return fmt.Errorf("step: pull image %s: %w", imageRef, err)
	}
	if refreshErr := r.refreshRegistryAuth(ctx, imageRef); refreshErr != nil {
		return fmt.Errorf("step: refresh registry auth after pull image %s unauthorized: %w", imageRef, refreshErr)
	}
	if retryErr := r.pullImageOnce(ctx, imageRef); retryErr != nil {
		return fmt.Errorf("step: pull image %s after registry auth refresh: %w", imageRef, retryErr)
	}
	return nil
}

func (r *containerRuntime) pullImageOnce(ctx context.Context, imageRef string) error {
	registryAuth, err := r.registryAuthForImage(imageRef)
	if err != nil {
		return err
	}

	reader, err := r.images.ImagePull(ctx, imageRef, client.ImagePullOptions{
		RegistryAuth: registryAuth,
	})
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	// Drain the response to ensure the pull completes before returning.
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}
	if err := reader.Wait(ctx); err != nil {
		return err
	}
	return nil
}

func isDockerAuthPullError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "authentication required") ||
		strings.Contains(msg, "no basic auth credentials") ||
		strings.Contains(msg, "denied: requested access")
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
