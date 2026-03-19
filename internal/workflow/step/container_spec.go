package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type certMountOption struct {
	key      string
	target   string
	readOnly bool
}

var certMountOptions = []certMountOption{
	{key: "ploy_ca_cert_path", target: "/etc/ploy/certs/ca.crt", readOnly: true},
	{key: "ploy_client_cert_path", target: "/etc/ploy/certs/client.crt", readOnly: true},
	{key: "ploy_client_key_path", target: "/etc/ploy/certs/client.key", readOnly: true},
}

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
}

// buildContainerSpec assembles a ContainerSpec from the manifest and workspace path.
// The runID and jobID parameters thread workflow identifiers into container labels
// for correlation with telemetry and log aggregation systems.
// tmpStagingDir is an optional path to a directory containing pre-materialized tmp
// files; each manifest.TmpDir entry is mounted read-only at /tmp/<name>.
func buildContainerSpec(runID types.RunID, jobID types.JobID, manifest contracts.StepManifest, workspace string, outDir string, inDir string, tmpStagingDir string) (ContainerSpec, error) {
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

	// Mount each tmp file read-only at /tmp/<name> from the staging directory.
	// Runtime hardening: reject malformed names or canonical duplicates to keep
	// staging and mount path derivation deterministic.
	if strings.TrimSpace(tmpStagingDir) != "" {
		seenTmpNames := make(map[string]struct{}, len(manifest.TmpDir))
		for _, tf := range manifest.TmpDir {
			name, err := contracts.NormalizeTmpFileName(tf.Name)
			if err != nil {
				return ContainerSpec{}, fmt.Errorf("tmp file name %q is not valid: %w", tf.Name, err)
			}
			if _, dup := seenTmpNames[name]; dup {
				return ContainerSpec{}, fmt.Errorf("tmp file name duplicate %q", name)
			}
			seenTmpNames[name] = struct{}{}
			mounts = append(mounts, ContainerMount{
				Source:   filepath.Join(tmpStagingDir, name),
				Target:   "/tmp/" + name,
				ReadOnly: true,
			})
		}
	}

	// Optional: mount host Docker socket for containers that request it via manifest options
	if mountDockerSocket, ok := manifest.OptionBool("mount_docker_socket"); ok && mountDockerSocket {
		const sock = "/var/run/docker.sock"
		if fi, err := os.Stat(sock); err == nil && !fi.IsDir() {
			mounts = append(mounts, ContainerMount{Source: sock, Target: sock, ReadOnly: false})
		}
	}

	// Optional: mount TLS certificates for control-plane API access from containers.
	for _, opt := range certMountOptions {
		certPath, ok := manifest.OptionString(opt.key)
		if !ok || certPath == "" {
			continue
		}
		if fi, err := os.Stat(certPath); err == nil && !fi.IsDir() {
			mounts = append(mounts, ContainerMount{
				Source:   certPath,
				Target:   opt.target,
				ReadOnly: opt.readOnly,
			})
		}
	}
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
		Env:              manifest.Env,
		Mounts:           mounts,
		Labels:           labels,
		LimitNanoCPUs:    nanoCPUs,
		LimitMemoryBytes: memBytes,
		LimitDiskBytes:   diskBytes,
		StorageSizeOpt:   storageSizeOpt,
	}, nil
}
