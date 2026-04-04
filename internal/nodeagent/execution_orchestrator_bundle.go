package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// materializeHydraResources collects unique content hashes from the manifest's
// CA/In/Out/Home entries, downloads each referenced spec bundle by its bundleID
// (resolved via bundleMap), verifies the SHA-256 digest, and extracts the
// archive into stagingDir/<hash>. Returns nil when no entries require
// materialization.
func (r *runController) materializeHydraResources(ctx context.Context, manifest contracts.StepManifest, bundleMap map[string]string, stagingDir string) error {
	hashes := collectUniqueHashes(manifest)
	if len(hashes) == 0 {
		return nil
	}
	if r.artifactUploader == nil {
		return fmt.Errorf("uploader not initialized")
	}
	for _, hash := range hashes {
		bundleID, ok := bundleMap[hash]
		if !ok {
			return fmt.Errorf("no bundle mapping for hash %s", hash)
		}
		if err := r.materializeResource(ctx, bundleID, hash, stagingDir); err != nil {
			return err
		}
	}
	return nil
}

// materializeResource downloads a spec bundle by bundleID, verifies that its
// SHA-256 digest starts with the expected hash prefix, and extracts the
// archive into stagingDir/<hash>.
func (r *runController) materializeResource(ctx context.Context, bundleID, hash, stagingDir string) error {
	data, err := r.artifactUploader.DownloadSpecBundle(ctx, bundleID)
	if err != nil {
		return fmt.Errorf("download resource %s (bundle %s): %w", hash, bundleID, err)
	}
	if err := verifyDigestPrefix(data, hash); err != nil {
		return fmt.Errorf("resource %s: %w", hash, err)
	}
	dst := filepath.Join(stagingDir, hash)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create staging dir for %s: %w", hash, err)
	}
	if err := extractBundle(data, dst); err != nil {
		return fmt.Errorf("extract resource %s: %w", hash, err)
	}
	return nil
}

// collectUniqueHashes extracts all unique content hashes referenced by the
// manifest's CA, In, Out, and Home entries. Returns a deduplicated slice.
func collectUniqueHashes(manifest contracts.StepManifest) []string {
	seen := make(map[string]struct{})
	var hashes []string

	for _, entry := range manifest.CA {
		hash, err := contracts.ParseStoredCAEntry(entry)
		if err != nil {
			slog.Warn("skip invalid CA entry during hash collection", "entry", entry, "error", err)
			continue
		}
		if _, ok := seen[hash]; !ok {
			seen[hash] = struct{}{}
			hashes = append(hashes, hash)
		}
	}
	for _, entry := range manifest.In {
		parsed, err := contracts.ParseStoredInEntry(entry)
		if err != nil {
			slog.Warn("skip invalid In entry during hash collection", "entry", entry, "error", err)
			continue
		}
		if _, ok := seen[parsed.Hash]; !ok {
			seen[parsed.Hash] = struct{}{}
			hashes = append(hashes, parsed.Hash)
		}
	}
	for _, entry := range manifest.Out {
		parsed, err := contracts.ParseStoredOutEntry(entry)
		if err != nil {
			slog.Warn("skip invalid Out entry during hash collection", "entry", entry, "error", err)
			continue
		}
		if _, ok := seen[parsed.Hash]; !ok {
			seen[parsed.Hash] = struct{}{}
			hashes = append(hashes, parsed.Hash)
		}
	}
	for _, entry := range manifest.Home {
		parsed, err := contracts.ParseStoredHomeEntry(entry)
		if err != nil {
			slog.Warn("skip invalid Home entry during hash collection", "entry", entry, "error", err)
			continue
		}
		if _, ok := seen[parsed.Hash]; !ok {
			seen[parsed.Hash] = struct{}{}
			hashes = append(hashes, parsed.Hash)
		}
	}
	return hashes
}

// verifyDigestPrefix checks that the SHA-256 digest of data starts with the
// given hex prefix. The prefix may be 7–64 hex characters (shortHash format).
func verifyDigestPrefix(data []byte, prefix string) error {
	hash := sha256.Sum256(data)
	actual := hex.EncodeToString(hash[:])
	expected := strings.TrimSpace(prefix)
	expected = strings.TrimPrefix(expected, "sha256:")
	if !strings.HasPrefix(strings.ToLower(actual), strings.ToLower(expected)) {
		return fmt.Errorf("digest mismatch: expected prefix %s, got %s", expected, actual)
	}
	return nil
}

// verifyBundleDigest checks that the SHA-256 digest of data matches expectedDigest.
// expectedDigest may be a bare hex string or carry a "sha256:" prefix as returned
// by the server's upload response.
func verifyBundleDigest(data []byte, expectedDigest string) error {
	hash := sha256.Sum256(data)
	actual := hex.EncodeToString(hash[:])
	expected := strings.TrimSpace(expectedDigest)
	expected = strings.TrimPrefix(expected, "sha256:")
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("digest mismatch: expected %s, got %s", expectedDigest, actual)
	}
	return nil
}

// extractBundle decompresses and extracts a tar.gz bundle into stagingDir.
// Security rules enforced per entry:
//   - Symlink entries are rejected (TypeSymlink).
//   - Hardlink entries are rejected (TypeLink).
//   - Absolute paths are rejected.
//   - Any ".." path component is rejected.
//   - Duplicate canonical paths are rejected.
//   - Only regular files and directories are extracted; unknown types are skipped.
func extractBundle(data []byte, stagingDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	seen := make(map[string]struct{})

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		switch hdr.Typeflag {
		case tar.TypeSymlink:
			return fmt.Errorf("symlink entry %q not permitted in spec bundle", hdr.Name)
		case tar.TypeLink:
			return fmt.Errorf("hardlink entry %q not permitted in spec bundle", hdr.Name)
		}

		// Normalize and validate the entry path.
		cleaned := path.Clean(hdr.Name)
		if cleaned == "" || cleaned == "." {
			continue
		}
		if path.IsAbs(cleaned) {
			return fmt.Errorf("absolute path %q not permitted in spec bundle", hdr.Name)
		}
		for _, part := range strings.Split(cleaned, "/") {
			if part == ".." {
				return fmt.Errorf("path traversal in entry %q not permitted in spec bundle", hdr.Name)
			}
		}

		if _, dup := seen[cleaned]; dup {
			return fmt.Errorf("duplicate archive entry %q in spec bundle", cleaned)
		}
		seen[cleaned] = struct{}{}

		// Verify the resolved destination stays under stagingDir.
		dst := filepath.Join(stagingDir, filepath.FromSlash(cleaned))
		rel, err := filepath.Rel(stagingDir, dst)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("entry %q would escape staging directory", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", cleaned, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("create parent for %q: %w", cleaned, err)
			}
			perm := hdr.FileInfo().Mode().Perm()
			if perm == 0 {
				perm = 0o644
			}
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
			if err != nil {
				return fmt.Errorf("create file %q: %w", cleaned, err)
			}
			_, copyErr := io.Copy(f, tr)
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("write file %q: %w", cleaned, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close file %q: %w", cleaned, closeErr)
			}
		// Skip unknown entry types (devices, char files, FIFOs, etc.).
		}
	}

	return nil
}