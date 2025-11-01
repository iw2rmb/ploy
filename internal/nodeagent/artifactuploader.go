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
func (u *ArtifactUploader) UploadArtifact(ctx context.Context, runID, stageID string, paths []string, name string) error {
	if len(paths) == 0 {
		return nil // Nothing to upload.
	}

	// Create tar.gz bundle from the specified paths.
	bundleBytes, err := createTarGzBundle(paths)
	if err != nil {
		return fmt.Errorf("create tar.gz bundle: %w", err)
	}

	// Check size cap (≤ 1 MiB gzipped).
	const maxBundleSize = 1 << 20 // 1 MiB
	if len(bundleBytes) > maxBundleSize {
		return fmt.Errorf("gzipped artifact bundle exceeds size cap: %d > %d bytes", len(bundleBytes), maxBundleSize)
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
		return fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL.
	url := fmt.Sprintf("%s/v1/nodes/%s/stage/%s/artifact", u.cfg.ServerURL, u.cfg.NodeID, stageID)

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request.
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status.
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// createTarGzBundle creates a gzipped tar archive from the given file paths.
func createTarGzBundle(paths []string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for _, path := range paths {
		if err := addPathToTar(tarWriter, path); err != nil {
			_ = tarWriter.Close()
			_ = gzWriter.Close()
			return nil, fmt.Errorf("add %s to tar: %w", path, err)
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

// addPathToTar adds a file or directory to the tar archive.
func addPathToTar(tw *tar.Writer, path string) error {
	// Get file info.
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	// Create tar header from file info.
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header: %w", err)
	}

	// Use relative path as the name in the archive.
	header.Name = filepath.Base(path)

	// Write header.
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// If it's a regular file, write the content.
	if info.Mode().IsRegular() {
		file, err := os.Open(path)
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
