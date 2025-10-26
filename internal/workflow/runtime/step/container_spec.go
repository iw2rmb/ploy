package step

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildContainerSpec assembles a ContainerSpec from the manifest and workspace.
func buildContainerSpec(manifest contracts.StepManifest, workspace Workspace) (ContainerSpec, error) {
	mounts := make([]ContainerMount, 0, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		path, ok := workspace.Inputs[input.Name]
		if !ok {
			return ContainerSpec{}, fmt.Errorf("step: workspace missing input %q", input.Name)
		}
		mounts = append(mounts, ContainerMount{
			Source:   path,
			Target:   input.MountPath,
			ReadOnly: input.Mode == contracts.StepInputModeReadOnly,
		})
	}
	command := append([]string{}, manifest.Command...)
	if len(manifest.Args) > 0 {
		command = append(command, manifest.Args...)
	}
	env := make(map[string]string, len(manifest.Env))
	if len(manifest.Env) > 0 {
		keys := make([]string, 0, len(manifest.Env))
		for key := range manifest.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			env[key] = manifest.Env[key]
		}
	}
	workingDir := manifest.WorkingDir
	if strings.TrimSpace(workingDir) == "" {
		workingDir = workspace.WorkingDir
	}
	return ContainerSpec{
		Image:      manifest.Image,
		Command:    command,
		WorkingDir: workingDir,
		Env:        env,
		Mounts:     mounts,
		Retain:     manifest.Retention.RetainContainer,
	}, nil
}

// ContainerRuntime executes containers for step runs.
type ContainerRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) (ContainerHandle, error)
	Start(ctx context.Context, handle ContainerHandle) error
	Wait(ctx context.Context, handle ContainerHandle) (ContainerResult, error)
	Logs(ctx context.Context, handle ContainerHandle) ([]byte, error)
}

// ContainerSpec describes a container execution request.
type ContainerSpec struct {
	Image      string
	Command    []string
	WorkingDir string
	Env        map[string]string
	Mounts     []ContainerMount
	Retain     bool
}

// ContainerMount describes a host path mount.
type ContainerMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerHandle identifies a prepared container.
type ContainerHandle struct {
	ID string
}

// ContainerResult captures container exit metadata.
type ContainerResult struct {
	ExitCode    int
	StartedAt   time.Time
	CompletedAt time.Time
}
