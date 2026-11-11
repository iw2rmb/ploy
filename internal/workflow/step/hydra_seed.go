package step

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func seedDirsFromStaging(manifest contracts.StepManifest, mounts JobMounts) error {
	for _, kind := range contracts.HydraFileKinds() {
		targetDir, err := hydraSeedTarget(kind, mounts)
		if err != nil {
			return err
		}
		if err := seedDirFromStaging(kind, kind.Entries(manifest), mounts.Staging, targetDir); err != nil {
			return err
		}
	}
	return nil
}

func hydraSeedTarget(kind contracts.HydraFileKind, mounts JobMounts) (string, error) {
	switch kind {
	case contracts.HydraFileIn:
		return mounts.In, nil
	case contracts.HydraFileOut:
		return mounts.Out, nil
	case contracts.HydraFileHome:
		return mounts.Home, nil
	case contracts.HydraFileTmp:
		return mounts.Tmp, nil
	default:
		return "", fmt.Errorf("unsupported Hydra file kind %q", kind)
	}
}

func seedDirFromStaging(kind contracts.HydraFileKind, entries []string, stagingDir, targetDir string) error {
	if stagingDir == "" || targetDir == "" {
		return nil
	}
	cleanTargetDir := filepath.Clean(targetDir)
	for _, entry := range entries {
		parsed, err := contracts.ParseStoredEntry(kind, entry)
		if err != nil {
			return fmt.Errorf("%s entry %q: %w", kind, entry, err)
		}
		if kind == contracts.HydraFileHome && parsed.ReadOnly {
			continue
		}
		rel := filepath.FromSlash(parsed.Dst)
		if kind != contracts.HydraFileHome {
			rel = strings.TrimPrefix(rel, string(filepath.Separator)+kind.String()+string(filepath.Separator))
		}
		src := filepath.Join(stagingDir, parsed.Hash, "content")
		dst := filepath.Clean(filepath.Join(targetDir, rel))
		if dst != cleanTargetDir && !strings.HasPrefix(dst, cleanTargetDir+string(filepath.Separator)) {
			return fmt.Errorf("%s entry %q: resolved path %s escapes %sDir", kind, entry, dst, kind)
		}
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("seed %s %s: %w", kind, parsed.Dst, err)
		}
	}
	return nil
}

// copyPath copies src to dst. If src is a directory, it copies recursively.
// If src is a file, it copies the file preserving permissions and timestamps.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst, info.Mode().Perm(), info.ModTime())
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, de := range entries {
		s := filepath.Join(src, de.Name())
		d := filepath.Join(dst, de.Name())
		if de.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
		} else {
			info, err := de.Info()
			if err != nil {
				return err
			}
			if err := copyFile(s, d, info.Mode().Perm(), info.ModTime()); err != nil {
				return err
			}
		}
	}
	if err := os.Chmod(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode, modTime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(df, sf); err != nil {
		if closeErr := df.Close(); closeErr != nil {
			return fmt.Errorf("copy file close error: %w (copy error: %v)", closeErr, err)
		}
		return err
	}
	if err := df.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, perm); err != nil {
		return err
	}
	return os.Chtimes(dst, modTime, modTime)
}
