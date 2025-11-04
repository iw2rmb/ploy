package step

import (
	"time"

	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

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

// buildContainerSpec assembles a ContainerSpec from the manifest and workspace path.
func buildContainerSpec(manifest contracts.StepManifest, workspace string, outDir string) (ContainerSpec, error) {
	// Mount the first RW input at its mount path; fallback to working dir.
	mounts := make([]ContainerMount, 0, len(manifest.Inputs))
	// Always mount the hydrated workspace to the declared RW mount (first RW input)
	if len(manifest.Inputs) > 0 {
		in := manifest.Inputs[0]
		mounts = append(mounts, ContainerMount{Source: workspace, Target: in.MountPath, ReadOnly: false})
	} else {
		mounts = append(mounts, ContainerMount{Source: workspace, Target: "/workspace", ReadOnly: false})
	}
	// Optional /out mount for additional artifacts
	if strings.TrimSpace(outDir) != "" {
		mounts = append(mounts, ContainerMount{Source: outDir, Target: "/out", ReadOnly: false})
	}
	wd := manifest.WorkingDir
	if wd == "" && len(manifest.Inputs) > 0 {
		wd = manifest.Inputs[0].MountPath
	}
	return ContainerSpec{
		Image:      manifest.Image,
		Command:    append([]string{}, manifest.Command...),
		WorkingDir: wd,
		Env:        manifest.Env,
		Mounts:     mounts,
		Retain:     manifest.Retention.RetainContainer,
	}, nil
}
