package step

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const registryAuthRefreshCommand = "/usr/local/lib/ploy/ploy-node-auth-refresh"

func (r *DockerContainerRuntime) refreshRegistryAuthForPull(ctx context.Context, imageRef string) error {
	if r == nil || r.exec == nil {
		return errors.New("docker exec API not configured")
	}
	container, err := r.resolveRegistryAuthRefreshContainer(ctx)
	if err != nil {
		return err
	}

	created, err := r.exec.ExecCreate(ctx, container, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{registryAuthRefreshCommand, "refresh-for-pull", imageRef},
	})
	if err != nil {
		return fmt.Errorf("create auth refresh exec: %w", err)
	}

	output, attachErr := r.runAttachedExec(ctx, created.ID)
	inspect, inspectErr := r.exec.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if attachErr != nil {
		return fmt.Errorf("run auth refresh exec: %w", attachErr)
	}
	if inspectErr != nil {
		return fmt.Errorf("inspect auth refresh exec: %w", inspectErr)
	}
	if inspect.ExitCode != 0 {
		return fmt.Errorf("auth refresh exec exited %d: %s", inspect.ExitCode, strings.TrimSpace(output))
	}
	return nil
}

func (r *DockerContainerRuntime) resolveRegistryAuthRefreshContainer(ctx context.Context) (string, error) {
	want := strings.TrimSpace(r.opts.RegistryAuthRefreshContainer)
	if want == "" {
		return "", errors.New("registry auth refresh container not configured")
	}
	listed, err := r.exec.ContainerList(ctx, client.ContainerListOptions{})
	if err != nil {
		return "", fmt.Errorf("list auth refresh containers: %w", err)
	}
	for _, item := range listed.Items {
		if containerMatches(item, want) {
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("auth refresh container %q not found", want)
}

func containerMatches(item containertypes.Summary, want string) bool {
	if item.ID == want || strings.HasPrefix(item.ID, want) {
		return true
	}
	for _, name := range item.Names {
		trimmed := strings.TrimPrefix(strings.TrimSpace(name), "/")
		if trimmed == want {
			return true
		}
	}
	return false
}

func (r *DockerContainerRuntime) runAttachedExec(ctx context.Context, execID string) (string, error) {
	attached, err := r.exec.ExecAttach(ctx, execID, client.ExecAttachOptions{})
	if err != nil {
		return "", err
	}
	if attached.Conn != nil {
		defer attached.Close()
	}

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attached.Reader); err != nil {
		var raw bytes.Buffer
		_, _ = io.Copy(&raw, attached.Reader)
		return raw.String(), err
	}
	return stdout.String() + stderr.String(), nil
}
