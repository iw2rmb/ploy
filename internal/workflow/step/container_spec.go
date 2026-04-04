package step

import (
	"fmt"
	"io"
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
// stagingDir is an optional path to a staging directory for materialized Hydra
// resources; each In/Out/Home/CA entry is mounted from stagingDir/<shortHash>.
func buildContainerSpec(runID types.RunID, jobID types.JobID, manifest contracts.StepManifest, workspace string, outDir string, inDir string, stagingDir string) (ContainerSpec, error) {
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
	// Optional /in mount for cross-phase inputs.
	// Keep the top-level mount writable so nested Hydra file mounts under /in/*
	// can be created by the container runtime. Individual Hydra /in entries remain
	// read-only mounts.
	if strings.TrimSpace(inDir) != "" {
		mounts = append(mounts, ContainerMount{Source: inDir, Target: "/in", ReadOnly: false})
	}

	// Mount Hydra materialized resources from the staging directory.
	// Each entry references a shortHash; staged content lives at stagingDir/<shortHash>.
	if strings.TrimSpace(stagingDir) != "" {
		// CA entries: mount at deterministic CA cert path (read-only).
		// Archives are rooted under "content" (see buildSourceArchive), so
		// the mount source must include the content subdirectory.
		for _, entry := range manifest.CA {
			hash, err := contracts.ParseStoredCAEntry(entry)
			if err != nil {
				return ContainerSpec{}, fmt.Errorf("ca entry %q: %w", entry, err)
			}
			mounts = append(mounts, ContainerMount{
				Source:   filepath.Join(stagingDir, hash, "content"),
				Target:   "/etc/ploy/ca/" + hash,
				ReadOnly: true,
			})
		}
		// In entries: mount read-only at the declared destination.
		for _, entry := range manifest.In {
			parsed, err := contracts.ParseStoredInEntry(entry)
			if err != nil {
				return ContainerSpec{}, fmt.Errorf("in entry %q: %w", entry, err)
			}
			mounts = append(mounts, ContainerMount{
				Source:   filepath.Join(stagingDir, parsed.Hash, "content"),
				Target:   parsed.Dst,
				ReadOnly: true,
			})
		}
		// Out entries: validate destinations and enforce outDir presence.
		// Out entries are seeded into outDir by SeedOutDirFromStaging and
		// covered by the single /out mount — no separate mounts are created.
		for _, entry := range manifest.Out {
			parsed, err := contracts.ParseStoredOutEntry(entry)
			if err != nil {
				return ContainerSpec{}, fmt.Errorf("out entry %q: %w", entry, err)
			}
			if strings.TrimSpace(outDir) == "" {
				return ContainerSpec{}, fmt.Errorf("out entry %q: outDir required for destination %s", entry, parsed.Dst)
			}
		}
		// Home entries: mount at $HOME/<dst> with mode from entry.
		// Resolve HOME from manifest envs; fall back to /home/user.
		homeDir := "/home/user"
		if h := manifest.Envs["HOME"]; h != "" {
			homeDir = h
		}
		for _, entry := range manifest.Home {
			parsed, err := contracts.ParseStoredHomeEntry(entry)
			if err != nil {
				return ContainerSpec{}, fmt.Errorf("home entry %q: %w", entry, err)
			}
			mounts = append(mounts, ContainerMount{
				Source:   filepath.Join(stagingDir, parsed.Hash, "content"),
				Target:   homeDir + "/" + parsed.Dst,
				ReadOnly: parsed.ReadOnly,
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
		Env:              manifest.Envs,
		Mounts:           mounts,
		Labels:           labels,
		LimitNanoCPUs:    nanoCPUs,
		LimitMemoryBytes: memBytes,
		LimitDiskBytes:   diskBytes,
		StorageSizeOpt:   storageSizeOpt,
	}, nil
}

// SeedOutDirFromStaging copies materialized Hydra out entry content from the
// staging directory into outDir so that the single /out mount covers both
// pre-seeded content and container writes. This ensures uploadOutDirBundle
// archives all out content including Hydra-originated entries.
func SeedOutDirFromStaging(manifest contracts.StepManifest, stagingDir, outDir string) error {
	if stagingDir == "" || outDir == "" {
		return nil
	}
	cleanOutDir := filepath.Clean(outDir)
	for _, entry := range manifest.Out {
		parsed, err := contracts.ParseStoredOutEntry(entry)
		if err != nil {
			return fmt.Errorf("out entry %q: %w", entry, err)
		}
		rel := strings.TrimPrefix(parsed.Dst, "/out/")
		src := filepath.Join(stagingDir, parsed.Hash, "content")
		dst := filepath.Clean(filepath.Join(outDir, rel))
		if dst != cleanOutDir && !strings.HasPrefix(dst, cleanOutDir+string(filepath.Separator)) {
			return fmt.Errorf("out entry %q: resolved path %s escapes outDir", entry, dst)
		}
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("seed out %s: %w", parsed.Dst, err)
		}
	}
	return nil
}

// copyPath copies src to dst. If src is a directory, it copies recursively.
// If src is a file, it copies the file preserving permissions.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, de := range entries {
		s := filepath.Join(src, de.Name())
		d := filepath.Join(dst, de.Name())
		if de.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else {
			info, err := de.Info()
			if err != nil {
				return err
			}
			if err := copyFile(s, d, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(df, sf); err != nil {
		df.Close()
		return err
	}
	return df.Close()
}
