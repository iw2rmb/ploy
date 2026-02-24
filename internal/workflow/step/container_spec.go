package step

import (
	"time"

	"os"
	"strings"

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
	Retain     bool
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
}

// buildContainerSpec assembles a ContainerSpec from the manifest and workspace path.
// The runID parameter threads the workflow run identifier into container labels
// for correlation with telemetry and log aggregation systems.
func buildContainerSpec(runID types.RunID, manifest contracts.StepManifest, workspace string, outDir string, inDir string) (ContainerSpec, error) {
	// Mount the first input at its mount path; fallback to working dir.
	mounts := make([]ContainerMount, 0, len(manifest.Inputs))
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
	// Optional /out mount for additional artifacts
	if strings.TrimSpace(outDir) != "" {
		mounts = append(mounts, ContainerMount{Source: outDir, Target: "/out", ReadOnly: false})
	}
	// Optional /in mount for cross-phase inputs (read-only)
	if strings.TrimSpace(inDir) != "" {
		mounts = append(mounts, ContainerMount{Source: inDir, Target: "/in", ReadOnly: true})
	}

	// Optional: mount host Docker socket for containers that request it via manifest options
	if mountDockerSocket, ok := manifest.OptionBool("mount_docker_socket"); ok && mountDockerSocket {
		const sock = "/var/run/docker.sock"
		if fi, err := os.Stat(sock); err == nil && !fi.IsDir() {
			mounts = append(mounts, ContainerMount{Source: sock, Target: sock, ReadOnly: false})
		}
	}

	// Optional: mount TLS certificates for control-plane API access from containers
	if caCertPath, ok := manifest.OptionString("ploy_ca_cert_path"); ok && caCertPath != "" {
		if fi, err := os.Stat(caCertPath); err == nil && !fi.IsDir() {
			mounts = append(mounts, ContainerMount{Source: caCertPath, Target: "/etc/ploy/certs/ca.crt", ReadOnly: true})
		}
	}
	if clientCertPath, ok := manifest.OptionString("ploy_client_cert_path"); ok && clientCertPath != "" {
		if fi, err := os.Stat(clientCertPath); err == nil && !fi.IsDir() {
			mounts = append(mounts, ContainerMount{Source: clientCertPath, Target: "/etc/ploy/certs/client.crt", ReadOnly: true})
		}
	}
	if clientKeyPath, ok := manifest.OptionString("ploy_client_key_path"); ok && clientKeyPath != "" {
		if fi, err := os.Stat(clientKeyPath); err == nil && !fi.IsDir() {
			mounts = append(mounts, ContainerMount{Source: clientKeyPath, Target: "/etc/ploy/certs/client.key", ReadOnly: true})
		}
	}
	wd := manifest.WorkingDir
	if wd == "" && len(manifest.Inputs) > 0 {
		wd = manifest.Inputs[0].MountPath
	}
	// Prepare labels: thread run identifier when provided.
	// Labels enable container correlation with telemetry and log aggregation.
	var labels map[string]string
	if !runID.IsZero() {
		labels = map[string]string{types.LabelRunID: runID.String()}
	}

	// Convert resource hints to runtime limits.
	nanoCPUs, memBytes, diskBytes, storageSizeOpt := manifest.Resources.ToLimits()

	return ContainerSpec{
		Image:            manifest.Image,
		Command:          append([]string{}, manifest.Command...),
		WorkingDir:       wd,
		Env:              manifest.Env,
		Mounts:           mounts,
		Retain:           manifest.Retention.RetainContainer,
		Labels:           labels,
		LimitNanoCPUs:    nanoCPUs,
		LimitMemoryBytes: memBytes,
		LimitDiskBytes:   diskBytes,
		StorageSizeOpt:   storageSizeOpt,
	}, nil
}
