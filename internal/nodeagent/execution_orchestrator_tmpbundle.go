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
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// materializeTmpBundle downloads a spec bundle, verifies its SHA-256 digest, and
// extracts the archive into stagingDir. Extraction applies traversal-safe rules.
func (r *runController) materializeTmpBundle(ctx context.Context, bundle *contracts.TmpBundleRef, stagingDir string) error {
	if bundle == nil {
		return nil
	}
	if r.artifactUploader == nil {
		return fmt.Errorf("uploader not initialized")
	}
	data, err := r.artifactUploader.DownloadSpecBundle(ctx, bundle.BundleID)
	if err != nil {
		return fmt.Errorf("download bundle %s: %w", bundle.BundleID, err)
	}
	if err := verifyBundleDigest(data, bundle.Digest); err != nil {
		return fmt.Errorf("bundle %s: %w", bundle.BundleID, err)
	}
	if err := extractTmpBundle(data, stagingDir); err != nil {
		return fmt.Errorf("extract bundle %s: %w", bundle.BundleID, err)
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

// extractTmpBundle decompresses and extracts a tar.gz bundle into stagingDir.
// Security rules enforced per entry:
//   - Symlink entries are rejected (TypeSymlink).
//   - Hardlink entries are rejected (TypeLink).
//   - Absolute paths are rejected.
//   - Any ".." path component is rejected.
//   - Duplicate canonical paths are rejected.
//   - Only regular files and directories are extracted; unknown types are skipped.
func extractTmpBundle(data []byte, stagingDir string) error {
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
