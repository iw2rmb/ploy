package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

func (r *DockerContainerRuntime) shouldDelegateAuthPull(imageRef string, pullErr error) bool {
	if r == nil || r.delegatedPull == nil || pullErr == nil {
		return false
	}
	configuredHost := normalizeAuthRegistryKey(r.opts.DelegatedAuthPullRegistry)
	if configuredHost == "" || imageRegistryHost(imageRef) != configuredHost {
		return false
	}
	return isAuthPullError(pullErr)
}

func (r *DockerContainerRuntime) delegateAuthPull(ctx context.Context, imageRef string, pullErr error) error {
	configuredHost := normalizeAuthRegistryKey(r.opts.DelegatedAuthPullRegistry)
	slog.Warn("docker image pull auth fallback attempted",
		"image", imageRef,
		"registry_host", configuredHost,
		"error", pullErr,
	)

	updaterID, err := r.findNodeUpdaterContainer(ctx)
	if err != nil {
		slog.Warn("docker image pull auth fallback failed",
			"image", imageRef,
			"registry_host", configuredHost,
			"error", err,
		)
		return fmt.Errorf("step: pull image %s: %w; delegated pull fallback failed: %v", imageRef, pullErr, err)
	}
	slog.Info("docker image pull auth fallback updater found",
		"image", imageRef,
		"registry_host", configuredHost,
		"updater_container_id", updaterID,
	)

	output, exitCode, err := r.execUpdaterDockerPull(ctx, updaterID, imageRef)
	if err != nil {
		slog.Warn("docker image pull auth fallback failed",
			"image", imageRef,
			"registry_host", configuredHost,
			"updater_container_id", updaterID,
			"exit_code", exitCode,
			"error", err,
		)
		return fmt.Errorf("step: pull image %s: %w; delegated pull fallback failed: %v", imageRef, pullErr, err)
	}
	if exitCode != 0 {
		err := fmt.Errorf("updater docker pull exited with code %d: %s", exitCode, strings.TrimSpace(output))
		slog.Warn("docker image pull auth fallback failed",
			"image", imageRef,
			"registry_host", configuredHost,
			"updater_container_id", updaterID,
			"exit_code", exitCode,
			"error", err,
		)
		return fmt.Errorf("step: pull image %s: %w; delegated pull fallback failed: %v", imageRef, pullErr, err)
	}
	if _, err := r.images.ImageInspect(ctx, imageRef); err != nil {
		err = fmt.Errorf("re-inspect delegated image %s: %w", imageRef, err)
		slog.Warn("docker image pull auth fallback failed",
			"image", imageRef,
			"registry_host", configuredHost,
			"updater_container_id", updaterID,
			"exit_code", exitCode,
			"error", err,
		)
		return fmt.Errorf("step: pull image %s: %w; delegated pull fallback failed: %v", imageRef, pullErr, err)
	}
	slog.Info("docker image pull auth fallback succeeded",
		"image", imageRef,
		"registry_host", configuredHost,
		"updater_container_id", updaterID,
	)
	return nil
}

func (r *DockerContainerRuntime) findNodeUpdaterContainer(ctx context.Context) (string, error) {
	list, err := r.delegatedPull.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("label", "com.docker.compose.service=node-updater"),
	})
	if err != nil {
		return "", fmt.Errorf("list node-updater containers by service label: %w", err)
	}
	if id := firstContainerID(list); id != "" {
		return id, nil
	}

	list, err = r.delegatedPull.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("name", "ploy-node-updater-1"),
	})
	if err != nil {
		return "", fmt.Errorf("list node-updater containers by name: %w", err)
	}
	if id := firstContainerID(list); id != "" {
		return id, nil
	}
	return "", errors.New("find node-updater container: no running container found")
}

func firstContainerID(list client.ContainerListResult) string {
	for _, item := range list.Items {
		if strings.TrimSpace(item.ID) != "" {
			return item.ID
		}
	}
	return ""
}

func (r *DockerContainerRuntime) execUpdaterDockerPull(ctx context.Context, updaterID, imageRef string) (string, int, error) {
	const shellScript = `dp auth service-acc --key-file "${PLOY_DP_SERVICE_ACCOUNT_KEY_FILE:-/etc/ploy/dp.sa.json}" >/dev/null && docker pull "$1"`
	create, err := r.delegatedPull.ExecCreate(ctx, updaterID, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd: []string{
			"/usr/bin/bash",
			"-lc",
			shellScript,
			"ploy-node-auth-pull",
			imageRef,
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("create updater exec: %w", err)
	}
	if strings.TrimSpace(create.ID) == "" {
		return "", 0, errors.New("create updater exec: empty exec id")
	}

	attached, err := r.delegatedPull.ExecAttach(ctx, create.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", 0, fmt.Errorf("attach updater exec: %w", err)
	}
	if attached.Conn != nil {
		defer attached.Close()
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attached.Reader); err != nil {
		raw, readErr := io.ReadAll(attached.Reader)
		if readErr != nil {
			return "", 0, fmt.Errorf("read updater exec output: %w", readErr)
		}
		stdoutBuf.Write(raw)
	}
	output := strings.TrimSpace(stdoutBuf.String() + stderrBuf.String())

	inspect, err := r.delegatedPull.ExecInspect(ctx, create.ID, client.ExecInspectOptions{})
	if err != nil {
		return output, 0, fmt.Errorf("inspect updater exec: %w", err)
	}
	if inspect.Running {
		return output, 0, errors.New("inspect updater exec: still running after attach completed")
	}
	return output, inspect.ExitCode, nil
}

func isAuthPullError(err error) bool {
	text := strings.ToLower(err.Error())
	for _, needle := range []string{
		"unauthorized",
		"authorization header required",
		"authentication required",
		"no basic auth credentials",
		"denied requested access",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
