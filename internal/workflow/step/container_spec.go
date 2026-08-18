package step

import (
	"fmt"
	"io"
	"os"
	"path"
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

	// In, out, and tmp entries are seeded into their common parent mounts.
	for _, entry := range manifest.In {
		if _, err := contracts.ParseStoredInEntry(entry); err != nil {
			return ContainerSpec{}, fmt.Errorf("in entry %q: %w", entry, err)
		}
	}
	for _, entry := range manifest.Out {
		if _, err := contracts.ParseStoredOutEntry(entry); err != nil {
			return ContainerSpec{}, fmt.Errorf("out entry %q: %w", entry, err)
		}
	}
	for _, entry := range manifest.Tmp {
		if _, err := contracts.ParseStoredTmpEntry(entry); err != nil {
			return ContainerSpec{}, fmt.Errorf("tmp entry %q: %w", entry, err)
		}
	}

	nested := make([]nestedMountContract, 0, len(manifest.Home))
	// Writable Hydra home content is copied into the job home before container
	// creation. Read-only content remains a nested mount so its mode is retained.
	for _, entry := range manifest.Home {
		parsed, err := contracts.ParseStoredHomeEntry(entry)
		if err != nil {
			return ContainerSpec{}, fmt.Errorf("home entry %q: %w", entry, err)
		}
		if !parsed.ReadOnly {
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
	if err := validateContainerMounts(mounts, nested); err != nil {
		return ContainerSpec{}, fmt.Errorf("validate container mounts: %w", err)
	}
	env, err := applyReservedJobEnv(manifest.Envs, jobMounts)
	if err != nil {
		return ContainerSpec{}, fmt.Errorf("prepare job environment: %w", err)
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
		Env:              env,
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
// pre-seeded content and container writes.
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

// SeedInDirFromStaging copies materialized Hydra in entry content from the
// staging directory into inDir before the container receives the directory as
// one read-only /in mount.
func SeedInDirFromStaging(manifest contracts.StepManifest, stagingDir, inDir string) error {
	if stagingDir == "" || inDir == "" {
		return nil
	}
	cleanInDir := filepath.Clean(inDir)
	for _, entry := range manifest.In {
		parsed, err := contracts.ParseStoredInEntry(entry)
		if err != nil {
			return fmt.Errorf("in entry %q: %w", entry, err)
		}
		rel := strings.TrimPrefix(parsed.Dst, "/in/")
		src := filepath.Join(stagingDir, parsed.Hash, "content")
		dst := filepath.Clean(filepath.Join(inDir, rel))
		if dst != cleanInDir && !strings.HasPrefix(dst, cleanInDir+string(filepath.Separator)) {
			return fmt.Errorf("in entry %q: resolved path %s escapes inDir", entry, dst)
		}
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("seed in %s: %w", parsed.Dst, err)
		}
	}
	return nil
}

// SeedTmpDirFromStaging copies materialized Hydra tmp entry content from the
// staging directory into tmpDir so that the single /tmp mount exposes all tmp
// files while keeping them outside durable repo artifacts.
func SeedTmpDirFromStaging(manifest contracts.StepManifest, stagingDir, tmpDir string) error {
	if stagingDir == "" || tmpDir == "" {
		return nil
	}
	cleanTmpDir := filepath.Clean(tmpDir)
	for _, entry := range manifest.Tmp {
		parsed, err := contracts.ParseStoredTmpEntry(entry)
		if err != nil {
			return fmt.Errorf("tmp entry %q: %w", entry, err)
		}
		rel := strings.TrimPrefix(parsed.Dst, "/tmp/")
		src := filepath.Join(stagingDir, parsed.Hash, "content")
		dst := filepath.Clean(filepath.Join(tmpDir, rel))
		if dst != cleanTmpDir && !strings.HasPrefix(dst, cleanTmpDir+string(filepath.Separator)) {
			return fmt.Errorf("tmp entry %q: resolved path %s escapes tmpDir", entry, dst)
		}
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("seed tmp %s: %w", parsed.Dst, err)
		}
	}
	return nil
}

// SeedHomeDirFromStaging copies writable Hydra home entries into the job-owned
// home. Read-only entries remain bind mounts so the container cannot mutate
// their materialized sources.
func SeedHomeDirFromStaging(manifest contracts.StepManifest, stagingDir, homeDir string) error {
	if stagingDir == "" || homeDir == "" {
		return nil
	}
	cleanHomeDir := filepath.Clean(homeDir)
	for _, entry := range manifest.Home {
		parsed, err := contracts.ParseStoredHomeEntry(entry)
		if err != nil {
			return fmt.Errorf("home entry %q: %w", entry, err)
		}
		if parsed.ReadOnly {
			continue
		}
		src := filepath.Join(stagingDir, parsed.Hash, "content")
		dst := filepath.Clean(filepath.Join(homeDir, filepath.FromSlash(parsed.Dst)))
		if dst != cleanHomeDir && !strings.HasPrefix(dst, cleanHomeDir+string(filepath.Separator)) {
			return fmt.Errorf("home entry %q: resolved path %s escapes homeDir", entry, dst)
		}
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("seed home %s: %w", parsed.Dst, err)
		}
	}
	return nil
}

// copyPath copies src to dst. If src is a directory, it copies recursively.
// If src is a file, it copies the file preserving permissions and timestamps.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode().Perm(), info.ModTime())
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
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
			if err := copyFile(s, d, info.Mode().Perm(), info.ModTime()); err != nil {
				return err
			}
		}
	}
	if err := os.Chmod(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode, modTime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
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
		if closeErr := df.Close(); closeErr != nil {
			return fmt.Errorf("copy file close error: %w (copy error: %v)", closeErr, err)
		}
		return err
	}
	if err := df.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, perm); err != nil {
		return err
	}
	return os.Chtimes(dst, modTime, modTime)
}
