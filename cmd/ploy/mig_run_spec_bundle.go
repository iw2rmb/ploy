// mig_run_spec_bundle.go provides archive building and upload primitives
// for the Hydra file-record compiler.
//
// buildSourceArchive creates deterministic tar.gz payloads from files and
// directories. uploadSpecBundle uploads payloads to the server's spec-bundle
// store with built-in deduplication by content hash.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// buildSourceArchive creates a deterministic gzip-compressed tar archive from
// a single file or directory at resolvedPath.
//
// For content-addressed determinism the archive uses a fixed root name "content"
// so that identical source data and metadata produce the same archive regardless
// of source path. File and directory permissions plus modification times are
// preserved from the source.
func buildSourceArchive(resolvedPath string) ([]byte, error) {
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", resolvedPath, err)
	}

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	if info.IsDir() {
		dirHdr := &tar.Header{
			Name:     "content/",
			Typeflag: tar.TypeDir,
			Mode:     int64(info.Mode().Perm()),
			ModTime:  info.ModTime(),
		}
		if err := tw.WriteHeader(dirHdr); err != nil {
			return nil, fmt.Errorf("write dir header: %w", err)
		}
		if err := addDirToTar(tw, resolvedPath, "content"); err != nil {
			return nil, fmt.Errorf("walk dir: %w", err)
		}
	} else {
		data, err := os.ReadFile(resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", resolvedPath, err)
		}
		hdr := &tar.Header{
			Name:     "content",
			Typeflag: tar.TypeReg,
			Mode:     int64(info.Mode().Perm()),
			Size:     int64(len(data)),
			ModTime:  info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("write header: %w", err)
		}
		if _, err := tw.Write(data); err != nil {
			return nil, fmt.Errorf("write data: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	return buf.Bytes(), nil
}

// buildInlineContentArchive creates a deterministic gzip-compressed tar archive
// with a single regular file entry named "content".
func buildInlineContentArchive(content []byte) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Name:     "content",
		Typeflag: tar.TypeReg,
		Mode:     0o644,
		Size:     int64(len(content)),
		ModTime:  time.Unix(0, 0).UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return nil, fmt.Errorf("write data: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}
	return buf.Bytes(), nil
}

// addDirToTar recursively adds all files under dirPath to tw with paths relative
// to the entry name prefix. Entries within each directory are sorted for determinism.
// Symlinks are skipped silently.
func addDirToTar(tw *tar.Writer, dirPath, namePrefix string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dirPath, err)
	}

	// ReadDir already returns sorted entries.
	for _, de := range entries {
		childPath := filepath.Join(dirPath, de.Name())
		childName := namePrefix + "/" + de.Name()

		info, err := de.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", childPath, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Skip symlinks.
			continue
		}

		if de.IsDir() {
			dirHdr := &tar.Header{
				Name:     childName + "/",
				Typeflag: tar.TypeDir,
				Mode:     int64(info.Mode().Perm()),
				ModTime:  info.ModTime(),
			}
			if err := tw.WriteHeader(dirHdr); err != nil {
				return fmt.Errorf("write dir header %s: %w", childName, err)
			}
			if err := addDirToTar(tw, childPath, childName); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(childPath)
			if err != nil {
				return fmt.Errorf("read file %s: %w", childPath, err)
			}
			hdr := &tar.Header{
				Name:     childName,
				Typeflag: tar.TypeReg,
				Mode:     int64(info.Mode().Perm()),
				Size:     int64(len(data)),
				ModTime:  info.ModTime(),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("write header %s: %w", childName, err)
			}
			if _, err := tw.Write(data); err != nil {
				return fmt.Errorf("write data %s: %w", childName, err)
			}
		}
	}
	return nil
}

// computeSpecBundleCID computes the content identifier for a spec bundle archive
// using the same scheme as the server (bafy-prefixed SHA256 prefix).
func computeSpecBundleCID(data []byte) string {
	hash := sha256.Sum256(data)
	return "bafy" + hex.EncodeToString(hash[:])[:32]
}

// probeSpecBundleByCID checks whether a spec bundle with the given CID already
// exists on the server via HEAD /v1/spec-bundles?cid={cid}. Returns the
// bundle ID from the X-Bundle-ID response header (empty when not provided),
// true if a bundle with the CID exists, false for 404, and an error for other
// statuses.
func probeSpecBundleByCID(ctx context.Context, base *url.URL, client *http.Client, cid string) (bundleID string, exists bool, err error) {
	endpoint := base.JoinPath("v1", "spec-bundles")
	q := endpoint.Query()
	q.Set("cid", cid)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.String(), nil)
	if err != nil {
		return "", false, fmt.Errorf("spec-bundle probe: build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("spec-bundle probe: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Header.Get("X-Bundle-ID"), true, nil
	case http.StatusNotFound:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("spec-bundle probe: unexpected status %s", resp.Status)
	}
}

// uploadSpecBundle POSTs archiveBytes to POST base/v1/spec-bundles with Content-Type
// application/octet-stream. Accepts 200 (deduplicated) and 201 (new upload).
// Returns bundle_id, cid, and digest from the JSON response.
func uploadSpecBundle(ctx context.Context, base *url.URL, client *http.Client, archiveBytes []byte) (bundleID, cid, digest string, err error) {
	endpoint := base.JoinPath("v1", "spec-bundles")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(archiveBytes))
	if err != nil {
		return "", "", "", fmt.Errorf("spec-bundle upload: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("spec-bundle upload: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil {
			if msg := apiErr.Error; msg != "" {
				return "", "", "", fmt.Errorf("spec-bundle upload: server error: %s", msg)
			}
		}
		return "", "", "", fmt.Errorf("spec-bundle upload: unexpected status %s", resp.Status)
	}

	var response struct {
		BundleID string `json:"bundle_id"`
		CID      string `json:"cid"`
		Digest   string `json:"digest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", "", "", fmt.Errorf("spec-bundle upload: decode response: %w", err)
	}
	if response.BundleID == "" {
		return "", "", "", fmt.Errorf("spec-bundle upload: empty bundle_id in response")
	}
	if response.CID == "" {
		return "", "", "", fmt.Errorf("spec-bundle upload: empty cid in response")
	}
	if response.Digest == "" {
		return "", "", "", fmt.Errorf("spec-bundle upload: empty digest in response")
	}
	return response.BundleID, response.CID, response.Digest, nil
}
