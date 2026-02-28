package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// ArtifactBundleEntry defines one filesystem source and its archive path inside
// an uploaded artifact bundle.
type ArtifactBundleEntry struct {
	SourcePath  string
	ArchivePath string
}

// DiffUploader uploads diff and summary data to the control-plane server.
type DiffUploader struct {
	*baseUploader
}

// NewDiffUploader creates a new diff uploader.
func NewDiffUploader(cfg Config) (*DiffUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &DiffUploader{baseUploader: base}, nil
}

// UploadDiff compresses and uploads a diff to the server.
func (u *DiffUploader) UploadDiff(ctx context.Context, runID types.RunID, jobID types.JobID, diffBytes []byte, summary types.DiffSummary) error {
	gzippedDiff, err := gzipCompress(diffBytes, "gzipped diff")
	if err != nil {
		return err
	}
	payload := map[string]any{
		"patch":   gzippedDiff,
		"summary": summary,
	}
	apiPath := fmt.Sprintf("/v1/runs/%s/jobs/%s/diff", runID.String(), jobID.String())
	resp, err := u.postJSON(ctx, apiPath, payload, http.StatusCreated, "upload diff")
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// ArtifactUploader uploads artifact bundles (tar.gz) to the control-plane server.
type ArtifactUploader struct {
	*baseUploader
}

// NewArtifactUploader creates a new artifact uploader.
func NewArtifactUploader(cfg Config) (*ArtifactUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &ArtifactUploader{baseUploader: base}, nil
}

// UploadArtifact creates a tar.gz bundle from the specified paths and uploads it to the server.
func (u *ArtifactUploader) UploadArtifact(ctx context.Context, runID types.RunID, jobID types.JobID, paths []string, name string) (string, string, error) {
	if len(paths) == 0 {
		return "", "", nil
	}
	bundleBytes, err := createTarGzBundle(paths)
	if err != nil {
		return "", "", fmt.Errorf("create tar.gz bundle: %w", err)
	}
	return u.uploadBundle(ctx, runID, jobID, bundleBytes, name)
}

// UploadArtifactEntries creates a tar.gz bundle from explicit source->archive mappings
// and uploads it to the server.
func (u *ArtifactUploader) UploadArtifactEntries(ctx context.Context, runID types.RunID, jobID types.JobID, entries []ArtifactBundleEntry, name string) (string, string, error) {
	if len(entries) == 0 {
		return "", "", nil
	}
	bundleBytes, err := createTarGzBundleFromEntries(entries)
	if err != nil {
		return "", "", fmt.Errorf("create tar.gz bundle: %w", err)
	}
	return u.uploadBundle(ctx, runID, jobID, bundleBytes, name)
}

func (u *ArtifactUploader) uploadBundle(ctx context.Context, runID types.RunID, jobID types.JobID, bundleBytes []byte, name string) (string, string, error) {
	if err := validateUploadSize(bundleBytes, "gzipped artifact bundle"); err != nil {
		return "", "", err
	}
	payload := map[string]any{"bundle": bundleBytes}
	if name != "" {
		payload["name"] = name
	}
	apiPath := fmt.Sprintf("/v1/runs/%s/jobs/%s/artifact", runID.String(), jobID.String())
	resp, err := u.postJSON(ctx, apiPath, payload, http.StatusCreated, "upload artifact")
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		ArtifactBundleID string `json:"artifact_bundle_id"`
		CID              string `json:"cid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("decode response: %w", err)
	}
	if out.ArtifactBundleID == "" {
		return "", "", fmt.Errorf("server returned empty artifact_bundle_id")
	}
	return out.ArtifactBundleID, out.CID, nil
}

// createTarGzBundle creates a gzipped tar archive from the given file paths.
func createTarGzBundle(paths []string) ([]byte, error) {
	entries := make([]ArtifactBundleEntry, 0, len(paths))
	for _, p := range paths {
		entries = append(entries, ArtifactBundleEntry{SourcePath: p})
	}
	return createTarGzBundleFromEntries(entries)
}

func createTarGzBundleFromEntries(entries []ArtifactBundleEntry) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for _, entry := range entries {
		root := entry.SourcePath
		// Resolve to absolute path for consistent walking; header names will be relative.
		absRoot, err := filepath.Abs(root)
		if err != nil {
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			return nil, fmt.Errorf("abs path: %w", err)
		}

		info, err := os.Lstat(absRoot)
		if err != nil {
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			return nil, fmt.Errorf("stat %s: %w", root, err)
		}

		base := strings.TrimSpace(entry.ArchivePath)
		if base == "" {
			base = filepath.Base(absRoot)
		}
		base = normalizeArchivePath(base)
		if base == "" {
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			return nil, fmt.Errorf("invalid archive path for %s", root)
		}

		// Write the root itself (dir or file) and recurse when directory.
		// Use absRoot as allowed root for symlink validation.
		if err := addPathToTar(tarWriter, absRoot, base, info, absRoot); err != nil {
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			return nil, fmt.Errorf("add %s to tar: %w", root, err)
		}

		if info.IsDir() {
			// Walk directory contents and add entries relative to the root base.
			err = filepath.WalkDir(absRoot, func(p string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if p == absRoot { // already added root
					return nil
				}
				// Use os.Lstat instead of d.Info() to avoid following symlinks.
				// d.Info() follows symlinks, which could:
				//   1. Exfiltrate data from outside the workspace via symlinks
				//   2. Cause the tar to contain file contents instead of symlink entries
				// os.Lstat returns info about the symlink itself, not its target.
				fi, err := os.Lstat(p)
				if err != nil {
					return err
				}
				// Compute name inside archive as base/rel
				rel, err := filepath.Rel(absRoot, p)
				if err != nil {
					return err
				}
				name := filepath.Join(base, rel)
				return addPathToTar(tarWriter, p, name, fi, absRoot)
			})
			if err != nil {
				_ = tarWriter.Close()
				_ = gzWriter.Close()
				return nil, fmt.Errorf("walk %s: %w", root, err)
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		_ = gzWriter.Close()
		return nil, fmt.Errorf("close tar writer: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}

func normalizeArchivePath(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	n = filepath.ToSlash(n)
	n = strings.TrimPrefix(n, "./")
	n = strings.TrimPrefix(n, "/")
	n = path.Clean("/" + n)
	n = strings.TrimPrefix(n, "/")
	if n == "." || n == "" || strings.HasPrefix(n, "../") {
		return ""
	}
	return n
}

// addPathToTar writes a single filesystem entry to the tar using the provided name.
// allowedRoot is the directory that symlinks must resolve within; symlinks pointing
// outside are logged and skipped to prevent data exfiltration.
func addPathToTar(tw *tar.Writer, fsPath, name string, info os.FileInfo, allowedRoot string) error {
	// Support symlink headers by reading the target.
	linkTarget := ""
	if info.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(fsPath)
		if err != nil {
			return fmt.Errorf("readlink: %w", err)
		}

		// Validate symlink target is within allowed root to prevent data exfiltration.
		// Resolve the absolute target path.
		absTarget := t
		if !filepath.IsAbs(t) {
			absTarget = filepath.Join(filepath.Dir(fsPath), t)
		}
		absTarget = filepath.Clean(absTarget)

		// Check if target is within allowed root.
		rel, err := filepath.Rel(allowedRoot, absTarget)
		if err != nil || strings.HasPrefix(rel, "..") {
			slog.Warn("symlink target outside allowed root, skipping",
				"symlink", fsPath,
				"target", t,
				"allowed_root", allowedRoot,
			)
			return nil // Skip this symlink
		}

		linkTarget = t
	}

	header, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		return fmt.Errorf("create tar header: %w", err)
	}
	header.Name = normalizeArchivePath(name)
	if header.Name == "" {
		return fmt.Errorf("invalid tar header name %q", name)
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write file content for regular files only.
	if info.Mode().IsRegular() {
		file, err := os.Open(fsPath)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				slog.Warn("failed to close file during tar write", "path", fsPath, "error", closeErr)
			}
		}()

		if _, err := io.Copy(tw, file); err != nil {
			return fmt.Errorf("copy file: %w", err)
		}
	}
	return nil
}
