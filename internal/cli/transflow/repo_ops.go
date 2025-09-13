package transflow

import (
    "archive/tar"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// hasRepoChanges returns true if the working tree has any changes
func hasRepoChanges(repoPath string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// getHeadHash returns the current HEAD commit hash
func getHeadHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// createTarFromDir creates a tar archive of a directory using Go's archive/tar
func createTarFromDir(srcDir, dstTar string) error {
    // Remove existing tar if any
    _ = os.Remove(dstTar)

    f, err := os.Create(dstTar)
    if err != nil {
        return fmt.Errorf("create tar: %w", err)
    }
    defer func() {
        _ = f.Close()
    }()

    tw := tar.NewWriter(f)
    defer func() { _ = tw.Close() }()

    walkFn := func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return err
        }
        rel, err := filepath.Rel(srcDir, path)
        if err != nil {
            return err
        }
        if rel == "." { return nil }
        name := "./" + filepath.ToSlash(rel)

        info, err := d.Info()
        if err != nil { return err }
        if info.IsDir() {
            // Do not emit directory headers; implied by file paths
            return nil
        }
        if info.Mode().IsRegular() {
            hdr := &tar.Header{ Name: name, ModTime: info.ModTime() }
            hdr.Mode = int64(info.Mode().Perm())
            hdr.Typeflag = tar.TypeReg
            hdr.Size = info.Size()
            if err := tw.WriteHeader(hdr); err != nil { return err }
            rf, err := os.Open(path)
            if err != nil {
                return err
            }
            if _, err := io.Copy(tw, rf); err != nil {
                _ = rf.Close()
                return err
            }
            _ = rf.Close()
            return nil
        }
        // Skip non-regular files
        return nil
    }

    if err := filepath.WalkDir(srcDir, walkFn); err != nil {
        // Fallback to system tar if Go tar hits an unexpected platform edge case
        _ = f.Close()
        _ = os.Remove(dstTar)
        cmd := exec.Command("tar", "-cf", dstTar, ".")
        cmd.Dir = srcDir
        if out, e2 := cmd.CombinedOutput(); e2 != nil {
            return fmt.Errorf("walk dir: %v; fallback tar failed: %v: %s", err, e2, string(out))
        }
        // success via fallback
        return nil
    }

    // Ensure tar not empty
    if st, err := f.Stat(); err == nil {
        if st.Size() == 0 {
            return fmt.Errorf("created empty tar archive")
        }
    }
    return nil
}

// test indirection for getHeadHash
var getHeadHashFn = getHeadHash
