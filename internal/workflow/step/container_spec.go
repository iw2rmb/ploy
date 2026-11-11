package step

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ContainerSpec describes a container execution request.
type ContainerSpec struct {
	Image      string
	Command    []string
	WorkingDir string
	Env        map[string]string
	Mounts     []ContainerMount
	Labels     map[string]string
	// Optional resource limits (0 => unlimited)
	LimitNanoCPUs    int64
	LimitMemoryBytes int64
	// Optional disk limit for writable layer (bytes; 0 => unlimited).
	// When set, Docker runtime may pass a storage option (driver dependent).
	LimitDiskBytes int64
	// Optional raw storage size option string passed to Docker (e.g., "10G").
	// Set only when the operator provided PLOY_BUILDGATE_LIMIT_DISK_SPACE.
	StorageSizeOpt string
}

// ContainerMount describes a host path mount.
type ContainerMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerHandle identifies a prepared container by its ID.
type ContainerHandle string

// ContainerResult captures container exit metadata.
type ContainerResult struct {
	ExitCode    int
	StartedAt   time.Time
	CompletedAt time.Time
	ContainerID string
	InspectJSON []byte
}

// buildContainerSpec assembles a ContainerSpec from the manifest and workspace path.
// The runID and jobID parameters thread workflow identifiers into container labels
// for correlation with telemetry and log aggregation systems.
func buildContainerSpec(runID types.RunID, jobID types.JobID, manifest contracts.StepManifest, workspace string, jobMounts JobMounts) (ContainerSpec, error) {
	// Mount the first input at its mount path; fallback to working dir.
	mounts := make([]ContainerMount, 0, len(manifest.Inputs)+9)
	// Always mount the hydrated workspace to the declared mount (first input), respecting mode.
	if len(manifest.Inputs) > 0 {
		in := manifest.Inputs[0]
		mounts = append(mounts, ContainerMount{
			Source:   workspace,
			Target:   in.MountPath,
			ReadOnly: in.Mode == contracts.StepInputModeReadOnly,
		})
	} else {
		mounts = append(mounts, ContainerMount{Source: workspace, Target: "/workspace", ReadOnly: false})
	}
	homeDir, err := resolveJobHome(manifest.Envs)
	if err != nil {
		return ContainerSpec{}, fmt.Errorf("resolve job home: %w", err)
	}
	commonMounts, err := buildCommonJobMounts(jobMounts, homeDir)
	if err != nil {
		return ContainerSpec{}, fmt.Errorf("prepare common job mounts: %w", err)
	}
	mounts = append(mounts, commonMounts...)

	nested := make([]nestedMountContract, 0, len(manifest.Home))
	for _, kind := range contracts.HydraFileKinds() {
		for _, entry := range kind.Entries(manifest) {
			parsed, err := contracts.ParseStoredEntry(kind, entry)
			if err != nil {
				return ContainerSpec{}, fmt.Errorf("%s entry %q: %w", kind, entry, err)
			}
			// Writable Hydra home content is copied into the job home before
			// creation. Read-only content remains a nested mount to retain its mode.
			if kind != contracts.HydraFileHome || !parsed.ReadOnly {
				continue
			}
			target := path.Join(homeDir, parsed.Dst)
			mounts = append(mounts, ContainerMount{
				Source:   filepath.Join(jobMounts.Staging, parsed.Hash, "content"),
				Target:   target,
				ReadOnly: true,
			})
			nested = append(nested, nestedMountContract{parent: homeDir, child: target})
		}
	}

	// Optional: mount host Docker socket for containers that request it via manifest options
	if mountDockerSocket, ok := manifest.OptionBool("mount_docker_socket"); ok && mountDockerSocket {
		mounts = appendHostSocketMount(mounts, "/var/run/docker.sock")
	}

	if err := validateContainerMounts(mounts, nested); err != nil {
		return ContainerSpec{}, fmt.Errorf("validate container mounts: %w", err)
	}
	env := applyReservedJobEnv(manifest.Envs, jobMounts, homeDir)
	wd := manifest.WorkingDir
	if wd == "" && len(manifest.Inputs) > 0 {
		wd = manifest.Inputs[0].MountPath
	}
	// Prepare labels: thread run and job identifiers when provided.
	// Labels enable container correlation with telemetry and log aggregation.
	var labels map[string]string
	if !runID.IsZero() {
		labels = map[string]string{types.LabelRunID: runID.String()}
	}
	if !jobID.IsZero() {
		if labels == nil {
			labels = make(map[string]string, 1)
		}
		labels[types.LabelJobID] = jobID.String()
	}

	// Convert resource hints to runtime limits.
	nanoCPUs, memBytes, diskBytes, storageSizeOpt := manifest.Resources.ToLimits()

	return ContainerSpec{
		Image:            manifest.Image,
		Command:          append([]string{}, manifest.Command...),
		WorkingDir:       wd,
		Env:              env,
		Mounts:           mounts,
		Labels:           labels,
		LimitNanoCPUs:    nanoCPUs,
		LimitMemoryBytes: memBytes,
		LimitDiskBytes:   diskBytes,
		StorageSizeOpt:   storageSizeOpt,
	}, nil
}

// appendHostSocketMount bind-mounts an existing host socket at the same path.
// It skips an empty, missing, or directory path.
func appendHostSocketMount(mounts []ContainerMount, socketPath string) []ContainerMount {
	if socketPath == "" {
		return mounts
	}
	info, err := os.Stat(socketPath)
	if err != nil || info.IsDir() {
		return mounts
	}
	return append(mounts, ContainerMount{Source: socketPath, Target: socketPath, ReadOnly: false})
}
