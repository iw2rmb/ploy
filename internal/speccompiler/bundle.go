// bundle.go provides deterministic archive primitives for file records.
package speccompiler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// buildSourceArchive creates a deterministic gzip-compressed tar archive from
// a single file or directory at resolvedPath.
//
// For content-addressed determinism the archive uses a fixed root name "content"
// so that identical source data and metadata produce the same archive regardless
// of source path. File and directory permissions plus modification times are
// preserved from the source.
func (c *Compiler) buildSourceArchive(resolvedPath string) ([]byte, error) {
	info, err := c.source.Stat(resolvedPath)
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
		if err := c.addDirToTar(tw, resolvedPath, "content"); err != nil {
			return nil, fmt.Errorf("walk dir: %w", err)
		}
	} else {
		data, err := c.source.ReadFile(resolvedPath)
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

// addDirToTar recursively adds all files under dirPath to tw with paths relative
// to the entry name prefix. Entries within each directory are sorted for determinism.
// Symlinks are skipped silently.
func (c *Compiler) addDirToTar(tw *tar.Writer, dirPath, namePrefix string) error {
	entries, err := c.source.ReadDir(dirPath)
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
			if err := c.addDirToTar(tw, childPath, childName); err != nil {
				return err
			}
		} else {
			data, err := c.source.ReadFile(childPath)
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
func BundleCID(data []byte) string {
	hash := sha256.Sum256(data)
	return "bafy" + hex.EncodeToString(hash[:])[:32]
}
