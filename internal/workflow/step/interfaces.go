package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ContainerRuntimeOptions holds configuration for Docker runtime.
type ContainerRuntimeOptions struct {
	// PullImage controls whether the runtime refreshes the image before container
	// creation.
	PullImage bool
	// Network is optional Docker network name (empty => default bridge).
	Network string
	// RegistryAuthConfigFile is a Docker auth config JSON file path
	// (DOCKER_AUTH_CONFIG format). When set, each image pull reads current
	// credentials from this file.
	RegistryAuthConfigFile string
	// RegistryAuthRefreshSocket is an optional Unix socket owned by the host.
	// When set, an auth failure on image pull asks the host to refresh registry
	// auth before retrying the same pull once.
	RegistryAuthRefreshSocket string
	// RegistryAuthConfigJSON is a Docker auth config JSON payload (DOCKER_AUTH_CONFIG
	// format). When set, image pulls use matching registry credentials.
	RegistryAuthConfigJSON string
}

// ContainerRuntime executes containers.
// Docker-socket specs require job-owned /tmp storage. Their socket proxy uses
// the spec's mount projection and lasts for one Create/Start/Wait execution.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	Start(ctx context.Context, handle ContainerHandle) error
	Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	Logs(ctx context.Context, handle ContainerHandle) ([]byte, error)
	Remove(ctx context.Context, handle ContainerHandle) error
}

// GateExecutor validates build artifacts.
// The primary implementation is gateExecutor (gate_docker.go) which runs
// validation containers locally via the container runtime.
type GateExecutor interface {
	Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string, mounts JobMounts) (*contracts.BuildGateStageMetadata, error)
}

// DiffGenerator generates diffs between states.
type DiffGenerator interface {
	Generate(ctx context.Context, workspace string) ([]byte, error)
}
