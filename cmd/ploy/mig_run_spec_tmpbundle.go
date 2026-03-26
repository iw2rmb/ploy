// mig_run_spec_tmpbundle.go handles archiving tmp_dir entries into a gzip tar bundle
// and uploading the bundle to the server's spec-bundle store.
//
// This replaces the old per-file path→content inline embedding with a single
// content-addressed archive upload. The spec receives a tmp_bundle reference
// (bundle_id, cid, digest, entries) instead of inline file bytes.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// tmpDirEntry represents a single entry in a spec's tmp_dir array.
// Each entry has a logical name (used inside the container) and a local path.
type tmpDirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// buildSpecBundleArchive builds a deterministic gzip-compressed tar archive from the
// provided tmp_dir entries. Entries are sorted by Name for determinism.
// Files are added as single tar entries named <name>.
// Directories are added as a <name>/ entry plus all nested files (sorted, recursive).
// Symlinks are skipped silently.
// All tar headers use zero timestamps for content-addressed determinism.
// File permissions use 0o644 for files, 0o755 for directories.
func buildSpecBundleArchive(entries []tmpDirEntry) ([]byte, error) {
	// Sort entries by Name for determinism.
	sorted := make([]tmpDirEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for _, entry := range sorted {
		resolved, err := resolvePath(entry.Path)
		if err != nil {
			return nil, fmt.Errorf("tmp_dir entry %q: resolve path: %w", entry.Name, err)
		}

		info, err := os.Lstat(resolved)
		if err != nil {
			return nil, fmt.Errorf("tmp_dir entry %q: stat %s: %w", entry.Name, resolved, err)
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			// Skip symlinks silently.
			continue

		case info.IsDir():
			// Add directory entry.
			dirHdr := &tar.Header{
				Name:     entry.Name + "/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
				ModTime:  time.Time{},
			}
			if err := tw.WriteHeader(dirHdr); err != nil {
				return nil, fmt.Errorf("tmp_dir entry %q: write dir header: %w", entry.Name, err)
			}
			// Walk directory recursively with sorted order.
			if err := addDirToTar(tw, resolved, entry.Name); err != nil {
				return nil, fmt.Errorf("tmp_dir entry %q: walk dir: %w", entry.Name, err)
			}

		default:
			// Regular file.
			data, err := os.ReadFile(resolved)
			if err != nil {
				return nil, fmt.Errorf("tmp_dir entry %q: read file %s: %w", entry.Name, resolved, err)
			}
			hdr := &tar.Header{
				Name:     entry.Name,
				Typeflag: tar.TypeReg,
				Mode:     0o644,
				Size:     int64(len(data)),
				ModTime:  time.Time{},
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, fmt.Errorf("tmp_dir entry %q: write header: %w", entry.Name, err)
			}
			if _, err := tw.Write(data); err != nil {
				return nil, fmt.Errorf("tmp_dir entry %q: write data: %w", entry.Name, err)
			}
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
				Mode:     0o755,
				ModTime:  time.Time{},
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
				Mode:     0o644,
				Size:     int64(len(data)),
				ModTime:  time.Time{},
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

// archiveAndUploadTmpDirsInPlace processes all blocks with a tmp_dir key in the spec,
// builds a gzip tar archive of the entries, uploads it to the server, and replaces
// tmp_dir with tmp_bundle in-place.
//
// If no tmp_dir blocks are found, returns nil immediately without touching base/client.
// If tmp_dir blocks are found but base is nil, returns a descriptive error.
//
// Processes: steps[], build_gate.router, build_gate.healing.by_error_kind.*
func archiveAndUploadTmpDirsInPlace(ctx context.Context, base *url.URL, client *http.Client, spec map[string]any) error {
	// Collect all blocks that have tmp_dir to check if we need client/base.
	type blockRef struct {
		block  map[string]any
		prefix string
	}
	var blocks []blockRef

	if steps, ok := spec["steps"].([]any); ok {
		for i, s := range steps {
			if stepEntry, ok := s.(map[string]any); ok {
				if _, hasTmpDir := stepEntry["tmp_dir"]; hasTmpDir {
					blocks = append(blocks, blockRef{stepEntry, fmt.Sprintf("steps[%d]", i)})
				}
			}
		}
	}
	if bg, ok := spec["build_gate"].(map[string]any); ok {
		if router, ok := bg["router"].(map[string]any); ok {
			if _, hasTmpDir := router["tmp_dir"]; hasTmpDir {
				blocks = append(blocks, blockRef{router, "build_gate.router"})
			}
		}
		if healing, ok := bg["healing"].(map[string]any); ok {
			if byErrorKind, ok := healing["by_error_kind"].(map[string]any); ok {
				for errorKind, item := range byErrorKind {
					action, ok := item.(map[string]any)
					if !ok {
						continue
					}
					if _, hasTmpDir := action["tmp_dir"]; hasTmpDir {
						blocks = append(blocks, blockRef{action, fmt.Sprintf("build_gate.healing.by_error_kind.%s", errorKind)})
					}
				}
			}
		}
	}

	if len(blocks) == 0 {
		return nil
	}

	if base == nil {
		return fmt.Errorf("tmp_dir sections found but no server base URL available for bundle upload")
	}
	if client == nil {
		return fmt.Errorf("tmp_dir sections found but no HTTP client available for bundle upload")
	}

	for _, ref := range blocks {
		if err := processTmpDirBlock(ctx, base, client, ref.block, ref.prefix); err != nil {
			return err
		}
	}
	return nil
}

// processTmpDirBlock converts a single block's tmp_dir into a tmp_bundle reference.
func processTmpDirBlock(ctx context.Context, base *url.URL, client *http.Client, block map[string]any, prefix string) error {
	raw := block["tmp_dir"]
	entriesRaw, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s.tmp_dir: expected array, got %T", prefix, raw)
	}

	var entries []tmpDirEntry
	for i, item := range entriesRaw {
		entryMap, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.tmp_dir[%d]: expected object, got %T", prefix, i, item)
		}
		nameVal, hasName := entryMap["name"]
		if !hasName {
			return fmt.Errorf("%s.tmp_dir[%d]: missing name field", prefix, i)
		}
		name, ok := nameVal.(string)
		if !ok {
			return fmt.Errorf("%s.tmp_dir[%d].name: expected string, got %T", prefix, i, nameVal)
		}
		name, err := contracts.NormalizeTmpFileName(name)
		if err != nil {
			return fmt.Errorf("%s.tmp_dir[%d].name: %w", prefix, i, err)
		}
		pathVal, hasPath := entryMap["path"]
		if !hasPath {
			return fmt.Errorf("%s.tmp_dir[%d]: missing path field", prefix, i)
		}
		path, ok := pathVal.(string)
		if !ok {
			return fmt.Errorf("%s.tmp_dir[%d].path: expected string path, got %T", prefix, i, pathVal)
		}
		entries = append(entries, tmpDirEntry{Name: name, Path: path})
	}

	archiveBytes, err := buildSpecBundleArchive(entries)
	if err != nil {
		return fmt.Errorf("%s: build archive: %w", prefix, err)
	}

	bundleID, cid, digest, err := uploadSpecBundle(ctx, base, client, archiveBytes)
	if err != nil {
		return fmt.Errorf("%s: upload bundle: %w", prefix, err)
	}

	// Build entry names list.
	entryNames := make([]string, len(entries))
	for i, e := range entries {
		entryNames[i] = e.Name
	}

	delete(block, "tmp_dir")
	block["tmp_bundle"] = map[string]any{
		"bundle_id": bundleID,
		"cid":       cid,
		"digest":    digest,
		"entries":   entryNames,
	}
	return nil
}
