package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// WorkspaceHydrator prepares a workspace for execution.
type WorkspaceHydrator interface {
	Hydrate(ctx context.Context, manifest contracts.StepManifest, workspace string) error
}

// DockerContainerRuntimeOptions holds configuration for Docker runtime.
type DockerContainerRuntimeOptions struct {
	// PullImage controls whether the runtime ensures the image is available
	// (by pulling it only when missing) before container creation.
	PullImage bool
	// Network is optional Docker network name (empty => default bridge).
	Network string
}

// ContainerRuntime executes containers.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	Start(ctx context.Context, handle ContainerHandle) error
	Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	Logs(ctx context.Context, handle ContainerHandle) ([]byte, error)
	Remove(ctx context.Context, handle ContainerHandle) error
}

// GateExecutor validates build artifacts.
// The primary implementation is dockerGateExecutor (gate_docker.go) which runs
// validation containers locally via the container runtime.
type GateExecutor interface {
	Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}

// DiffGenerator generates diffs between states.
type DiffGenerator interface {
	Generate(ctx context.Context, workspace string) ([]byte, error)
	// GenerateBetween computes a diff between two directories (base and modified).
	// Used by C2 to capture pre-mig healing changes (base clone → healed workspace).
	GenerateBetween(ctx context.Context, baseDir, modifiedDir string) ([]byte, error)
}
