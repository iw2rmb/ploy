package step

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// FilesystemWorkspaceHydratorOptions configures the filesystem-backed workspace hydrator.
type FilesystemWorkspaceHydratorOptions struct {
	// ArtifactRoot defines where snapshot and diff tarballs are stored. When empty, the hydrator
	// resolves the root using XDG cache semantics.
	ArtifactRoot string
}

// FilesystemWorkspaceHydrator materialises step inputs by extracting snapshot and diff tarballs
// from the local artifact cache.
type FilesystemWorkspaceHydrator struct {
	artifactRoot string
}

// NewFilesystemWorkspaceHydrator constructs a workspace hydrator that reads artifacts from the filesystem.
func NewFilesystemWorkspaceHydrator(opts FilesystemWorkspaceHydratorOptions) (*FilesystemWorkspaceHydrator, error) {
	root := strings.TrimSpace(opts.ArtifactRoot)
	if root == "" {
		defaultRoot, err := defaultArtifactRoot()
		if err != nil {
			return nil, err
		}
		root = defaultRoot
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("step: ensure artifact root: %w", err)
	}
	return &FilesystemWorkspaceHydrator{artifactRoot: root}, nil
}

// Prepare hydrates the workspace for the provided manifest by extracting the referenced artifacts.
func (h *FilesystemWorkspaceHydrator) Prepare(ctx context.Context, req WorkspaceRequest) (Workspace, error) {
	if h == nil {
		return Workspace{}, errors.New("step: workspace hydrator not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := req.Manifest.Validate(); err != nil {
		return Workspace{}, fmt.Errorf("step: manifest invalid: %w", err)
	}
	root, err := os.MkdirTemp("", "ploy-step-workspace-*")
	if err != nil {
		return Workspace{}, fmt.Errorf("step: create workspace: %w", err)
	}

	inputPaths := make(map[string]string, len(req.Manifest.Inputs))
	for _, input := range req.Manifest.Inputs {
		select {
		case <-ctx.Done():
			return Workspace{}, ctx.Err()
		default:
		}
		targetDir := filepath.Join(root, sanitizeName(input.Name))
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return Workspace{}, fmt.Errorf("step: prepare input %s: %w", input.Name, err)
		}

		switch {
		case strings.TrimSpace(input.SnapshotCID) != "":
			if err := h.extractArtifact(ctx, snapshotArtifactPath(h.artifactRoot, input.SnapshotCID), targetDir); err != nil {
				return Workspace{}, fmt.Errorf("step: hydrate snapshot %s: %w", input.SnapshotCID, err)
			}
		case strings.TrimSpace(input.DiffCID) != "":
			if err := h.extractArtifact(ctx, diffArtifactPath(h.artifactRoot, input.DiffCID), targetDir); err != nil {
				return Workspace{}, fmt.Errorf("step: hydrate diff %s: %w", input.DiffCID, err)
			}
		default:
			return Workspace{}, fmt.Errorf("step: input %s missing snapshot or diff reference", input.Name)
		}
		inputPaths[input.Name] = targetDir
	}

	return Workspace{
		Inputs:     inputPaths,
		WorkingDir: resolveDefaultWorkingDir(req.Manifest),
	}, nil
}

func (h *FilesystemWorkspaceHydrator) extractArtifact(ctx context.Context, artifactPath string, dest string) error {
	info, err := os.Stat(artifactPath)
	if err != nil {
		return fmt.Errorf("resolve artifact: %w", err)
	}
	if info.IsDir() {
		return copyDirectory(ctx, artifactPath, dest)
	}
	return untarFile(ctx, artifactPath, dest)
}

func snapshotArtifactPath(root, cid string) string {
	return filepath.Join(root, "snapshots", sanitizeName(cid))
}

func diffArtifactPath(root, cid string) string {
	return filepath.Join(root, "diffs", sanitizeName(cid))
}

func resolveDefaultWorkingDir(manifest contracts.StepManifest) string {
	for _, input := range manifest.Inputs {
		if input.Mode == contracts.StepInputModeReadWrite {
			return input.MountPath
		}
	}
	return "/"
}

func sanitizeName(value string) string {
	if value == "" {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	builder := strings.Builder{}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	return builder.String()
}

func untarFile(ctx context.Context, path string, dest string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	var reader io.Reader = file
	if isCompressed(path) {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("decompress %s: %w", path, err)
		}
		defer func() {
			_ = gzReader.Close()
		}()
		reader = gzReader
	}

	tr := tar.NewReader(reader)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar %s: %w", path, err)
		}

		target := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %s escapes destination", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				closeErr := out.Close()
				if closeErr != nil {
					err = errors.Join(err, closeErr)
				}
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// Ignore other entry types for now.
		}
	}
	return nil
}

func copyDirectory(ctx context.Context, src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func isCompressed(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".tgz")
}

func defaultArtifactRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("PLOY_ARTIFACT_ROOT")); root != "" {
		return root, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("step: resolve artifact root: %w", err)
	}
	return filepath.Join(base, "ploy", "artifacts"), nil
}
