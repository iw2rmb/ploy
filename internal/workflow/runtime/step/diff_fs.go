package step

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// FilesystemDiffGeneratorOptions configures the filesystem diff generator.
type FilesystemDiffGeneratorOptions struct {
	// TempDir optionally overrides where diff tarballs are written.
	TempDir string
}

// FilesystemDiffGenerator captures workspace diffs by archiving the read-write mount into a tarball.
type FilesystemDiffGenerator struct {
	tempDir string
}

// NewFilesystemDiffGenerator constructs a diff generator writing tarballs to the filesystem.
func NewFilesystemDiffGenerator(opts FilesystemDiffGeneratorOptions) *FilesystemDiffGenerator {
	tempDir := strings.TrimSpace(opts.TempDir)
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &FilesystemDiffGenerator{tempDir: tempDir}
}

// Capture archives the read-write workspace mount into a tarball, enabling downstream publication.
func (g *FilesystemDiffGenerator) Capture(ctx context.Context, req DiffRequest) (DiffResult, error) {
	if g == nil {
		return DiffResult{}, fmt.Errorf("step: diff generator not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	overlayPath, err := locateWritableMount(req)
	if err != nil {
		return DiffResult{}, err
	}
	if overlayPath == "" {
		return DiffResult{}, fmt.Errorf("step: diff capture requires writable mount")
	}

	diffFile, err := os.CreateTemp(g.tempDir, "ploy-diff-*.tar")
	if err != nil {
		return DiffResult{}, fmt.Errorf("step: create diff file: %w", err)
	}

	if err := archiveDirectory(ctx, overlayPath, diffFile); err != nil {
		_ = diffFile.Close()
		return DiffResult{}, err
	}

	if err := diffFile.Close(); err != nil {
		return DiffResult{}, err
	}

	return DiffResult{Path: diffFile.Name()}, nil
}

func locateWritableMount(req DiffRequest) (string, error) {
	for _, input := range req.Manifest.Inputs {
		if input.Mode == contracts.StepInputModeReadWrite {
			path, ok := req.Workspace.Inputs[input.Name]
			if !ok {
				return "", fmt.Errorf("step: workspace missing mount for %s", input.Name)
			}
			return path, nil
		}
	}
	return "", nil
}

func archiveDirectory(ctx context.Context, source string, writer io.Writer) error {
	tw := tar.NewWriter(writer)

	walkErr := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if info.Mode().IsRegular() || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			var linkTarget string
			if info.Mode()&os.ModeSymlink != 0 {
				linkTarget, err = os.Readlink(path)
				if err != nil {
					return err
				}
			}
			header, err := tar.FileInfoHeader(info, linkTarget)
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(rel)
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, file); err != nil {
				closeErr := file.Close()
				if closeErr != nil {
					err = errors.Join(err, closeErr)
				}
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			return nil
		}
		return nil
	})
	closeErr := tw.Close()
	if walkErr != nil {
		if closeErr != nil {
			return errors.Join(walkErr, closeErr)
		}
		return walkErr
	}
	return closeErr
}
