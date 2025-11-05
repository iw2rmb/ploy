package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ArtifactUploader uploads artifact bundles (tar.gz) to the control-plane server.
type ArtifactUploader struct {
	cfg    Config
	client *http.Client
}

// NewArtifactUploader creates a new artifact uploader.
func NewArtifactUploader(cfg Config) (*ArtifactUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &ArtifactUploader{
		cfg:    cfg,
		client: client,
	}, nil
}

// UploadArtifact creates a tar.gz bundle from the specified paths and uploads it to the server.
func (u *ArtifactUploader) UploadArtifact(ctx context.Context, runID, stageID string, paths []string, name string) (string, string, error) {
	if len(paths) == 0 {
		return "", "", nil // Nothing to upload.
	}

	// Create tar.gz bundle from the specified paths.
	bundleBytes, err := createTarGzBundle(paths)
	if err != nil {
		return "", "", fmt.Errorf("create tar.gz bundle: %w", err)
	}

	// Check size cap (≤ 1 MiB gzipped).
	const maxBundleSize = 1 << 20 // 1 MiB
	if len(bundleBytes) > maxBundleSize {
		return "", "", fmt.Errorf("gzipped artifact bundle exceeds size cap: %d > %d bytes", len(bundleBytes), maxBundleSize)
	}

	// Build request payload.
	payload := map[string]interface{}{
		"run_id": runID,
		"bundle": bundleBytes,
	}
	if name != "" {
		payload["name"] = name
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL.
	url := fmt.Sprintf("%s/v1/nodes/%s/stage/%s/artifact", u.cfg.ServerURL, u.cfg.NodeID, stageID)

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request.
	resp, err := u.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status.
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	var out struct {
		ArtifactBundleID string `json:"artifact_bundle_id"`
		CID              string `json:"cid"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.ArtifactBundleID, out.CID, nil
}

// createTarGzBundle creates a gzipped tar archive from the given file paths.
func createTarGzBundle(paths []string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for _, root := range paths {
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

		base := filepath.Base(absRoot)

		// Write the root itself (dir or file) and recurse when directory.
		if err := addPathToTar(tarWriter, absRoot, base, info); err != nil {
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
				fi, err := d.Info()
				if err != nil {
					return err
				}
				// Compute name inside archive as base/rel
				rel, err := filepath.Rel(absRoot, p)
				if err != nil {
					return err
				}
				name := filepath.Join(base, rel)
				return addPathToTar(tarWriter, p, name, fi)
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

// addPathToTar writes a single filesystem entry to the tar using the provided name.
func addPathToTar(tw *tar.Writer, fsPath, name string, info os.FileInfo) error {
	// Support symlink headers by reading the target.
	linkTarget := ""
	if info.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(fsPath)
		if err != nil {
			return fmt.Errorf("readlink: %w", err)
		}
		linkTarget = t
	}

	header, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		return fmt.Errorf("create tar header: %w", err)
	}
	header.Name = name

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Write file content for regular files only.
	if info.Mode().IsRegular() {
		file, err := os.Open(fsPath)
		if err != nil {
			return fmt.Errorf("open file: %w", err)
		}
		defer file.Close()

		if _, err := io.Copy(tw, file); err != nil {
			return fmt.Errorf("copy file: %w", err)
		}
	}
	return nil
}
